---
name: docs-author
description: Author and maintain Windsor CLI reference Markdown for ingestion into windsorcli.github.io (Astro). Use when adding or editing CLI docs, command reference, env/config catalogs, or Cobra-generated MD under docs/.
---

# CLI docs author

## Apply when

- Adding or changing user-facing CLI behavior that needs reference text (flags, env, exit codes, examples).
- Creating or reorganizing files under `docs/` (especially `docs/reference/`).
- Wiring or updating Cobra → Markdown generation for commands.

## Do not apply when

- The change is code-only with no doc surface, or README tweaks unrelated to published reference.

## Contract with the docs site

Reference from this repo is **ingested** into [windsorcli.github.io](https://github.com/windsorcli/windsorcli.github.io) at build time (pinned release). Target URL prefix after materialization:

| Author in this repo | Public URL (stable contract) |
|---------------------|-------------------------------|
| `docs/reference/commands/**` | `https://www.windsorcli.dev/docs/reference/cli/commands/**` |
| `docs/reference/*.md` (config, env catalog, etc.) | `https://www.windsorcli.dev/docs/reference/cli/**` |

**Do not** author encyclopedia-style reference under paths that would map to **editorial** routes such as `/docs/cli/getting-started` (those pages live in the website repo). CLI reference belongs under `docs/reference/`.

Editorial journeys (install flow copy, “getting started” prose) stay in the website repo unless the team explicitly chooses to duplicate—prefer linking out to `https://www.windsorcli.dev/docs/...` for blueprint/schema concepts.

## Frontmatter (Markdown)

Use YAML frontmatter compatible with Astro content collections:

- `title` (required): short page title.
- `description` (recommended): one line for SEO/cards.
- Optional: `sidebar_order` (number) for nav ordering after ingest.

## Voice and structure

- **Neutral, imperative, versioned.** Tables for flags, env vars, defaults, and “required vs optional.”
- **Examples**: real shell snippets; show common and advanced invocations separately when helpful.
- **Cross-links**: blueprint API, schema, facets → `https://www.windsorcli.dev/docs/blueprints/...` (full URL in source is acceptable so links work on GitHub too).

## Commands

- Prefer **generated** command reference from Cobra (single source of truth with `cmd/` flags) into `docs/reference/commands/` or the agreed generator output path.
- If hand-maintaining a command page, it must stay aligned with flag names and defaults in code; call out breaking changes in PR description.

## Config and environment

- Centralize `WINDSOR_*` and related env in a dedicated reference file (e.g. `docs/reference/environment.md`) or structured fragments—avoid scattering the same table across many pages.

## PR checklist

- [ ] New/changed flags or env documented under `docs/reference/` (or generator updated).
- [ ] Frontmatter present on new pages.
- [ ] No accidental slug overlap with website-only `/docs/cli/*` narrative paths.
- [ ] Links to generic blueprint docs use windsorcli.dev URLs where repo-relative links would break post-ingest.

## Internal architecture note

Ingest layout, pins, and Renovate expectations: [windsorcli.github.io `docs/plan.md` on GitHub](https://github.com/windsorcli/windsorcli.github.io/blob/main/docs/plan.md) (maintainer doc, not shipped to the public docs site; paths may evolve until `docs:vendor` lands—prefer the **public URL prefix** column when in doubt).

To preview reference in the website repo against a **local** checkout: from `windsorcli.github.io`, run `npm run docs:vendor:local` (expects `../cli` and `../core`), then `npm run dev`.
