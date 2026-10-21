#!/usr/bin/env bash
# Deletes the review output files when either contains a credential pattern.
# publish-review.sh then posts its "did not complete" stub and fails the job.
set -euo pipefail

PATTERN='sk-ant-[A-Za-z0-9_-]{10,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}'

for f in /tmp/summary-block.md /tmp/review-comments.json; do
  if [ -f "$f" ] && grep -Eq "$PATTERN" "$f"; then
    echo "::error::Review output contains a credential pattern; discarding it."
    rm -f /tmp/summary-block.md /tmp/review-comments.json
    exit 0
  fi
done
