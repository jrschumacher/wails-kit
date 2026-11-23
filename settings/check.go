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
	ErrUnknownSetting errors.Code = "settings_unknown_key"
)

// DefaultCheckTimeout is how long a back-end check may run (BEHAVIOUR.md's
// task timeout).
const DefaultCheckTimeout = 2 * time.Minute

func init() {
	errors.RegisterMessages(map[errors.Code]string{
		ErrCheckFailed:    "Couldn't confirm this value. Check it and enter it again.",
		ErrCheckTimeout:   "The check took too long. Enter the value again to retry.",
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

// CheckValue checks values[key] and, if it passes, saves values with
// SetValues. It is the binding the settings window calls for a field with a
// Check, when the person leaves the field and again after they stop typing.
//
//   - The field's own validation runs first; if it fails, its errors come
//     back and the back-end check doesn't run.
//   - The check then runs under its own timeout, not the caller's: it carries
//     on when the window that asked closes.
//   - A pass saves values and clears any failure recorded for the key. An
//     empty value, and a password field holding SecretMask (no change), pass
//     without a check.
//   - A failure is recorded (CheckFailures), emitted as EventCheckFailed and
//     returned as a ValidationError with CodeCheck; nothing is saved.
//   - A check overtaken by a newer one for the same key saves, records and
//     emits nothing, and returns no errors.
func (s *Service) CheckValue(key string, values map[string]any) ([]ValidationError, error) {
	field, fn, ok := s.checkFor(key)
	if !ok {
		return nil, errors.Newf(ErrUnknownSetting, "unknown setting %q", key)
	}
	var own []ValidationError
	for _, e := range Validate(s.schema, values) {
		if e.Field == key {
			own = append(own, e)
		}
	}
	if len(own) > 0 {
		return own, nil
	}
	value := values[key]
	if str, isStr := value.(string); value == nil || (isStr && str == "") || (field.Type == FieldPassword && str == SecretMask) {
		fn = nil
	}

	s.checkMu.Lock()
	s.checkSeq[key]++
	seq := s.checkSeq[key]
	s.checkMu.Unlock()

	var err error
	if fn != nil {
		err = s.runCheck(fn, value, copyValues(values))
	}

	s.checkMu.Lock()
	if s.checkSeq[key] != seq {
		s.checkMu.Unlock()
		return nil, nil
	}
	if err == nil {
		delete(s.failures, key)
		s.checkMu.Unlock()
		return s.SetValues(values)
	}
	failure := ValidationError{Field: key, Message: checkMessage(err), Code: CodeCheck}
	s.failures[key] = failure
	s.checkMu.Unlock()

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

// CheckFailures returns the back-end checks that failed and haven't passed
// since, in schema order. Settings shows them when it opens, and opens on the
// first one's field.
func (s *Service) CheckFailures() []ValidationError {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
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
