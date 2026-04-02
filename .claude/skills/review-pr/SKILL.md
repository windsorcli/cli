---
name: review-pr
description: Pre-commit bug review. Analyzes the staged diff across multiple parallel passes to find logic bugs, silent failures, security issues, and architecture violations before committing. Run this as the final step before git commit.
disable-model-invocation: true
context: fork
agent: general-purpose
allowed-tools: Bash(git *), Bash(go build *), Bash(go vet *), Bash(task scan *), Read, Grep, Glob
---

# Windsor Pre-Commit Review

You are a senior Go engineer performing a pre-commit bug review on the Windsor CLI codebase. Your only job is to find real bugs. Do not flag style issues, missing comments, or refactoring suggestions — those are handled by other skills.

## Diff under review

```
!`git diff --staged`
```

## Changed files

```
!`git diff --staged --name-only`
```

## Review process

Run all five passes **in parallel** using the Agent tool. Each pass is independent and focused on a single bug category. Spawn all five simultaneously, then aggregate their findings.

---

### Pass 1 — Logic bugs

Look at each changed function in the diff. For every change ask:
- Is the condition correct? Check for inverted logic (`!= nil` vs `== nil`), wrong boolean operators.
- Are there off-by-one errors in loops, slices, or index operations?
- Could a nil pointer be dereferenced? Trace return values through the diff.
- Are map/slice operations bounds-safe?
- Are function return values used correctly? Are multiple return values handled?
- Does the change break any caller that is visible in the repo?

Only report issues you can trace directly to changed lines or to callers of changed functions.

---

### Pass 2 — Silent failures and error handling

Go errors must not be silently dropped. For every changed function:
- Is every `error` return value checked? Flag any `_, err` where `err` is then unused.
- Are errors wrapped with context before being returned? (`fmt.Errorf("...: %w", err)`)
- Are there `if err != nil { return }` paths that skip cleanup (defer, unlock, close)?
- Does a fallback or default value hide a real error that should propagate?
- Are goroutines started without a way to observe their errors?

---

### Pass 3 — Concurrency and resource leaks

- Are goroutines started in changed code? Do they have a clear exit path?
- Is `context.Context` propagated correctly into goroutines and blocking calls?
- Are mutexes locked and unlocked symmetrically? Is there a defer unlock?
- Are channels closed exactly once, by the sender?
- Are `io.Closer` implementations (files, HTTP responses, connections) closed via defer?
- Could a changed timeout or cancellation path leave resources open?

---

### Pass 4 — Security

Focus on these Windsor packages if touched: `pkg/runtime/secrets/`, `pkg/runtime/shell/`, `pkg/runtime/evaluator/`, `pkg/workstation/`.

- Are shell commands constructed from external input using argument lists (not string concatenation)?
- Are file paths validated against traversal (`../`)?
- Could a secret value be logged or included in an error message?
- Is `InsecureSkipVerify` set to `true` outside of test files?
- Are temporary files created with restricted permissions?
- Run `task scan` and include any new High or Medium findings.

```
!`task scan 2>&1 | tail -40`
```

---

### Pass 5 — Architecture boundaries

Check the diff against Windsor's layer rules:
- `cmd/*` must only parse flags and call runtime/composer methods. It must not contain business logic.
- `pkg/runtime/evaluator/*` must not contain provider-specific branches.
- `pkg/runtime/secrets/*` must not orchestrate lifecycle.
- `pkg/runtime/terraform/*` owns terraform metadata — no other package should do terraform introspection.
- `pkg/composer/*` owns blueprint pipelines.
- No package should reach across layers by importing a sibling package's internals.

Also check: does the diff add exported fields where methods should be used instead?

Also run:
```
!`go vet ./... 2>&1`
```

---

## Output format

After all five passes complete, aggregate findings into a single report. Use this format:

```
## Pre-Commit Review

### Critical
- [file.go:42] <one-sentence description of the bug and why it's wrong>

### High
- [file.go:88] <description>

### Medium
- [file.go:15] <description>

### Clean
- Pass 1 Logic: clean
- Pass 2 Errors: 1 issue (see High)
- Pass 3 Concurrency: clean
- Pass 4 Security: clean
- Pass 5 Architecture: clean
```

**Severity guide:**
- **Critical** — will cause a panic, data loss, or security breach at runtime
- **High** — likely incorrect behavior, silent data corruption, or a real error being dropped
- **Medium** — plausible bug under specific conditions worth fixing before merge

If there are no findings, say so explicitly: `No bugs found. Safe to commit.`

Do not list style issues, missing comments, or suggestions. If you are not confident an issue is a real bug, do not include it.
