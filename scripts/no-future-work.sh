#!/usr/bin/env bash
#
# no-future-work.sh - Fail if a doc uses future-work framing: a "not
# covered yet"-style heading, or roadmap/TODO phrasing about unfinished
# work on the doc's own subject. Docs describe what's true right now.
#
# ADRs and roadmap files are the deliberate exception (see EXCLUDE_PATHSPECS
# below) — discussing what's deferred is their actual job, not a doc smell.
#
# Usage:
#   scripts/no-future-work.sh          # lint every tracked markdown file
#   scripts/no-future-work.sh <files>  # lint specific files

set -euo pipefail

EXCLUDE_PATHSPECS=(
  ':!docs/adr/**'
  ':!docs/adrs/**'
  ':!**/roadmap-*.md'
  ':!.claude/**'
  ':!**/.terraform/**'
)

# Heading-level banners that announce unfinished work by name. A heading is
# an unambiguous signal — "here is what this page doesn't do yet" — unlike
# inline "not yet", which also shows up describing real runtime state (an
# IP not yet assigned mid-apply, a kustomization not yet deployed mid-diff)
# that has nothing to do with unfinished documentation.
HEADING_PATTERN='^#+ *(Not (Covered|Built|Implemented|Supported|Decided) Yet|Coming Soon|Future Work|Roadmap|TODO|Todo)\b'

# Inline phrases proven, in practice, to be future-work framing rather than
# a description of current state. Deliberately narrow — see HEADING_PATTERN
# comment above for why a bare "not yet" isn't on this list.
PHRASE_PATTERN='planned follow-up|on the roadmap|not yet a component|target service set|is a real follow-up, not decided|coming soon|not yet built|not covered here yet|not covered yet'

files=()
if [ "$#" -gt 0 ]; then
  files=("$@")
else
  while IFS= read -r f; do files+=("$f"); done < <(git ls-files -- '*.md' "${EXCLUDE_PATHSPECS[@]}")
fi

status=0
for f in "${files[@]}"; do
  [ -f "$f" ] || continue
  hits="$(grep -nEi "$HEADING_PATTERN" "$f" || true)"
  if [ -n "$hits" ]; then
    echo "error: $f has a future-work heading. Docs describe the current state — cut the section, or fold any real current-state fact into prose without the future framing." >&2
    echo "$hits" >&2
    status=1
  fi
  hits="$(grep -nEi "$PHRASE_PATTERN" "$f" || true)"
  if [ -n "$hits" ]; then
    echo "error: $f uses future-work phrasing. Docs describe the current state, not planned work." >&2
    echo "$hits" >&2
    status=1
  fi
done

exit $status
