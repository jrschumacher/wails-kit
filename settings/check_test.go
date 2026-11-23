package settings

import (
	"context"
	stderrors "errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
	"github.com/jrschumacher/wails-kit/keyring"
)

const errTokenRejected errors.Code = "test_token_rejected"

func init() {
	errors.RegisterMessages(map[errors.Code]string{errTokenRejected: "The service rejected this token. Paste a current one."})
}

func checkedService(t *testing.T, fn CheckFunc, opts ...ServiceOption) (*Service, *events.MemoryEmitter) {
	t.Helper()
	mem := events.NewMemoryEmitter()
	base := []ServiceOption{
		WithStoragePath(filepath.Join(t.TempDir(), "settings.json")),
		WithKeyring(keyring.NewMemoryStore()),
		WithEmitter(events.NewEmitter(mem)),
		WithGroup(Group{
			Key:   "account",
			Label: "Account",
			Fields: []Field{
				{Key: "account.name", Type: FieldText, Label: "Display name"},
				{Key: "account.token", Type: FieldPassword, Label: "API token", Check: &Check{Pending: "Checking token…"}, Validation: &Validation{Required: true, MinLen: 4}},
				{Key: "account.folder", Type: FieldText, Label: "Projects folder", Check: &Check{}},
				{Key: "account.plain", Type: FieldText, Label: "Plain", Check: &Check{}},
			},
			CheckFuncs: map[string]CheckFunc{
				"account.token":  fn,
				"account.folder": fn,
			},
		}),
	}
	return NewService(append(base, opts...)...), mem
}

func rejectBad(_ context.Context, value any, _ map[string]any) error {
	switch value {
	case "bad-token":
		return errors.New(errTokenRejected, "token rejected", nil)
	case "/raw":
		return stderrors.New("stat /raw: no such file or directory")
	}
	return nil
}

func TestCheckValue_PassSaves(t *testing.T) {
	svc, mem := checkedService(t, rejectBad)
	errs, err := svc.CheckValue("account.token", map[string]any{"account.token": "good-token", "account.name": "Sam"})
	if err != nil || errs != nil {
		t.Fatalf("got %v, %v", errs, err)
	}
	if got, _ := svc.GetSecret("account.token"); got != "good-token" {
		t.Errorf("secret = %q, want it saved", got)
	}
	values, _ := svc.GetValues()
	if values["account.name"] != "Sam" {
		t.Errorf("name = %v, want the other values saved too", values["account.name"])
	}
	if n := len(mem.Events()); n != 0 {
		t.Errorf("a pass emits nothing, got %d events", n)
	}
}

func TestCheckValue_FailureIsNotSavedAndIsReported(t *testing.T) {
	svc, mem := checkedService(t, rejectBad)
	errs, err := svc.CheckValue("account.token", map[string]any{"account.token": "bad-token"})
	if err != nil {
		t.Fatal(err)
	}
	want := ValidationError{Field: "account.token", Message: "The service rejected this token. Paste a current one.", Code: CodeCheck}
	if len(errs) != 1 || errs[0] != want {
		t.Fatalf("errs = %v, want %v", errs, want)
	}
	if svc.secrets.Has("account.token") {
		t.Error("a value that failed its check must not be saved")
	}
	if got := svc.CheckFailures(); len(got) != 1 || got[0] != want {
		t.Errorf("CheckFailures = %v", got)
	}
	evs := mem.Events()
	if len(evs) != 1 || evs[0].Name != EventCheckFailed {
		t.Fatalf("events = %v", evs)
	}
	payload := evs[0].Data.(CheckFailedPayload)
	wantPayload := CheckFailedPayload{Key: "account.token", Label: "API token", Message: want.Message, Notice: "Your API token wasn't saved.", Attention: true}
	if payload != wantPayload {
		t.Errorf("payload = %+v, want %+v", payload, wantPayload)
	}

	// A later pass clears the failure.
	if errs, _ := svc.CheckValue("account.token", map[string]any{"account.token": "good-token"}); errs != nil {
		t.Fatalf("errs = %v", errs)
	}
	if got := svc.CheckFailures(); len(got) != 0 {
		t.Errorf("CheckFailures after a pass = %v", got)
	}
}

func TestCheckValue_PlainErrorAndNeutralNotice(t *testing.T) {
	svc, mem := checkedService(t, rejectBad)
	errs, _ := svc.CheckValue("account.folder", map[string]any{"account.folder": "/raw"})
	if len(errs) != 1 || errs[0].Message != "Couldn't confirm this value. Check it and enter it again." {
		t.Fatalf("errs = %v; a raw error must not reach the person", errs)
	}
	p := mem.Events()[0].Data.(CheckFailedPayload)
	if p.Attention || p.Notice != "Your projects folder wasn't saved." {
		t.Errorf("payload = %+v", p)
	}
}

func TestCheckValue_OwnValidationFirst(t *testing.T) {
	called := false
	svc, mem := checkedService(t, func(context.Context, any, map[string]any) error { called = true; return nil })
	errs, _ := svc.CheckValue("account.token", map[string]any{"account.token": "ab"})
	if len(errs) != 1 || errs[0].Code != CodeMinLen {
		t.Fatalf("errs = %v", errs)
	}
	if called || len(mem.Events()) != 0 || len(svc.CheckFailures()) != 0 {
		t.Error("the back-end check must not run when the field's own validation fails")
	}
}

func TestCheckValue_MaskAndNoFuncPassWithoutCheck(t *testing.T) {
	called := false
	svc, _ := checkedService(t, func(context.Context, any, map[string]any) error { called = true; return stderrors.New("no") })
	if errs, err := svc.CheckValue("account.token", map[string]any{"account.token": SecretMask}); errs != nil || err != nil {
		t.Fatalf("mask: %v %v", errs, err)
	}
	if errs, err := svc.CheckValue("account.plain", map[string]any{"account.token": SecretMask, "account.plain": "x"}); errs != nil || err != nil {
		t.Fatalf("no func: %v %v", errs, err)
	}
	if called {
		t.Error("the check ran")
	}
	values, _ := svc.GetValues()
	if values["account.plain"] != "x" {
		t.Errorf("plain = %v", values["account.plain"])
	}
}

func TestCheckValue_UnknownKey(t *testing.T) {
	svc, _ := checkedService(t, rejectBad)
	_, err := svc.CheckValue("nope", nil)
	if !errors.IsCode(err, ErrUnknownSetting) {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckValue_Timeout(t *testing.T) {
	svc, _ := checkedService(t, func(ctx context.Context, _ any, _ map[string]any) error {
		<-ctx.Done()
		return ctx.Err()
	}, WithCheckTimeout(20*time.Millisecond))
	errs, _ := svc.CheckValue("account.folder", map[string]any{"account.folder": "/x"})
	if len(errs) != 1 || errs[0].Message != "The check took too long. Enter the value again to retry." {
		t.Fatalf("errs = %v", errs)
	}
	// A check that ignores its context still ends at the timeout.
	block := make(chan struct{})
	defer close(block)
	svc2, _ := checkedService(t, func(context.Context, any, map[string]any) error { <-block; return nil }, WithCheckTimeout(20*time.Millisecond))
	errs, _ = svc2.CheckValue("account.folder", map[string]any{"account.folder": "/x"})
	if len(errs) != 1 || errs[0].Code != CodeCheck {
		t.Fatalf("errs = %v", errs)
	}
}

// The check carries on when the caller goes away: CheckValue takes no
// context from the caller, and an overtaken check changes nothing.
func TestCheckValue_OvertakenCheckChangesNothing(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	svc, mem := checkedService(t, func(_ context.Context, value any, _ map[string]any) error {
		started <- struct{}{}
		if value == "/slow" {
			<-release
			return stderrors.New("fails late")
		}
		return nil
	})
	done := make(chan []ValidationError)
	go func() {
		errs, _ := svc.CheckValue("account.folder", map[string]any{"account.folder": "/slow", "account.token": SecretMask})
		done <- errs
	}()
	<-started
	if errs, _ := svc.CheckValue("account.folder", map[string]any{"account.folder": "/fast", "account.token": SecretMask}); errs != nil {
		t.Fatalf("errs = %v", errs)
	}
	close(release)
	if errs := <-done; errs != nil {
		t.Errorf("overtaken check returned %v", errs)
	}
	if len(mem.Events()) != 0 || len(svc.CheckFailures()) != 0 {
		t.Error("an overtaken check must not report a failure")
	}
	values, _ := svc.GetValues()
	if values["account.folder"] != "/fast" {
		t.Errorf("folder = %v", values["account.folder"])
	}
}

func TestInSentence(t *testing.T) {
	for in, want := range map[string]string{"Display name": "display name", "API key": "API key", "GitHub token": "GitHub token", "token": "token", "": ""} {
		if got := inSentence(in); got != want {
			t.Errorf("inSentence(%q) = %q, want %q", in, got, want)
		}
	}
}
