#!/usr/bin/env bash
# Tests for release-notes.sh. Injects the upstream body from a fixture (offline).
set -uo pipefail
# shellcheck disable=SC2164 # repo-root cd: relative script paths below fail loudly if this cd fails
cd "$(dirname "$0")/.."

FIXTURE="scripts/testdata/collector-releases-v0.154.0.txt"
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

# Case B: otlpinf has commits (HEAD~1..HEAD -> at least one bullet)
out2="$(UPSTREAM_BODY_FILE="$FIXTURE" scripts/release-notes.sh HEAD~1 v0.154.0)"
if printf '%s' "$out2" | grep -q '^• '; then echo "ok: otlpinf commits listed";
else echo "FAIL: expected '• ' commit bullets"; fail=1; fi

# Case C: upstream fetch unavailable (missing fixture -> soft fallback, exit 0)
out3="$(UPSTREAM_BODY_FILE="/nonexistent/file" scripts/release-notes.sh HEAD v0.154.0)"
if printf '%s' "$out3" | grep -qF "could not be fetched"; then echo "ok: soft-fail fallback";
else echo "FAIL: expected soft-fail fallback"; fail=1; fi

if [[ $fail -ne 0 ]]; then echo "TESTS FAILED"; exit 1; fi
echo "ALL TESTS PASSED"
