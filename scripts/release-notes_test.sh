#!/usr/bin/env bash
# Tests for release-notes.sh. Injects the upstream body from a fixture (offline).
# NOTE: `set -e` is intentionally omitted so every assertion runs and accumulates
# into `fail` rather than aborting on the first failure.
set -uo pipefail
# shellcheck disable=SC2164 # repo-root cd: relative script paths below fail loudly if this cd fails
cd "$(dirname "$0")/.."

FIXTURE="scripts/testdata/collector-releases-v0.154.0.txt"
SCRIPT="$(pwd)/scripts/release-notes.sh"
ABS_FIXTURE="$(pwd)/$FIXTURE"
fail=0

# Case A: no otlpinf commits (prev == HEAD -> empty git range -> fallback line)
out="$(UPSTREAM_BODY_FILE="$FIXTURE" scripts/release-notes.sh HEAD v0.154.0)"

assert_contains() {
  if printf '%s' "$out" | grep -qF -- "$1"; then echo "ok: contains '$1'";
  else echo "FAIL: missing '$1'"; fail=1; fi
}
assert_absent() {
  if printf '%s' "$out" | grep -qF -- "$1"; then echo "FAIL: should not contain '$1'"; fail=1;
  else echo "ok: absent '$1'"; fi
}

assert_contains "Check the [v0.154.0 contrib changelog]"
assert_contains "[v0.154.0 core changelog]"
assert_contains "## opentelemetry-infinity changes"
assert_contains "no otlpinf code changes"
assert_contains "## Embedded otelcol-contrib v0.154.0"
assert_contains "Remove deprecated JMX receiver"
assert_absent  "4ae7f285f3df635cb3c456f8dfba045973bb4d91"
assert_absent  "## Changelog"

# Case B: otlpinf has commits -> bullet list. Uses a throwaway repo so the test does
# NOT depend on the ambient checkout depth (HEAD~1 may not exist in a shallow clone).
tmp_repo="$(mktemp -d)"
(
  cd "$tmp_repo" || exit 1
  git init -q
  git config user.email test@example.com
  git config user.name test
  git commit -q --allow-empty -m "base"
  git commit -q --allow-empty -m "feat: add a thing"
)
out2="$(cd "$tmp_repo" && UPSTREAM_BODY_FILE="$ABS_FIXTURE" "$SCRIPT" HEAD~1 v0.154.0)"
rm -rf "$tmp_repo"
if printf '%s' "$out2" | grep -q '^• feat: add a thing '; then echo "ok: otlpinf commits listed";
else echo "FAIL: expected '• feat: add a thing' bullet"; fail=1; fi

# Case C: upstream fetch unavailable (missing fixture -> soft fallback, exit 0)
out3="$(UPSTREAM_BODY_FILE="/nonexistent/file" scripts/release-notes.sh HEAD v0.154.0)"
if printf '%s' "$out3" | grep -qF "could not be fetched"; then echo "ok: soft-fail fallback";
else echo "FAIL: expected soft-fail fallback"; fail=1; fi

# Case D: CRLF upstream body still curates correctly (regression guard for `tr -d '\r'`).
# Build a CRLF copy portably (awk ORS works on both GNU and BSD awk; `sed 's/$/\r/'` does not).
crlf_fixture="$(mktemp)"
awk 'BEGIN{ORS="\r\n"}1' "$FIXTURE" > "$crlf_fixture"
out4="$(UPSTREAM_BODY_FILE="$crlf_fixture" scripts/release-notes.sh HEAD v0.154.0)"
rm -f "$crlf_fixture"
header_count="$(printf '%s\n' "$out4" | grep -c 'for changelogs on specific components')"
if printf '%s' "$out4" | grep -qF "Remove deprecated JMX receiver" \
   && ! printf '%s' "$out4" | grep -qF "## Changelog" \
   && [ "$header_count" -eq 1 ]; then
  echo "ok: CRLF body curated (single header, deprecation kept, changelog stripped)";
else
  echo "FAIL: CRLF handling regression (header_count=$header_count)"; fail=1;
fi

if [[ $fail -ne 0 ]]; then echo "TESTS FAILED"; exit 1; fi
echo "ALL TESTS PASSED"
