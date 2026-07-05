#!/usr/bin/env bash
# Publishes a Claude PR review that was written to files by the review step.
# The model writes /tmp/summary-block.md (required) and, optionally,
# /tmp/review-comments.json; this script does every GitHub write. Publishing
# lives here — not in the model's sandbox — so it can never be permission-denied
# mid-run, and the sticky summary is guaranteed to post even when the review
# step errored, hit its turn cap, or produced nothing.
#
# Required env: GH_TOKEN, GH_REPO (owner/repo), PR_NUMBER, PR_HEAD_SHA.
set -euo pipefail

: "${GH_REPO:?}" "${PR_NUMBER:?}" "${PR_HEAD_SHA:?}"

MARKER="<!-- claude-code-review:summary -->"
END_MARKER="<!-- /claude-code-review:summary -->"
SUMMARY=/tmp/summary-block.md
COMMENTS=/tmp/review-comments.json
STATUS=0

# --- Sticky summary (always) ------------------------------------------------
if [ ! -s "$SUMMARY" ]; then
  # The review step produced no summary (errored, hit the turn cap, or a dead
  # key). Post a visible stub and fail the job so the miss is a red X, not a
  # silent green.
  RUN_URL="${GITHUB_SERVER_URL:-https://github.com}/${GH_REPO}/actions/runs/${GITHUB_RUN_ID:-}"
  {
    echo "> [!WARNING]"
    echo ">"
    echo "> **Review did not complete**"
    echo ">"
    echo "> The automated review produced no summary. See the [run](${RUN_URL})."
  } > "$SUMMARY"
  STATUS=1
fi

{
  echo "$MARKER"
  echo
  cat "$SUMMARY"
  echo
  echo "$END_MARKER"
} > /tmp/summary-marked.md

gh api "repos/${GH_REPO}/pulls/${PR_NUMBER}" --jq '.body // ""' > /tmp/pr-body.md || : > /tmp/pr-body.md

STICKY_MARKER="$MARKER" END_MARKER="$END_MARKER" python3 - <<'PY'
import os, re, pathlib
marker = os.environ["STICKY_MARKER"]
end    = os.environ["END_MARKER"]
body   = pathlib.Path("/tmp/pr-body.md").read_text()
block  = pathlib.Path("/tmp/summary-marked.md").read_text().rstrip() + "\n"
pattern = re.compile(re.escape(marker) + r".*?" + re.escape(end) + r"\s*", re.DOTALL)
body = pattern.sub("", body).rstrip()
new  = (body + "\n\n" + block) if body else block
pathlib.Path("/tmp/pr-body.new.md").write_text(new)
PY

gh api --method PATCH "repos/${GH_REPO}/pulls/${PR_NUMBER}" --field body=@/tmp/pr-body.new.md > /dev/null
echo "Sticky summary upserted into PR #${PR_NUMBER}."

# --- Inline comments (only when the model wrote findings) -------------------
if [ -s "$COMMENTS" ] && [ "$(jq 'length' "$COMMENTS" 2>/dev/null || echo 0)" -gt 0 ]; then
  # Resolve prior unresolved bot threads first, so a re-review doesn't stack
  # duplicate inline comments on top of the old ones.
  owner="${GH_REPO%/*}"
  repo="${GH_REPO#*/}"
  # shellcheck disable=SC2016  # $owner/$repo/$num are GraphQL variables, passed via -F
  gh api graphql -f query='
    query($owner:String!,$repo:String!,$num:Int!){
      repository(owner:$owner,name:$repo){
        pullRequest(number:$num){
          reviewThreads(first:100){ nodes{ id isResolved
            comments(first:1){ nodes{ author{ login } } } } } } } }' \
    -F owner="$owner" -F repo="$repo" -F num="$PR_NUMBER" \
    --jq '.data.repository.pullRequest.reviewThreads.nodes[]
          | select(.isResolved==false)
          | select(.comments.nodes[0].author.login | test("\\[bot\\]$|^github-actions|^claude$"))
          | .id' 2>/dev/null | while read -r tid; do
    [ -n "$tid" ] || continue
    # shellcheck disable=SC2016  # $id is a GraphQL variable, passed via -F
    gh api graphql -f query='
      mutation($id:ID!){ resolveReviewThread(input:{threadId:$id}){ thread{ id } } }' \
      -F id="$tid" > /dev/null || true
  done

  jq -n --arg sha "$PR_HEAD_SHA" --slurpfile c "$COMMENTS" \
    '{commit_id:$sha, event:"COMMENT", comments:$c[0]}' > /tmp/review.json
  if gh api -X POST "repos/${GH_REPO}/pulls/${PR_NUMBER}/reviews" --input /tmp/review.json > /dev/null; then
    echo "Posted $(jq 'length' "$COMMENTS") inline comment(s)."
  else
    echo "::warning::Inline review POST failed (line anchors may be stale); summary still posted."
  fi
fi

exit "$STATUS"
