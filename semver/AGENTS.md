# semver — agent notes

## Purpose

Semantic-version parsing and comparison. It exists as a standalone leaf so
consumers that need version ordering — `updates` for release selection,
`firstrun` for detecting upgrade/downgrade transitions — do not have to import
each other. It owns parsing, precedence, and comparison only. It does not own
version *discovery* (where a version string comes from), release feeds, or
update policy; those belong to `updates` and the consuming app.

Extracted from `updates/version.go` in WP-05. Behaviour was preserved exactly;
a review confirmed the original implementation correct including prerelease
precedence, so changes here should be additive rather than corrective.

## Public API (load-bearing signatures)

```go
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Raw        string // the input string as given, including any leading "v"
}

func ParseVersion(s string) (Version, error)
func (v Version) String() string
func (v Version) Compare(other Version) int   // -1, 0, +1
func (v Version) NewerThan(other Version) bool
```

## Invariants (do not break)

- **`Raw` preserves the input verbatim and takes no part in comparison.** It
  exists so a caller can echo back the tag it was given (`v1.2.3`) without
  reconstructing it. Never compare `Raw` values to decide ordering or equality.
- **A prerelease is lower-precedence than its release.** `2.0.0-beta.1` <
  `2.0.0`. Getting this backwards would offer users a downgrade from a stable
  release to a prerelease.
- **Numeric prerelease identifiers compare numerically, alphanumerics
  lexically**, and numeric always sorts below alphanumeric. `beta.9` <
  `beta.10` — string comparison alone gets this wrong.
- **A leading `v` is accepted on input and absent from `String()`.** Git tags
  carry it, semver does not. Round-tripping must not silently mutate a tag.
- **Parse failures are errors, never a zero Version.** A caller that treats an
  unparseable version as `0.0.0` would consider every release an upgrade.

## Dependencies & insulation

Standard library only. No kit packages, no `wails/v3` — this is a leaf and must
stay one. If it ever needs a kit dependency, that is a signal the logic belongs
in the caller instead. Not on the AD-4 Wails allowlist and must never be.

## Extension points

- Constraint/range matching (`^1.2.0`, `>=1.0 <2.0`) if a consumer ever needs
  it — additive, no signature changes required.
- A `MustParseVersion` for test and literal use, if the ergonomics justify it.

## Testing

`go test ./semver/ -race`. Pure functions, no doubles, no I/O, no network — a
table-driven suite is the whole story. Coverage should stay near 100%; there is
no excuse for uncovered branches in a package this shape.

Cases that must stay covered: prerelease ordering (including numeric vs
alphanumeric identifiers), build-metadata equality, `v` prefix round-trip,
and malformed input returning an error rather than a zero value.

## File map

- `version.go` — the `Version` type, parsing, precedence, comparison
- `version_test.go` — table-driven parse and comparison cases

## Landmines

- **`Compare` returning `0` does not mean the inputs were identical** — `v1.2.3`
  and `1.2.3` compare equal but have different `Raw`. Do not use
  `Compare(x) == 0` as a substitute for string equality when identity matters.
- **There is no `Build` field.** Build metadata (`1.0.0+abc`) is not modelled.
  If a consumer ever needs it, adding it must not change precedence — semver §10
  excludes build metadata from ordering.
- The precedence rules read as fussy but each one is load-bearing for update
  safety. Before "simplifying" a comparison branch, check which downgrade or
  spurious-upgrade scenario it prevents.
