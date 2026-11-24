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

func TestCheckValue_PassSavesOnlyItsKey(t *testing.T) {
	svc, mem := checkedService(t, rejectBad)
	if _, err := svc.SetValues(map[string]any{"account.name": "Sam", "account.token": "first-token"}); err != nil {
		t.Fatal(err)
	}
	errs, err := svc.CheckValue("account.token", map[string]any{"account.token": "good-token", "account.name": "Stale"})
	if err != nil || errs != nil {
		t.Fatalf("got %v, %v", errs, err)
	}
	if got, _ := svc.GetSecret("account.token"); got != "good-token" {
		t.Errorf("secret = %q, want it saved", got)
	}
	values, _ := svc.GetValues()
	if values["account.name"] != "Sam" {
		t.Errorf("name = %v; the other values are context, never saved", values["account.name"])
	}
	if n := len(mem.Events()); n != 0 {
		t.Errorf("a pass emits nothing, got %d events", n)
	}
}

// Another field failing its validation (here a required token left empty)
// doesn't stop a checked field from saving, and no errors means it saved.
func TestCheckValue_PassSavesWhenAnotherFieldIsInvalid(t *testing.T) {
	var notified []map[string]any
	svc, _ := checkedService(t, rejectBad, WithOnChange(func(v map[string]any) { notified = append(notified, v) }))
	values := map[string]any{"account.folder": "/work", "account.token": ""}
	if errs := Validate(svc.GetSchema(), values); len(errs) == 0 {
		t.Fatal("the test needs another field to be invalid")
	}
	errs, err := svc.CheckValue("account.folder", values)
	if err != nil || errs != nil {
		t.Fatalf("got %v, %v", errs, err)
	}
	saved, _ := svc.GetValues()
	if saved["account.folder"] != "/work" {
		t.Errorf("folder = %v, want it saved", saved["account.folder"])
	}
	if len(notified) != 1 || notified[0]["account.folder"] != "/work" {
		t.Errorf("onChange got %v", notified)
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
	if errs, err := svc.CheckValue("account.plain", map[string]any{"account.plain": "x"}); errs != nil || err != nil {
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
	if len(errs) != 1 || errs[0].Message != "The check timed out. Enter the value again once the service can be reached." {
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

// slowCheck is a CheckFunc that blocks on the values listed in gates until
// their channel closes, then passes (or fails for "bad-token"). started
// receives each value as its check begins.
func slowCheck(gates map[any]chan struct{}, started chan any) CheckFunc {
	return func(_ context.Context, value any, values map[string]any) error {
		started <- value
		if g, ok := gates[value]; ok {
			<-g
		}
		return rejectBad(context.Background(), value, values)
	}
}

func checkAsync(svc *Service, key string, values map[string]any) chan []ValidationError {
	done := make(chan []ValidationError, 1)
	go func() {
		errs, _ := svc.CheckValue(key, values)
		done <- errs
	}()
	return done
}

func changed(errs []ValidationError) bool {
	return len(errs) == 1 && errs[0].Code == CodeCheck && errs[0].Message == changedMessage
}

// A check that returns after another field changed doesn't put the old value back.
func TestCheckValue_RaceDoesNotRevertAnotherField(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan any, 4)
	svc, _ := checkedService(t, slowCheck(map[any]chan struct{}{"/slow": gate}, started))
	if _, err := svc.SetValues(map[string]any{"account.name": "Old", "account.token": "tok-1"}); err != nil {
		t.Fatal(err)
	}
	done := checkAsync(svc, "account.folder", map[string]any{"account.folder": "/slow", "account.name": "Old", "account.token": SecretMask})
	<-started
	if errs, err := svc.SetValues(map[string]any{"account.name": "New", "account.token": SecretMask}); errs != nil || err != nil {
		t.Fatalf("%v %v", errs, err)
	}
	close(gate)
	if errs := <-done; errs != nil {
		t.Fatalf("errs = %v", errs)
	}
	values, _ := svc.GetValues()
	if values["account.name"] != "New" || values["account.folder"] != "/slow" {
		t.Errorf("name = %v, folder = %v", values["account.name"], values["account.folder"])
	}
}

// A token cleared while its earlier value was being checked stays cleared,
// whether it was cleared through SetValues or through CheckValue.
func TestCheckValue_RaceDoesNotRestoreAClearedToken(t *testing.T) {
	for _, via := range []string{"SetValues", "CheckValue"} {
		t.Run(via, func(t *testing.T) {
			gate := make(chan struct{})
			started := make(chan any, 4)
			svc, _ := checkedService(t, slowCheck(map[any]chan struct{}{"good-token": gate}, started))
			// The token is optional here, so SetValues may clear it; one is saved.
			svc.schema.Groups[0].Fields[1].Validation = &Validation{MinLen: 4}
			if _, err := svc.SetValues(map[string]any{"account.token": "tok-1"}); err != nil {
				t.Fatal(err)
			}
			done := checkAsync(svc, "account.token", map[string]any{"account.token": "good-token"})
			<-started
			if via == "SetValues" {
				if _, err := svc.SetValues(map[string]any{"account.token": ""}); err != nil {
					t.Fatal(err)
				}
			} else if errs, err := svc.CheckValue("account.token", map[string]any{"account.token": ""}); errs != nil || err != nil {
				t.Fatalf("%v %v", errs, err)
			}
			close(gate)
			if errs := <-done; !changed(errs) {
				t.Fatalf("errs = %v, want the changed-while-checking error", errs)
			}
			if svc.secrets.Has("account.token") {
				t.Error("the cleared token came back")
			}
		})
	}
}

// Two passing checks for one key: only the newest saves, whichever returns first.
func TestCheckValue_RaceNewestCheckWins(t *testing.T) {
	for _, olderFirst := range []bool{true, false} {
		gateA, gateB := make(chan struct{}), make(chan struct{})
		started := make(chan any, 4)
		svc, mem := checkedService(t, slowCheck(map[any]chan struct{}{"/a": gateA, "/b": gateB}, started))
		a := checkAsync(svc, "account.folder", map[string]any{"account.folder": "/a"})
		<-started
		b := checkAsync(svc, "account.folder", map[string]any{"account.folder": "/b"})
		<-started
		var errsA, errsB []ValidationError
		if olderFirst {
			close(gateA)
			errsA = <-a
			close(gateB)
			errsB = <-b
		} else {
			close(gateB)
			errsB = <-b
			close(gateA)
			errsA = <-a
		}
		if !changed(errsA) || errsB != nil {
			t.Fatalf("olderFirst=%v: older %v, newer %v", olderFirst, errsA, errsB)
		}
		values, _ := svc.GetValues()
		if values["account.folder"] != "/b" {
			t.Errorf("olderFirst=%v: folder = %v", olderFirst, values["account.folder"])
		}
		if len(mem.Events()) != 0 || len(svc.CheckFailures()) != 0 {
			t.Error("an overtaken check must not report anything")
		}
	}
}

// A newer value that fails the field's own validation still overtakes an older check.
func TestCheckValue_OwnValidationFailureOvertakesOlderCheck(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan any, 4)
	svc, _ := checkedService(t, slowCheck(map[any]chan struct{}{"good-token": gate}, started))
	done := checkAsync(svc, "account.token", map[string]any{"account.token": "good-token"})
	<-started
	if errs, _ := svc.CheckValue("account.token", map[string]any{"account.token": "ab"}); len(errs) != 1 || errs[0].Code != CodeMinLen {
		t.Fatalf("errs = %v", errs)
	}
	close(gate)
	if errs := <-done; !changed(errs) {
		t.Fatalf("errs = %v", errs)
	}
	if svc.secrets.Has("account.token") {
		t.Error("the overtaken check saved its value")
	}
}

// A recorded failure goes when SetValues changes the key, or when the field
// is cleared through CheckValue; an unchanged key keeps it.
func TestCheckFailures_DroppedWhenTheKeyChanges(t *testing.T) {
	svc, _ := checkedService(t, rejectBad)
	fail := func(key, value string) {
		t.Helper()
		if errs, _ := svc.CheckValue(key, map[string]any{key: value}); len(errs) != 1 || errs[0].Code != CodeCheck {
			t.Fatalf("errs = %v", errs)
		}
	}
	fail("account.folder", "/raw")
	if _, err := svc.SetValues(map[string]any{"account.token": "tok-1", "account.name": "x"}); err != nil {
		t.Fatal(err)
	}
	if len(svc.CheckFailures()) != 1 {
		t.Fatal("a SetValues that doesn't change the key keeps its failure")
	}
	if _, err := svc.SetValues(map[string]any{"account.token": SecretMask, "account.folder": "/ok"}); err != nil {
		t.Fatal(err)
	}
	if got := svc.CheckFailures(); len(got) != 0 {
		t.Fatalf("after SetValues changed it: %v", got)
	}
	fail("account.folder", "/raw")
	if errs, err := svc.CheckValue("account.folder", map[string]any{"account.folder": ""}); errs != nil || err != nil {
		t.Fatalf("%v %v", errs, err)
	}
	if got := svc.CheckFailures(); len(got) != 0 {
		t.Fatalf("after clearing the field: %v", got)
	}
}

func TestInSentence(t *testing.T) {
	for in, want := range map[string]string{"Display name": "display name", "API key": "API key", "GitHub token": "GitHub token", "token": "token", "": ""} {
		if got := inSentence(in); got != want {
			t.Errorf("inSentence(%q) = %q, want %q", in, got, want)
		}
	}
}
