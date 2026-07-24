---
title: "Composition order"
description: "How facets and components are ordered when blueprints compose, within a blueprint and across its sources."
---
# Composition order

Two independent levels decide what composes before what. Keeping them
apart is the whole trick: `ordinal` orders facets **within a single
blueprint**, and source depth orders **across blueprints**.

| Level | Question it answers | Mechanism |
|------|------|------|
| Within one blueprint | Which of my own facets goes first? | `ordinal`, derived from the file basename, tie-broken by `metadata.name` |
| Across blueprints | Does my source's work land before mine? | Source depth — a source composes before the blueprint that references it |

An `ordinal` never crosses a source boundary. A downstream facet cannot
give itself a low ordinal to compose ahead of the source it extends.

## How composition runs

```
1. Load      Each source becomes its own loader. Sources that themselves
             declare sources: are resolved recursively.

2. Process   Facets are evaluated once per source. Within that source
             they sort by ordinal, then by name. Every component it
             contributes is stamped with its source name ("" for the
             blueprint you are composing).

3. Merge     The per-source blueprints merge in the order the sources:
             list declares, and your own blueprint merges last.
             Same-named components merge rather than duplicate.

4. Order     Components sort topologically on dependsOn, then cluster by
             name prefix (every addon-* together, alphabetically inside
             the cluster). This step does not consider source.

5. Depth     Components stably sort by source depth. This runs last.
```

Step 2 is the one that surprises people. Facets from different sources
are never in the same sort, so the ordinal table cannot rank a source's
facets against yours — they are evaluated in separate passes and only
meet at the merge in step 3.

## Source depth

Depth is distance in the reference graph:

- A source that references nothing else is depth 0.
- A source is one deeper than the deepest source it references.
- The blueprint you are composing is deepest of all.

A component inherits the depth of the source that contributed it. A
component whose source is not in the graph is treated as depth 0,
because nothing establishes it as layered on top of anything.

The depth sort is stable, so within one depth nothing moves and each
source keeps its own tier order. Dependencies flow from a referencing
blueprint down into its sources, so ordering by ascending depth already
places every dependency ahead of the components that depend on it.

Sources are walked in sorted order, and a source caught in a reference
cycle is pinned to depth 0, so a cyclic graph resolves to the same
depths on every run.

## Worked example

A `manager` blueprint references `core`. Core ships `addon-object-store`
and `addon-private-ca`; manager adds `addon-omni`. All three are
`addon-` facets, so all three carry ordinal 400.

Without the depth step, step 4 clusters the three under the `addon`
prefix and alphabetizes them:

```
addon-object-store   (core)
addon-omni           (manager)   <- stranded mid-list
addon-private-ca     (core)
```

Core is depth 0 and manager is depth 1, so the depth step lifts
manager's contribution to the end without disturbing core's own order:

```
addon-object-store   (core)
addon-private-ca     (core)
addon-omni           (manager)
```

Extending a blueprint now means composing after it, with no reserved
ordinal band to memorize and nothing for a new upstream layer to break.

## Common confusions

**"I set a lower ordinal so my facet runs first."** Ordinal only orders
facets inside one blueprint. Against a source, depth decides, and depth
wins.

**"My facet ties with the source's facets."** They never tie, because
they are never compared. Each source is processed on its own, and the
ordering between them comes from depth.

**"I need to feed a value into a source's facets."** Set it in context
configuration rather than reaching backwards with an ordinal. Context
values are in scope for every facet regardless of depth.

**"Single-source projects behave differently now."** They do not. With
no referencing blueprint every component sits at depth 0, so the depth
step changes nothing.

## See also

- [Facets reference](facets.md) — facet fields, including `ordinal`
- [Blueprint reference](blueprint.md) — the composed blueprint
- [Contexts directory](contexts.md) — where facets live on disk
