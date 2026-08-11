package kit

import (
	stderrors "errors"
	"fmt"

	kiterrors "github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/i18n"
)

// Error codes for the kit package.
const (
	// ErrComponentInit means one of the components New wires up failed to
	// construct. See GetComponent to find out which one.
	ErrComponentInit kiterrors.Code = "kit_component_init"
	// ErrConfig means New was called with structurally invalid AppInfo
	// (currently: an empty Name) before any component construction began.
	ErrConfig kiterrors.Code = "kit_config"
)

func init() {
	kiterrors.RegisterMessages(map[kiterrors.Code]i18n.Text{
		ErrComponentInit: i18n.T("wailskit.kit.errors.component_init", "The application failed to start because one of its components could not be initialized."),
		ErrConfig:        i18n.T("wailskit.kit.errors.config", "The application is misconfigured."),
	})
}

// componentErr wraps err as a kit construction failure, recording which
// component failed via the "component" field — see GetComponent. Returns
// nil if err is nil, so call sites can write
// `if err := componentErr("x", f()); err != nil { return nil, err }`
// unconditionally.
//
// This exists because "kit: construction failed: <opaque wrapped error>" is
// useless to a consumer whose config is wrong — the whole point of New
// composing a dozen packages in a fixed order is that when one of them
// rejects its options, the caller needs to know *which one* without reading
// New's source. See AGENTS.md, "Partial failure must be legible."
func componentErr(component string, err error) error {
	if err == nil {
		return nil
	}
	return kiterrors.Wrap(ErrComponentInit, fmt.Sprintf("kit: failed to initialize %s: %v", component, err), err).
		WithField("component", component)
}

// GetComponent extracts the component name componentErr recorded on err (or
// an error it wraps), e.g. "settings", "keyring", "updates". Returns ("",
// false) if err is not a kit construction failure — including nil, a plain
// error, or a *kiterrors.UserError with a different Code.
func GetComponent(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	var ue *kiterrors.UserError
	if !stderrors.As(err, &ue) {
		return "", false
	}
	if ue.Code != ErrComponentInit {
		return "", false
	}
	v, ok := ue.Fields["component"]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
