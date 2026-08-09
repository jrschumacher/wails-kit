package diagnostics

import (
	"archive/zip"
	"encoding/json"
	"time"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/health"
)

// WithHealth includes a health.json file in the bundle, built from r's
// current Snapshot() at CreateBundle time. Optional: with no WithHealth,
// health.json is simply absent from the bundle — never present-but-empty,
// matching WithSettings/WithLogDir's "unset option -> absent file"
// convention elsewhere in this package.
//
// health.json deliberately omits health.CheckStatus.Err. A probe failure's
// error string frequently embeds the full probed URL, including any query
// string: health/httpprobe.go's HTTPProbe surfaces net/http's raw
// *url.Error unmodified on failure, whose Error() renders as
// `Get "<url>": <cause>` — and health/schedule.go stores exactly that
// string into CheckStatus.Err. For an app whose health checks point at
// internal or semi-private endpoints (or an endpoint URL carrying an auth
// token in its query string, e.g. a signed status-page URL), repeating that
// text in a bundle destined for a third-party support endpoint (see README
// "Security") would leak it. There is no reliable way to redact an
// arbitrary Go error string down to "safe" without a heuristic that
// silently misses cases — this package already avoids exactly that kind of
// false-coverage elsewhere (see sanitizeSettings' doc comment) — so the
// honest choice is to omit the error text rather than guess at scrubbing
// it. HadError still reports whether a check is currently failing, without
// repeating why; Name/Class/State/Critical/CheckedAt/Latency are all
// developer-chosen labels or structural/timing data, not derived from a
// probed URL, and are kept as-is.
func WithHealth(r *health.Registry) ServiceOption {
	return func(s *Service) { s.health = r }
}

// healthSummary is health.Snapshot's bundle shape: everything except
// per-check Err text (see WithHealth).
type healthSummary struct {
	Overall health.State         `json:"overall"`
	Offline bool                 `json:"offline"`
	Checks  []healthCheckSummary `json:"checks"`
}

// healthCheckSummary is health.CheckStatus's bundle shape.
type healthCheckSummary struct {
	Name      string        `json:"name"`
	Class     health.Class  `json:"class"`
	State     health.State  `json:"state"`
	Critical  bool          `json:"critical"`
	HadError  bool          `json:"hadError"`
	CheckedAt time.Time     `json:"checkedAt"`
	Latency   time.Duration `json:"latencyNs"`
}

func (s *Service) writeHealth(zw *zip.Writer) error {
	snap := s.health.Snapshot()

	summary := healthSummary{
		Overall: snap.Overall,
		Offline: snap.Offline,
		Checks:  make([]healthCheckSummary, len(snap.Checks)),
	}
	for i, c := range snap.Checks {
		summary.Checks[i] = healthCheckSummary{
			Name:      c.Name,
			Class:     c.Class,
			State:     c.State,
			Critical:  c.Critical,
			HadError:  c.Err != "",
			CheckedAt: c.CheckedAt,
			Latency:   c.Latency,
		}
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "marshal health snapshot", err)
	}

	w, err := zw.Create("health.json")
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "create health.json in zip", err)
	}
	_, err = w.Write(data)
	if err != nil {
		return errors.Wrap(ErrBundleCreate, "write health.json", err)
	}
	return nil
}
