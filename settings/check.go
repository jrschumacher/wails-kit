package settings

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
)

// CodeCheck is the ValidationError.Code of a field whose back-end check failed.
const CodeCheck = "check"

// EventCheckFailed is emitted when a back-end check fails, so the main
// window can say so when Settings has closed.
const EventCheckFailed = "settings:check_failed"

// Error codes for back-end checks. Their messages show under the field.
const (
	// ErrCheckFailed: the check returned an error that isn't a *errors.UserError.
	ErrCheckFailed errors.Code = "settings_check_failed"
	// ErrCheckTimeout: the check ran past the check timeout.
	ErrCheckTimeout errors.Code = "settings_check_timeout"
	// ErrUnknownSetting: CheckValue was asked about a key the schema lacks.
	ErrUnknownSetting errors.Code = "settings_unknown_setting"
)

// DefaultCheckTimeout is how long a back-end check may run (BEHAVIOUR.md's
// task timeout).
const DefaultCheckTimeout = 2 * time.Minute

func init() {
	errors.RegisterMessages(map[errors.Code]string{
		ErrCheckFailed:    "Couldn't confirm this value. Check it and enter it again.",
		ErrCheckTimeout:   "The check timed out. Enter the value again once the service can be reached.",
		ErrUnknownSetting: "This setting doesn't exist.",
	})
}

// CheckFailedPayload is EventCheckFailed's payload.
type CheckFailedPayload struct {
	// Key is the field's key.
	Key string `json:"key"`
	// Label is the field's label.
	Label string `json:"label"`
	// Message is the field's error, as Settings shows it under the field.
	Message string `json:"message"`
	// Notice is the main window's words: "Your API key wasn't saved."
	Notice string `json:"notice"`
	// Attention is true when the field is required: the app can't work
	// without it, so the Notice takes the attention tone.
	Attention bool `json:"attention"`
}

// WithEmitter sets the emitter for EventCheckFailed.
func WithEmitter(e *events.Emitter) ServiceOption {
	return func(s *Service) {
		s.emitter = e
	}
}

// WithCheckTimeout sets how long a back-end check may run before it fails
// with ErrCheckTimeout (default DefaultCheckTimeout).
func WithCheckTimeout(d time.Duration) ServiceOption {
	return func(s *Service) {
		if d > 0 {
			s.checkTimeout = d
		}
	}
}

// CheckValue checks values[key] and, if it passes, saves that one value. It
// is the binding the settings window calls for a field with a Check, when
// the person leaves the field and again after they stop typing.
//
// The other entries of values are context only: the check and the field's
// condition see them, but CheckValue never saves them, so a check that
// returns minutes later can't put back a value changed meanwhile.
//
//   - The field's own validation runs first; if it fails, its errors come
//     back and the back-end check doesn't run.
//   - The check then runs under its own timeout, not the caller's: it carries
//     on when the window that asked closes.
//   - A pass saves values[key] alone, clears any failure recorded for the
//     key, and returns no errors: no errors means the value was saved. An
//     empty value, and a password field holding SecretMask (no change), pass
//     without a check.
//   - A failure is recorded (CheckFailures), emitted as EventCheckFailed and
//     returned as a ValidationError with CodeCheck; nothing is saved.
//   - When the key changed while the check ran (a newer CheckValue for it,
//     or a SetValues that changed it), nothing is saved, recorded or
//     emitted, and the result is a CodeCheck error saying so.
func (s *Service) CheckValue(key string, values map[string]any) ([]ValidationError, error) {
	field, fn, ok := s.checkFor(key)
	if !ok {
		return nil, errors.Newf(ErrUnknownSetting, "unknown setting %q", key)
	}
	// Any check already running for the key is out of date from here on.
	s.mu.Lock()
	s.gen[key]++
	gen := s.gen[key]
	s.mu.Unlock()

	if own := validateOne(field, values); len(own) > 0 {
		return own, nil
	}
	value := values[key]
	if str, isStr := value.(string); value == nil || (isStr && str == "") || (field.Type == FieldPassword && str == SecretMask) {
		fn = nil
	}

	var err error
	if fn != nil {
		err = s.runCheck(fn, value, copyValues(values))
	}

	s.mu.Lock()
	if s.gen[key] != gen {
		s.mu.Unlock()
		return []ValidationError{{Field: key, Message: changedMessage, Code: CodeCheck}}, nil
	}
	if err == nil {
		saveErr := s.saveOneLocked(field, value)
		if saveErr == nil {
			delete(s.failures, key)
		}
		s.mu.Unlock()
		if saveErr != nil {
			return nil, errors.Wrap(errors.ErrStorageWrite, "save checked setting", saveErr)
		}
		s.notifySaved()
		return nil, nil
	}
	failure := ValidationError{Field: key, Message: checkMessage(err), Code: CodeCheck}
	s.failures[key] = failure
	s.mu.Unlock()

	if s.emitter != nil {
		s.emitter.Emit(EventCheckFailed, CheckFailedPayload{
			Key:       key,
			Label:     field.Label,
			Message:   failure.Message,
			Notice:    fmt.Sprintf("Your %s wasn't saved.", inSentence(labelOf(field))),
			Attention: field.Validation != nil && field.Validation.Required,
		})
	}
	return []ValidationError{failure}, nil
}

// changedMessage: the key changed while its check ran, so the result no longer applies.
const changedMessage = "This setting changed while it was being checked. Enter it again to check it."

// CheckFailures returns the back-end checks that failed and haven't passed
// since, in schema order. A SetValues that changes the key, or a CheckValue
// that saves it (clearing the field included), drops its failure. Settings
// shows them when it opens, and opens on the first one's field.
func (s *Service) CheckFailures() []ValidationError {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ValidationError
	for _, group := range s.schema.Groups {
		for _, field := range group.Fields {
			if f, ok := s.failures[field.Key]; ok {
				out = append(out, f)
			}
		}
	}
	return out
}

// validateOne validates one field, with values as context for its condition.
func validateOne(field Field, values map[string]any) []ValidationError {
	if field.Validation == nil && field.Type != FieldSelect && field.Type != FieldToggle {
		return nil
	}
	if field.Condition != nil && !conditionMet(field.Condition, values) {
		return nil
	}
	return validateField(field, values[field.Key], values)
}

// saveOneLocked writes one field's value: a password to the keyring (the
// mask is no change, "" clears it), anything else into the settings file,
// merged with what's there. The caller holds s.mu.
func (s *Service) saveOneLocked(field Field, value any) error {
	switch field.Type {
	case FieldComputed:
		return nil
	case FieldPassword:
		str, _ := value.(string)
		switch str {
		case SecretMask:
			return nil
		case "":
			_ = s.secrets.Delete(field.Key)
			return nil
		}
		return s.secrets.Set(field.Key, str)
	}
	return s.store.Save(map[string]any{field.Key: value})
}

// notifySaved calls the onChange callbacks with every saved value (secrets
// masked) after CheckValue saves one.
func (s *Service) notifySaved() {
	if len(s.onChange) == 0 {
		return
	}
	values, err := s.store.Load()
	if err != nil {
		return
	}
	for k := range s.passwordKeys() {
		if s.secrets.Has(k) {
			values[k] = SecretMask
		}
	}
	for _, fn := range s.onChange {
		fn(values)
	}
}

// runCheck runs fn under the check timeout, returning when it does or when
// the timeout ends, whichever comes first.
func (s *Service) runCheck(fn CheckFunc, value any, values map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.checkTimeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("check panicked: %v", r)
			}
		}()
		done <- fn(ctx, value, values)
	}()
	select {
	case err := <-done:
		if err != nil && stderrors.Is(err, context.DeadlineExceeded) {
			return errors.Wrap(ErrCheckTimeout, "settings check timed out", err)
		}
		return err
	case <-ctx.Done():
		return errors.Wrap(ErrCheckTimeout, "settings check timed out", ctx.Err())
	}
}

// checkFor finds key's field and its CheckFunc (nil when the field has none).
func (s *Service) checkFor(key string) (Field, CheckFunc, bool) {
	for _, group := range s.schema.Groups {
		for _, field := range group.Fields {
			if field.Key == key {
				return field, group.CheckFuncs[key], true
			}
		}
	}
	return Field{}, nil, false
}

func checkMessage(err error) string {
	var ue *errors.UserError
	if stderrors.As(err, &ue) && ue.UserMsg != "" {
		return ue.UserMsg
	}
	return errors.New(ErrCheckFailed, "settings check failed", err).UserMsg
}

func labelOf(field Field) string {
	if field.Label != "" {
		return field.Label
	}
	return field.Key
}

// inSentence lowercases a label's first letter for use mid-sentence, unless
// its first word has other capitals ("API key", "GitHub token").
func inSentence(label string) string {
	first, size := utf8.DecodeRuneInString(label)
	if first == utf8.RuneError || !unicode.IsUpper(first) {
		return label
	}
	for _, r := range label[size:] {
		if unicode.IsSpace(r) {
			break
		}
		if unicode.IsUpper(r) {
			return label
		}
	}
	return string(unicode.ToLower(first)) + label[size:]
}

func copyValues(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
