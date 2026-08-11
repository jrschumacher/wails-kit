#!/usr/bin/env bash
#
# AD-4 enforcement: only an explicit allowlist of packages may import wails/v3.
#
# Rationale (docs/v2-roadmap.md, AD-4): the allowlisted packages exist *to be* Wails
# integration; wrapping them behind interfaces would be abstraction theater. Every other
# package must run in a non-GUI process (Prune's CLI/TUI) or is pure logic, so a Wails
# import there is a build-graph bug. Packages that need OS/GUI signals but are not
# allowlisted define a small source interface and receive the Wails-backed
# implementation from kit/wailsbridge.
#
# Run from the repo root:  .github/scripts/check-wails-imports.sh
#
# Portable to bash 3.2 (macOS default) — no mapfile, no associative arrays.

set -euo pipefail

MODULE="github.com/jrschumacher/wails-kit/v2"

# The packages permitted to import github.com/wailsapp/wails/v3 (AD-4).
# Exact full import paths — no prefix matching, no wildcards, so a future
# .../foo/shortcuts is not silently exempted.
ALLOWED_PKGS="
shortcuts
windowstate
permissions
kit/wailsbridge
"

ALLOWED=""
for p in $ALLOWED_PKGS; do
	ALLOWED="${ALLOWED}
${MODULE}/${p}"
done

# Everything under examples/ is exempt.
#
# AD-4's invariant is about *library* code: a consumer importing
# wails-kit/v2/settings must not acquire Wails. Examples are main programs
# that nobody imports, so an example importing Wails costs a consumer
# nothing — and a meaningful example for a GUI package has to construct a
# real app and window, so forbidding it would only produce examples that
# don't demonstrate the thing.
#
# This previously derived "examples/<allowlisted-pkg>", which assumed example
# directories mirror package import paths. They don't: examples/kit-gui
# demonstrates kit/wailsbridge. That coupling made the check fail on a
# correctly-written example, which is the wrong failure — a policy check that
# blocks correct code trains people to weaken the policy.
examples_prefix="${MODULE}/examples/"

is_allowed() {
	local pkg="$1" a
	case "$pkg" in
	"${examples_prefix}"*) return 0 ;;
	esac
	for a in $ALLOWED; do
		[ "$pkg" = "$a" ] && return 0
	done
	return 1
}

# -tags wails so the check still sees shortcuts/apply.go while it remains behind the
# legacy build tag (WP-06 deletes that tag; the flag is harmless afterwards).
# Imports, TestImports and XTestImports are all considered: AD-4 says a package either
# may import wails/v3 or must not import it at all, tests included.
offenders=$(
	go list -tags wails -f \
		'{{.ImportPath}}{{range .Imports}} {{.}}{{end}}{{range .TestImports}} {{.}}{{end}}{{range .XTestImports}} {{.}}{{end}}' \
		./... |
		awk '{ for (i = 2; i <= NF; i++) if ($i ~ /^github\.com\/wailsapp\/wails\/v3(\/|$)/) { print $1; break } }' |
		sort -u
)

violations=""
for pkg in $offenders; do
	if ! is_allowed "$pkg"; then
		violations="${violations}${pkg}
"
	fi
done

if [ -n "$violations" ]; then
	{
		echo "AD-4 violation: these packages import github.com/wailsapp/wails/v3 but are not on the allowlist:"
		printf '%s' "$violations" | sed 's/^/  /'
		echo
		echo "Allowed packages are:"
		printf '%s\n' $ALLOWED | sed 's/^/  /'
		echo
		echo "Fix by defining a narrow interface in the package and supplying the"
		echo "Wails-backed implementation from kit/wailsbridge (see AD-4)."
	} >&2
	exit 1
fi

echo "AD-4 OK: no non-allowlisted package imports wails/v3."
