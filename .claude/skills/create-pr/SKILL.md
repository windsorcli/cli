---
name: create-pr
description: Push the current branch and open or update its pull request with a Conventional-Commits title. Does NOT author a PR description -- the claude-code-review workflow upserts the canonical summary into the body on every push, so this skill leaves the body empty (except for a Closes #N issue-link line when the branch resolves an issue) for CI to fill and never touches an existing one. Run after committing and before announcing the PR. Use whenever the user asks to "open the PR", "push and PR", "create a branch and PR", or after the project's gates are green and the branch is ready.
---

# Create or Update PR

Open a pull request for the current branch with a Conventional-Commits
title and an empty body. The repo's `claude-code-review` workflow upserts
the canonical summary into the PR body on every push, so this skill never
authors a description: it creates the PR with no body and leaves the body
for CI, and it never overwrites a body that already exists.

## Apply when
- The user says "open the PR", "push the branch and PR", "make a PR", or
  similar after work is committed.
- The project's gates (lint + tests) have passed locally and the branch
  has commits ahead of `main`.
- Skip if `git status -s` shows uncommitted changes that should be in the
  PR -- ask the user to commit or stash first.

## Inputs to gather
1. `git rev-parse --abbrev-ref HEAD` — branch name.
2. `git log main..HEAD --oneline` — commits the PR contains.
3. `git diff main...HEAD --stat` — file-level shape of the change.
4. `gh pr view --json number 2>/dev/null` — does a PR exist already?
5. The issue this branch resolves, if any. Check the commit messages and
   branch name for an issue number, and the conversation for one you were
   working from. If a single issue is clearly the one being fixed, use it;
   if it's ambiguous or there might be several, ask the user rather than
   guess. A wrong `Closes #N` closes the wrong issue on merge.

If `gh pr view` returns "no pull requests found", we'll create a new PR.
Otherwise the PR already exists and we leave its body alone.

## Title rules

Hard requirements:
- **Conventional Commits shape**: `<type>(<scope>?): <description>`.
- **Types**: `feat`, `fix`, `chore`, `docs`, `test`, `refactor`, `ci`,
  `perf`, `build`. Pick from this fixed set.
- **Scope** (optional, lowercase): the package or area touched, e.g. the
  directory or component name, or `deps` for dependency bumps. Single
  word; multi-word scopes are a smell. Sample existing scopes with
  `git log main --pretty=format:%s | head -20`.
- **Description**: lowercase first letter, imperative or descriptive,
  no trailing punctuation.
- **Length**: aim for ≤ 65 chars total. Hard cap at 72.
- **Same shape as commit titles** in the project — sample
  `git log main --pretty=format:%s | head -20` to verify.

Examples that fit: `feat(api): inline default timeout`,
`fix(server): bound request ctx with command timeout`,
`chore(deps): bump some-dependency to v1.20`.

Anti-patterns to avoid:
- ALL CAPS prefixes ("M4: ..."): drop the milestone tag.
- Multi-clause titles with "and" or `+`: pick the bigger half. If the
  change really is two things, that's a sign it should be two PRs.
- Verbose nouns ("complete the foo resource"): use the verb
  ("complete foo").

## Body

Do not write a PR summary. The repo's `claude-code-review` workflow upserts
the canonical summary (a `> [!NOTE]` block delimited by
`<!-- claude-code-review:summary -->` markers) into the PR description on
every push, so authoring one here would only duplicate or fight with it.

The one thing that *does* belong in the body is a closing keyword when the
branch resolves an issue: a single `Closes #N` line (`Fixes #N` / `Resolves
#N` work too). GitHub's auto-close only fires from a keyword in the PR body
or a commit message — never the title — so without this line the issue stays
open and has to be closed by hand after merge. Nothing else goes in the body.

This is safe against CI: `.github/scripts/publish-review.sh` reads the
existing body, strips only its own delimited marker block, and re-appends the
summary *after* whatever prose remains (`new = body + "\n\n" + block`). A
`Closes #N` line at the top survives every push and stays intact through
merge. The empty-body default is unchanged — it's just `Closes #N` when there
is an issue, empty when there isn't.

If a PR already exists, leave its body untouched — CI owns it, and a human
may have added prose of their own that must not be clobbered. Do not retrofit
a `Closes #N` line into an existing body; if an already-open PR needs to
close an issue, tell the user so they can add the line or close the issue on
merge themselves.

## Decision tree

```
Does a PR exist for this branch?
├── No  → gh pr create with generated title. Body is "Closes #N" when the
│         branch resolves an issue, empty otherwise. Then print URL.
└── Yes → Leave the body alone (CI owns the summary; a human may have added
          prose). Print URL only.
```

After the PR exists, **always check CI status** (see below). The point
of opening a PR is to get the change reviewed and merged; surfacing a
red check immediately lets the user fix it before they walk away.

## Commands

Push first, then create the PR. The body is `Closes #N` when the branch
resolves an issue, empty otherwise. Pass `--body` explicitly so `gh` does
not drop into an interactive editor.

```bash
# 1. Push (set upstream on first push of this branch)
git push -u origin "$(git rev-parse --abbrev-ref HEAD)"

# 2. Detect existing PR
PR_NUM=$(gh pr view --json number --jq '.number // empty' 2>/dev/null)

# 3. Create when none exists; never touch an existing body.
#    Set BODY to "Closes #N" if the branch resolves an issue, else "".
if [ -z "$PR_NUM" ]; then
  BODY="Closes #1234"   # or "" when no issue is resolved
  gh pr create --title "<generated title>" --body "$BODY"
  PR_NUM=$(gh pr view --json number --jq .number)
else
  echo "PR #$PR_NUM already exists; leaving its body to CI."
fi
gh pr view --json url --jq .url
```

If `gh` returns `HTTP 401: Bad credentials`, the operator has a stale
`GITHUB_TOKEN` env var overriding the keychain. Prepend `unset GITHUB_TOKEN`
to the failing command and retry.

## CI status check

After the PR exists (just created OR already-existed), run a quick
status check. CI typically kicks off within a few seconds of the push,
but most pipelines take 1-5 minutes to complete. Two modes:

```bash
# Snapshot: list current state of every check, no waiting.
gh pr checks "$PR_NUM"
```

If any row shows `fail` or `failure`, surface those rows to the user
verbatim and direct them to the failing job's URL (the rightmost
column of `gh pr checks`). Don't try to diagnose the failure from the
skill -- the failing job's logs are the source of truth.

If every row shows `pending` or `queued`, that's expected on a fresh
push. Print the URL with a "checks running" hint. Do NOT block the
skill on completion -- the user can run `gh pr checks --watch` to
follow them.

If every row shows `pass`, say so explicitly. The user shouldn't have
to scroll back to verify.

## What NOT to do
- Don't author a PR summary. The `claude-code-review` workflow owns the
  description; anything beyond a `Closes #N` line duplicates or fights with
  it. The closing keyword is the only prose the skill writes.
- Don't guess the issue number for `Closes #N`. Confirm it or ask; a wrong
  number closes the wrong issue on merge.
- Don't overwrite or edit the body of an existing PR — including to add a
  `Closes #N` line. Hand that back to the user.
- Don't push to `main` directly. Always operate on a feature branch.
- Don't run `git push --force` unless the user explicitly asked.

## After posting

Print the PR URL on its own line so the operator can click through.
