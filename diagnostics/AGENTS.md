# diagnostics — agent notes

## Purpose

`diagnostics` collects application state, logs, and system info into a
shareable zip bundle for crash reporting and user support (`CreateBundle`),
and — separately, consent-gated — uploads that bundle to a remote endpoint
(`Submit`). It owns bundle assembly and the upload transport; it does not
own log redaction *content* (the consuming app's log lines go in verbatim —
see README "Security"), and it does not own the crash-sentinel/auto-report
decision logic that will eventually sit on top of `SubmissionConsent()`
(Phase 2, not this package).

## Public API (load-bearing signatures)

```go
func NewService(opts ...ServiceOption) (*Service, error)
func (s *Service) CreateBundle(ctx context.Context, outputDir string) (string, error)
func (s *Service) GetSystemInfo() SystemInfo

// Consent (consent.go)
const SettingConsent = "diagnostics.submission_consent" // default: false
func SettingsGroup() settings.Group
func (s *Service) SubmissionConsent() bool

// Upload (webhook.go) — Submit is the ONLY exported upload entry point.
func (s *Service) Submit(ctx context.Context, bundlePath, webhookURL string) error
// error code diagnostics.ErrConsentRequired when consent is absent.

func RecoverAndLog(svc *Service) func() // defer'd panic-to-crash-log helper

// Optional bundle content (health.go, firstrun.go) — absent option means
// the file is absent from the bundle, never present-but-empty.
func WithHealth(r *health.Registry) ServiceOption   // -> health.json, Err field omitted
func WithFirstRun(svc *firstrun.Service) ServiceOption // -> firstrun.json
```

## Invariants (do not break)

- **`Submit` is the only exported way to upload a bundle.** There is no
  consent-bypassing method. If you add a new upload path, it must call
  through the same consent check — never wire a new method straight to
  `submitBundle`/`doSubmit`.
- **Consent defaults to off, always.** `SubmissionConsent()` returns `false`
  when: no settings service is wired, the key is unset, `GetValues` errors,
  or the stored value isn't a `bool`. Every ambiguous case resolves to "no
  consent" — never to "assume yes."
- **`Submit` returns a typed, non-nil error when consent is absent** —
  `errors.Code = ErrConsentRequired`. Never make this a silent no-op; a
  silent no-op is indistinguishable from a broken uploader.
- **TLS/localhost enforcement happens before any network call.**
  `validateWebhookURL` rejects everything except `https://` and
  `http://localhost|127.0.0.1|::1` — checked in `submitBundle` before the
  retry loop starts, so a rejected URL never dials.
- **Webhook requests never follow redirects.** `webhookClient()` always sets
  `CheckRedirect` to `http.ErrUseLastResponse`. Don't relax this without
  re-deriving `validateWebhookURL` for the redirect target too — the whole
  point is that a compromised/misconfigured endpoint can't 3xx its way past
  the TLS/localhost check.
- **Bundle file mode is `0600`**, written via
  `os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)` — never
  `os.Create` (which yields `0644` before umask). The output directory being
  `0700` does not substitute for this; users move the file out of it.
- **Response bodies are never logged**, and no error message anywhere in
  this package includes response-body or bundle-file *content* — only
  status codes, URLs/hosts, and transport-level error text. Keep it that
  way; bundles can contain user content (e.g. LLM prompt logs in Prune).
- **Retries are bounded and backoff is capped.** `maxRetries` (default 3)
  bounds attempts; backoff is capped at `maxWebhookBackoff` (30s) regardless
  of how large a caller sets `WithWebhookMaxRetries`. Don't remove the cap.
- **`sanitizeSettings` only redacts `settings.FieldPassword` fields.** This
  is a known, documented gap (README "Security"), not a bug to silently
  "complete" by guessing at other sensitive-looking keys — that would be a
  false sense of coverage. If you want broader redaction, it needs a
  deliberate design (e.g. a field-level "sensitive" flag in `settings`),
  not a heuristic bolted on here.
- **`health.json` never includes `health.CheckStatus.Err`.** That string
  frequently embeds the full probed URL — including any query string — via
  net/http's raw `*url.Error` (see `health/httpprobe.go` +
  `health/schedule.go`). `writeHealth` (health.go) maps `CheckStatus` to the
  local `healthCheckSummary` type, which has no `Err`/error field at all —
  only a `HadError bool`. Don't "restore" the error text by adding an `Err
  string` field back, even redacted-looking (see README "Security" for why
  a partial-redaction heuristic here would be the same false-coverage
  mistake as `sanitizeSettings`'s scope, just unacknowledged).
- **`WithHealth`/`WithFirstRun` are read-time, not cached.** `writeHealth`
  calls `r.Snapshot()` and `writeFirstRun` calls `svc.Detect()` fresh on
  every `CreateBundle`, not once at `NewService`/option-application time —
  `firstrun.Service.Detect` in particular is documented read-only (no hooks
  run, no stamp written), which is exactly why it's safe to call here
  instead of threading a static `Info` value through.

## Dependencies & insulation

- `appdirs` — optional, for resolving the log directory (`WithDirs`).
- `settings` — optional (`WithSettings`); source of both the sanitized
  `settings.json` in the bundle and the consent value `Submit` reads. This
  package must stay Wails-free (AD-4 allowlist) — no `wails/v3` import here.
- `health` — optional (`WithHealth`); source of `health.json`
  (`health.go`). Wails-free, like `settings` — safe to depend on.
- `firstrun` — optional (`WithFirstRun`); source of `firstrun.json`
  (`firstrun.go`). Also Wails-free.
- `events` — optional (`WithEmitter`); emits `diagnostics:bundle_created` /
  `diagnostics:bundle_submitted`. Nil emitter is a no-op via `s.emit`.
- `errors` — every failure is an `errors.Code`-tagged `*errors.UserError`,
  registered in `diagnostics.go`'s `init()`.
- Stdlib only otherwise (`archive/zip`, `net/http`, `mime/multipart`, …) —
  zero *external* (non-kit) dependencies is a stated property in the
  README; `health`/`firstrun` are in-repo kit packages, not external ones,
  so this still holds.

## Extension points

- New bundle content sources: `WithCustomCollector(name, fn)` — output lands
  at `collectors/{name}` in the zip; failures are skipped silently (that's
  existing, intentional behavior — collectors must not be able to abort
  bundle creation). Prefer this over a new dedicated `With*` option +
  `write*` method unless the source is a first-class kit package the way
  `health`/`firstrun` are.
- New webhook behavior (headers, auth schemes): extend `ServiceOption`s in
  `diagnostics.go` (`WithWebhookToken` etc.) and thread through `doSubmit`,
  not by adding a second upload method.
- Composing consent elsewhere (e.g. the future crash-sentinel): call
  `SubmissionConsent()`, don't re-read `SettingConsent` from
  `settings.GetValues()` directly — keep the "no settings service / bad
  value → false" logic in one place.
- A new dedicated bundle-content option (like `WithHealth`/`WithFirstRun`):
  give it its own `<concern>.go` file (mirroring `health.go`/`firstrun.go`),
  a nil-checked field on `Service`, a numbered step in `CreateBundle`
  (`diagnostics.go`) that only fires when the field is non-nil, and decide
  redaction explicitly — don't assume a new source's serialized form is
  automatically bundle-safe (see the `health.json` Err-omission precedent).

## Testing

- Doubles: `events.NewMemoryEmitter()` + `events.NewEmitter(...)`,
  `keyring.NewMemoryStore()`, `settings.NewService(...)` against a
  `t.TempDir()` store path, `httptest.NewServer` for every webhook test,
  `health.New(health.WithoutDefaultConnectivityCheck())` + a local
  `failingProbe`/no-network `Probe` for `health.json` tests,
  `firstrun.New(firstrun.WithStoragePath(...), firstrun.WithVersion(...))`
  for `firstrun.json` tests. **No test may dial a real host** —
  `validateWebhookURL`-rejection tests assert on the error before any
  request would be sent; redirect tests redirect to an address
  (`127.0.0.1:1`) that must never actually be dialed, verified by asserting
  exactly 1 attempt reached the origin server; health tests use a fake
  `Probe` whose "error" is a synthetic string containing a URL/token, never
  an actual dial.
- `go test -race ./diagnostics/...` must stay green and cover: consent
  absent → refusal (`TestSubmitRefusesWithoutConsent`,
  `TestSubmitRefusesWithoutSettingsService`), consent present → success
  against `httptest` (`TestSubmitSucceedsWithConsent`), plaintext-endpoint
  rejection (`TestValidateWebhookURL`, `TestSubmitRejectsPlaintextEndpoint`),
  redirect refusal (`TestSubmitBundleDoesNotFollowRedirects`), upload-path
  redaction verified on the actual wire bytes, not the on-disk file
  (`TestUploadPathRedactsSecrets`), bundle file mode
  (`TestCreateBundle/bundle_file_is_created_0600,_not_0644`),
  `health.json` present with `WithHealth` and absent without it
  (`TestBundleIncludesHealth`, `TestBundleWithoutHealth_OmitsFile`) —
  including the negative assertion that a probe error's URL/query
  string/host never appear anywhere in the bundle bytes — and
  `firstrun.json` present with `WithFirstRun` and absent without it
  (`TestBundleIncludesFirstrun`, `TestBundleIncludesFirstrun_Fresh`,
  `TestBundleWithoutFirstrun_OmitsFile`).
- Not automatable here: real crash-reporting-endpoint behavior (rate
  limiting, WAF quirks) — that's an integration concern for the consuming
  app, not this package's suite.

## File map

- `diagnostics.go` — `Service`, `ServiceOption`s, error codes/messages,
  `CreateBundle` and its `writeSystemInfo`/`writeSettings`/`writeLogs`/
  `writeCollectors`/`writeManifest` helpers, `sanitizeSettings`.
- `consent.go` — `SettingConsent`, `SettingsGroup()`, `SubmissionConsent()`.
- `webhook.go` — `Submit` (consent-gated entry point), `submitBundle`
  (retry/backoff loop, unexported), `doSubmit` (single HTTP attempt),
  `webhookClient` (redirect policy), `validateWebhookURL`/`isLoopbackHost`
  (TLS/localhost policy).
- `panic.go` — `RecoverAndLog`.
- `health.go` — `WithHealth`, `healthSummary`/`healthCheckSummary` (the
  Err-omitting bundle shape), `writeHealth`.
- `firstrun.go` — `WithFirstRun`, `writeFirstRun`.
- `*_test.go` — `diagnostics_test.go` (bundle assembly), `consent_test.go`
  (consent gate + `SettingsGroup`), `webhook_test.go` (retry/backoff/timeout
  transport mechanics via the unexported `submitBundle`),
  `webhook_security_test.go` (the audit regression suite: TLS, redirects,
  upload-path redaction), `panic_test.go`, `health_test.go`
  (`TestBundleIncludesHealth` + the Err-omission negative assertions),
  `firstrun_test.go` (`TestBundleIncludesFirstrun` + fresh-install variant).

## Landmines

- `webhook_test.go` calls `svc.submitBundle(...)` directly (lowercase) to
  exercise retry/backoff/timeout mechanics without also having to wire a
  consenting settings service into every table-driven case. That's
  intentional test-package internal access, not an oversight — don't
  "fix" it by exporting `SubmitBundle` again; that would reopen the
  consent-bypass hole `Submit` exists to close.
- `SubmissionConsent()` treats *any* non-bool stored value as "no consent"
  rather than erroring. This is deliberate (fail closed), but means a
  type-mismatched settings migration would silently disable submission
  rather than surface loudly — acceptable given the alternative is failing
  open on a consent gate.
- The webhook client is rebuilt per call (`webhookClient()`), cloning
  `Transport`/`Jar`/`Timeout` off `s.httpClient` (or `http.DefaultClient`)
  but always overriding `CheckRedirect`. If a caller passes `WithHTTPClient`
  with a custom `CheckRedirect`, it is silently overridden — this is
  intentional (redirect-refusal is a security invariant, not a caller
  option) but worth knowing if a test's custom client behaves unexpectedly.
- `maxDrainBytes` (64KiB) caps how much of a response body is read before
  discarding — a hostile endpoint sending more than that just gets the
  connection's remainder left undrained. Fine for reuse-optimization
  purposes; don't mistake it for a security boundary on response size.
