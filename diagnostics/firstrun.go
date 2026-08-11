package diagnostics

import (
	"archive/zip"
	"encoding/json"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/firstrun"
)

// WithFirstRun includes a firstrun.json file in the bundle: the version
// transition (fresh/upgrade/downgrade/same) svc.Detect() reports as of
// CreateBundle time. Detect is read-only — it runs no hooks and writes no
// stamp (see firstrun.Service.Detect) — so this reflects the on-disk stamp
// freshly every time, not a value cached from whenever Run last executed.
// Optional: with no WithFirstRun, firstrun.json is simply absent from the
// bundle.
//
// Unlike WithHealth, this content is low-risk: version numbers, not
// endpoints or credentials. It's still worth a support engineer seeing —
// "user is on 2.3.0, upgraded from 2.1.0" often explains a support ticket
// by itself — so, unlike health.json's Err field, nothing here is redacted.
func WithFirstRun(svc *firstrun.Service) ServiceOption {
	return func(s *Service) { s.firstrun = svc }
}

func (s *Service) writeFirstRun(zw *zip.Writer) error {
	info, err := s.firstrun.Detect()
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "detect firstrun transition", err)
	}

	previous := ""
	if info.Kind != firstrun.Fresh {
		previous = info.Previous.String()
	}
	payload := firstrun.TransitionPayload{
		Kind:     info.Kind,
		Previous: previous,
		Current:  info.Current.String(),
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "marshal firstrun info", err)
	}

	w, err := zw.Create("firstrun.json")
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "create firstrun.json in zip", err)
	}
	_, err = w.Write(data)
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "write firstrun.json", err)
	}
	return nil
}
