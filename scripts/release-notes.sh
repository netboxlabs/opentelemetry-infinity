#!/usr/bin/env bash
#
# Assemble the GitHub Release body for an opentelemetry-infinity release.
#
# Usage:
#   scripts/release-notes.sh <prev_version> <new_version>
#   e.g. scripts/release-notes.sh v0.153.0 v0.154.0
#
# Output: assembled Markdown on stdout.
#
# Env:
#   GH_TOKEN            token used by `gh api` to fetch the upstream release notes.
#   UPSTREAM_BODY_FILE  (test only) read the upstream collector-releases body from this
#                       file instead of calling the GitHub API.
#
set -euo pipefail

PREV="${1:-}"
NEW="${2:-}"

if [[ -z "$PREV" || -z "$NEW" ]]; then
  echo "usage: $0 <prev_version> <new_version>" >&2
  exit 2
fi

CONTRIB_URL="https://github.com/open-telemetry/opentelemetry-collector-contrib/releases/tag/${NEW}"
CORE_URL="https://github.com/open-telemetry/opentelemetry-collector/releases/tag/${NEW}"

# 1. Header line (constructed by us; robust to upstream format drift).
printf 'Check the [%s contrib changelog](%s) and the [%s core changelog](%s) for changelogs on specific components.\n' \
  "$NEW" "$CONTRIB_URL" "$NEW" "$CORE_URL"

# 2. opentelemetry-infinity changes (git log since the previous release).
printf '\n## opentelemetry-infinity changes\n'
otlpinf_changes="$(git log "${PREV}..HEAD" --pretty=format:'• %s [%an]' 2>/dev/null || true)"
if [[ -n "$otlpinf_changes" ]]; then
  printf '%s\n' "$otlpinf_changes"
else
  printf '_Embedded otelcol-contrib upgraded %s → %s; no otlpinf code changes._\n' "$PREV" "$NEW"
fi

# 3. Embedded otelcol-contrib upstream notes.
fetch_upstream() {
  if [[ -n "${UPSTREAM_BODY_FILE:-}" ]]; then
    [[ -f "$UPSTREAM_BODY_FILE" ]] || return 1
    cat "$UPSTREAM_BODY_FILE"
    return
  fi
  # `// ""` => a null/absent body becomes an empty string, not the literal "null".
  gh api "repos/open-telemetry/opentelemetry-collector-releases/releases/tags/${NEW}" --jq '.body // ""'
}

# tr -d '\r' normalizes CRLF so the `$`-anchored seds below match regardless of
# upstream line endings. pipefail makes a failed fetch propagate to the `if`.
if ! upstream_raw="$(fetch_upstream 2>/dev/null | tr -d '\r')"; then
  upstream_raw=""
fi

printf '\n## Embedded otelcol-contrib %s\n' "$NEW"
if [[ -z "$upstream_raw" ]]; then
  printf '_Upstream release notes for %s could not be fetched. See the changelog links above._\n' "$NEW"
else
  # Drop upstream's leading "Check the ... changelog" line (we built our own),
  # strip from "## Changelog" to EOF (raw commit-hash list = noise),
  # drop the redundant "## <version>" heading, then trim leading blank lines.
  # Escape regex metacharacters (dots) in the version before using it in a sed pattern.
  new_re=${NEW//./\\.}
  upstream_curated="$(printf '%s\n' "$upstream_raw" \
    | sed '/^Check the .*for changelogs on specific components\.$/d' \
    | awk 'BEGIN{keep=1} /^## Changelog[[:space:]]*$/{keep=0} keep' \
    | sed "/^## ${new_re}[[:space:]]*\$/d" \
    | awk 'NF{f=1} f')"
  if [[ -n "${upstream_curated//[[:space:]]/}" ]]; then
    printf '%s\n' "$upstream_curated"
  else
    printf '_No curated upstream notes for this version. See the changelog links above._\n'
  fi
fi
