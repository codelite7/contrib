# entgql: collection split with sub-package dispatch — design

**Date:** 2026-05-17
**Branch:** `entgql-collection-subpkg` (codelite7/contrib fork)
**Status:** Design — awaiting review

## Context

The upstream ent codegen-reduction epic (commits on `entgo.io/ent` master through PR 6) splits each entity's query/client/mutation builders into per-entity sub-packages (`<gen>/<entity>/`). The root `gen` package keeps type aliases (`type AgentLicensingQuery = agentlicensing.Query`) so consumer code calling `*ent.AgentLicensingQuery` keeps compiling.

The `entgql` extension in this fork (`codelite7/contrib`) has a `WithSplitGoFiles(true)` option that emits per-entity `gql_collection_<entity>.go` files alongside the rest of the gen package. After PR 6, these files fail to compile:

```
gql_collection_agent_licensing.go:12:11: cannot define new methods on non-local type AgentLicensingQuery
```

The generated file declares `package gen` and tries to attach `CollectFields` / `collectField` methods to `*AgentLicensingQuery`, which is now an alias to `agentlicensing.Query` in a different package. Go forbids method definitions on aliased types from foreign packages.

This problem affects every entity. The bench `go build` fails after the first entity hits this error.

## Goals

1. **Fix the compile error** for `WithSplitGoFiles(true)` users on ent codebases that use per-entity sub-packages.
2. **Preserve the user API.** `query.CollectFields(ctx)` must keep working from consumer code without changes.
3. **Realize the compile-time parallelism gain** that the sub-package architecture was designed to enable. The per-entity collection code (the bulk of the cross-entity walking logic) should live in entity sub-packages so they can compile in parallel.

## Non-goals

- Re-architecting collection for non-split-go-files users. The existing `collection.tmpl` continues to handle them unchanged.
- Migrating other split templates (`node_entity`, `pagination_entity`). They emit free functions and entity-local types in the gen package, so the alias problem does not bite them.
- Updating downstream gqlgen-generated resolvers. The preserved `q.CollectFields(ctx)` API means resolvers do not change.
- Compile-time benchmarking. Measured separately after implementation.

## Constraint that drives the design

`gen` already imports each sub-package (for the type aliases). For a sub-package's collectField to do cross-entity work — recursing into another entity's collectField, constructing the other entity's `*Query`, applying that entity's pager — it would normally need to either (a) import `gen` (creates cycle: `gen → subpkg_A → gen → subpkg_B`) or (b) import sibling sub-packages directly (creates cycle: `subpkg_A → subpkg_B → subpkg_A` for any pair of entities with reciprocal edges).

Both paths cycle. The architectural primitive that breaks the cycle is **dispatch through an interface in an import-free intermediate package**: the intermediate package depends on nothing, sub-packages register implementations into it at `init()` time, and cross-entity operations look up the registered implementation and call through the interface.

## Architecture

### Package layout

```
<consumer-gen-pkg>/                              (ent-generated)
├── <entity>/                                      (existing per-entity sub-packages)
│   ├── ...existing PR6 files...
│   ├── gql_collection.go                          NEW — per-entity CollectFields/collectField methods
│   └── gql_collection_dispatch.go                 NEW — collector{} EntityCollector impl + init() registration
├── internal/
│   └── collectiondispatch/
│       └── dispatch.go                            NEW — EntityCollector interface + registry
├── gql_collection.go                              MODIFIED — shared helpers only (collectedField, fieldArgs, …)
└── gql_<entity>.go                                UNCHANGED — type aliases live here
```

### The dispatch package

A new generated package at `<gen>/internal/collectiondispatch`. **Import graph:** depends only on the standard library, gqlgen, and entgql. **Does NOT import** the gen package or any sub-package.

```go
package collectiondispatch

import (
    "context"
    "github.com/99designs/gqlgen/graphql"
)

// EntityCollector dispatches collection operations for a single entity.
// Concrete implementations live in each entity's sub-package and register
// themselves at init() time. Sub-packages call Get(otherEntity) and
// invoke the returned EntityCollector to operate on a different entity's
// types without taking a static import on that entity's sub-package.
//
// Every parameter and return is `any` at this boundary. Implementations
// assert back to their concrete *Query / *PaginateArgs / etc. types.
// This intentional type erasure is what breaks the entity-to-entity
// import cycle.
type EntityCollector interface {
    // NewQuery constructs a fresh *Query for the entity, given the shared
    // ent Config (typed as any). Returns the constructed *Query as any.
    NewQuery(config any) any

    // NewPaginateArgs constructs a *PaginateArgs for the entity from raw
    // resolver args (the map[string]any produced by graphql.OperationContext).
    NewPaginateArgs(args any) any

    // NewPager constructs the entity's Pager from the args.opts and last flag.
    NewPager(opts any, last bool) (any, error)

    // ApplyFilter applies the pager's filter to query. Returns the (possibly
    // wrapped) query and any error.
    ApplyFilter(pager any, query any) (any, error)

    // ApplyCursors applies after/before cursors to query.
    ApplyCursors(pager any, query any, after, before any) (any, error)

    // ApplyOrder applies the pager's order to query (used by oneNode branch).
    ApplyOrder(pager any, query any) any

    // ApplyLimit calls query.Limit(n) and returns the modified query.
    ApplyLimit(query any, n int) any

    // CollectFields recurses into the entity's collectField method for nested
    // selections. This is the primary cross-entity recursion call site.
    CollectFields(ctx context.Context, q any, oneNode bool, opCtx *graphql.OperationContext, collected graphql.CollectedField, path []string, satisfies ...string) error

    // LoadTotalCallback returns a func that the parent entity's loadTotal
    // slice will store. The closure body needs access to this entity's types
    // (loadTotal callbacks compute a count by Joining on the entity's table).
    // The factory pattern keeps the closure construction in the sub-package
    // where the types are local.
    LoadTotalCallback(...) func(ctx context.Context, nodes any) error
    // (exact signature TBD during implementation; the surface here may need
    // refinement once we inspect every loadTotal shape across edge variants)
}

var registry = make(map[string]EntityCollector)

// Register associates a collector with an entity name (typically snake_case
// of the entity, matching the sub-package directory name). Called from each
// sub-package's init().
func Register(entity string, c EntityCollector) {
    if _, exists := registry[entity]; exists {
        panic("collectiondispatch: duplicate registration for entity " + entity)
    }
    registry[entity] = c
}

// Get retrieves the collector for an entity. Returns nil if not registered.
// Callers should check for nil and produce a clear error rather than nil-dereference.
func Get(entity string) EntityCollector {
    return registry[entity]
}
```

### Per-entity files

#### `<entity>/gql_collection.go` (generated from new `collection_subpkg.tmpl`)

```go
package agentlicensing

import (
    "context"
    "fmt"
    "github.com/99designs/gqlgen/graphql"
    "<gen>/internal/collectiondispatch"
    // sql, predicate, sibling-entity-type imports as needed for SAME-entity ops
)

// CollectFields tells the query-builder to eagerly load connected nodes by resolver context.
func (q *Query) CollectFields(ctx context.Context, satisfies ...string) (*Query, error) {
    fc := graphql.GetFieldContext(ctx)
    if fc == nil {
        return q, nil
    }
    if err := q.collectField(ctx, false, graphql.GetOperationContext(ctx), fc.Field, nil, satisfies...); err != nil {
        return nil, err
    }
    return q, nil
}

// collectField iterates this entity's scalar fields and edge selections.
// For each cross-entity edge, it dispatches all operations on the OTHER entity
// through collectiondispatch. Same-entity operations (loadTotal closures on
// *Query, predicate.<Edge>, etc.) are direct calls to local symbols.
func (q *Query) collectField(ctx context.Context, oneNode bool, opCtx *graphql.OperationContext, collected graphql.CollectedField, path []string, satisfies ...string) error {
    path = append([]string(nil), path...)
    var (
        unknownSeen    bool
        fieldSeen      = make(map[string]struct{}, len(Columns))
        selectedFields = []string{FieldID}
    )
    for _, field := range graphql.CollectFields(opCtx, collected.Selections, satisfies) {
        switch field.Name {
        case "recordTypes":  // cross-entity edge to recordtype
            other := collectiondispatch.Get("recordtype")
            if other == nil {
                return fmt.Errorf("collectiondispatch: no collector registered for recordtype")
            }
            alias := field.Alias
            edgePath := append(path, alias)
            otherQuery := other.NewQuery(q.config)
            args := other.NewPaginateArgs(fieldArgs(ctx, /* WhereInput */ nil, edgePath...))
            // ... validateFirstLast (free helper in gen, no entity coupling) ...
            pager, err := other.NewPager(/* opts */ nil, /* last */ false)
            if err != nil { ... }
            otherQuery, err = other.ApplyFilter(pager, otherQuery)
            // ... etc, all cross-entity ops via `other` ...
            if err := other.CollectFields(ctx, otherQuery, false, opCtx, *field, edgePath, recordtypeImplementorsFromShared...); err != nil {
                return err
            }
        case "scalarFieldName":  // SAME-entity scalar field
            if _, ok := fieldSeen[field.Name]; ok { continue }
            selectedFields = append(selectedFields, /* corresponding column */ )
            fieldSeen[field.Name] = struct{}{}
        }
    }
    // ... selectedFields handling, unknownSeen handling — all entity-local logic ...
    return nil
}
```

Same-entity loop body (scalar fields, local-edge predicates) keeps the direct-symbol style. Cross-entity loop body (every edge case) is rewritten to call through `other`.

#### `<entity>/gql_collection_dispatch.go` (generated from new `collection_dispatch.tmpl`)

```go
package agentlicensing

import (
    "context"
    "github.com/99designs/gqlgen/graphql"
    "<gen>/internal/collectiondispatch"
)

type collector struct{}

// EntityCollector method implementations. Each asserts back to local concrete types.

func (collector) NewQuery(config any) any {
    return (&Client{config: config.(*Config)}).Query()  // exact ctor TBD per ent version
}

func (collector) NewPaginateArgs(args any) any {
    return newPaginateArgs(args.(map[string]any))  // newPaginateArgs is local; was newAgentLicensingPaginateArgs before
}

func (collector) NewPager(opts any, last bool) (any, error) {
    return newPager(opts.([]PaginateOption), last)
}

func (collector) ApplyFilter(pager any, query any) (any, error) {
    return pager.(*Pager).applyFilter(query.(*Query))
}

func (collector) ApplyCursors(pager any, query any, after, before any) (any, error) {
    return pager.(*Pager).applyCursors(query.(*Query), after.(*Cursor), before.(*Cursor))
}

func (collector) ApplyOrder(pager any, query any) any {
    return pager.(*Pager).applyOrder(query.(*Query))
}

func (collector) ApplyLimit(query any, n int) any {
    return query.(*Query).Limit(n)
}

func (collector) CollectFields(ctx context.Context, q any, oneNode bool, opCtx *graphql.OperationContext, collected graphql.CollectedField, path []string, satisfies ...string) error {
    return q.(*Query).collectField(ctx, oneNode, opCtx, collected, path, satisfies...)
}

func (collector) LoadTotalCallback(...) func(context.Context, any) error {
    // local closure operating on []*<Entity>; type-asserted within
}

func init() {
    collectiondispatch.Register("agentlicensing", collector{})
}
```

### Per-entity types: PaginateArgs, Pager, Cursor

The existing `collection_entity.tmpl` defines `<entity>PaginateArgs` (lowercase, gen-package-internal) and `new<Entity>PaginateArgs`. After this design, these move into the entity sub-package as `PaginateArgs` and `newPaginateArgs` (or stay lowercase if always accessed via the collector). Either way, they live where the rest of that entity's types live.

`Cursor` and order types may need to stay in either the gen package or move to sub-packages. The decision criterion: do they appear in user-facing API surfaces? If yes (e.g., `Cursor` is exported and consumers reference it), they stay in gen with their current exported names. If no (internal-only), they can move to sub-packages.

This is a detail that the implementation plan will pin down by inspecting actual usage.

### Shared helpers (`<gen>/gql_collection.go`)

After this design, the root `gen/gql_collection.go` holds only the helpers that are NOT bound to any entity:

- `collectedField(ctx, path...)` — graphql field-path navigation
- `hasCollectedField(ctx, path...)` — same
- `fieldArgs(ctx, whereInput, path...)` — same
- `validateFirstLast(first, last)` — pagination validation
- `paginateLimit(first, last)` — pagination math
- `mayAddCondition(satisfies, implementors)` — interface satisfaction

These have no per-entity type parameters. They stay in gen, generated from the existing `collection_shared.tmpl` (which already exists in this fork).

### Codegen wiring

In `entgql/extension.go`:

**Add**:
- `func (e *Extension) generateCollectionDispatchPkg(g *gen.Graph) error` — writes `<g.Target>/internal/collectiondispatch/dispatch.go` once
- `func (e *Extension) generateCollectionSubpkgFile(g *gen.Graph, n *gen.Type) error` — writes `<g.Target>/<entity>/gql_collection.go`
- `func (e *Extension) generateCollectionDispatchFile(g *gen.Graph, n *gen.Type) error` — writes `<g.Target>/<entity>/gql_collection_dispatch.go`

**Modify** `generateSplitGoFiles`:
- Call `generateCollectionDispatchPkg` once at the start of the fan-out
- Replace each per-entity `generateCollectionEntityFile` call with two calls: `generateCollectionSubpkgFile` + `generateCollectionDispatchFile`

**Remove**:
- `generateCollectionEntityFile` (no longer used)
- `CollectionEntityTemplate` (no longer parsed)
- `entgql/template/collection_entity.tmpl` (no longer needed)

In `entgql/template.go`:

**Add**:
- `CollectionSubpkgTemplate = parseEntityTemplate("template/collection_subpkg.tmpl", "gql_collection_subpkg")`
- `CollectionDispatchTemplate = parseEntityTemplate("template/collection_dispatch.tmpl", "gql_collection_dispatch")`
- `CollectionDispatchPkgTemplate = parseT("template/collection_dispatch_pkg.tmpl")`

**Remove**:
- `CollectionEntityTemplate`

### Behavior at runtime

1. At process start, every entity's `init()` runs, registering its `collector{}` with `collectiondispatch`.
2. Consumer calls `client.AgentLicensing.Query().CollectFields(ctx)`.
3. Resolves through `type AgentLicensingQuery = agentlicensing.Query` to `agentlicensing.Query.CollectFields` (subpkg method).
4. `CollectFields` calls `q.collectField(ctx, ...)` — local method.
5. `collectField` iterates fields. For each cross-entity edge, calls `collectiondispatch.Get(otherEntity)` and dispatches operations through the returned `EntityCollector` interface.
6. The other entity's `collector{}.CollectFields` asserts the query back to `*OtherQuery` and recurses on the local `collectField`.

No static cross-subpkg imports for cross-entity work. Each subpkg compiles independently of other subpkgs (it depends only on `collectiondispatch`, the gen package's shared helpers, and its own internal sub-imports).

## Compile-time impact

**Before this design:** `gen` package contains, per entity: type alias (~3 lines) + monolithic CollectFields/collectField (~200 lines) + PaginateArgs type/ctor (~80 lines). For 60 entities, that's ~17K lines in a single Go package.

**After this design:**
- `gen` package: per-entity ~3 lines of type alias + shared helpers (~150 lines total). On the order of 300 lines for 60 entities.
- Each `<entity>/gql_collection.go` + `<entity>/gql_collection_dispatch.go`: ~300 lines per sub-package, compiled in parallel.
- `internal/collectiondispatch`: ~80 lines once.

Compile parallelism win: instead of one 17K-line gen package serialized on a single core, 60 sub-packages of ~300 lines each can be compiled in parallel.

Type erasure cost at runtime: each cross-entity dispatch is a map lookup + a type assertion (negligible — collection runs once per GraphQL request, not in hot loops).

## Migration impact

### Consumers of `WithSplitGoFiles(true)`

- `q.CollectFields(ctx)` works identically (alias resolves through to subpkg method)
- `gen/gql_collection_<entity>.go` files disappear from generated output
- `<gen>/<entity>/gql_collection.go` and `<gen>/<entity>/gql_collection_dispatch.go` appear
- `<gen>/internal/collectiondispatch/` appears
- If any consumer reached into the OLD per-entity unexported types (`<entity>PaginateArgs`), it breaks — but these are unexported and not part of any public API contract

### Consumers of `WithSplitGoFiles(false)`

- No change. The monolithic `collection.tmpl` continues to generate `gen/gql_collection.go` with methods on the (non-aliased, fully concrete) `*<Entity>Query` types directly.

## Testing

### Unit tests

Following the existing pattern in `entgql/template_test.go`:

- `TestCollectionSubpkgTemplate_Parse` — round-trip template parses and contains `gql_collection_subpkg` define block
- `TestCollectionSubpkgTemplate_RenderSimple` — render against a fixture `Graph` with one entity, one edge; assert output contains expected method signatures
- `TestCollectionDispatchTemplate_Parse` — same
- `TestCollectionDispatchTemplate_RenderRegistersEntity` — assert output contains `collectiondispatch.Register("<entity>", collector{})`
- `TestCollectionDispatchPkgTemplate_Parse` — assert output declares the `EntityCollector` interface and `Register`/`Get` functions

### Integration test

A new test that:
1. Generates a small fixture ent schema with `WithSplitGoFiles(true)` enabled
2. Runs the full codegen pipeline against it
3. Compiles the result with `go build ./...`
4. Asserts compile success (no "non-local type" errors)
5. Optionally: runs a smoke-test resolver that exercises `query.CollectFields(ctx)` end-to-end against an in-memory ent backend

Test location: `entgql/internal/todo_subpkg/` or similar, mirroring existing integration test fixtures.

### Regression coverage

- Run all existing `WithSplitGoFiles(true)` tests against the new code — they must still pass
- Run all `WithSplitGoFiles(false)` tests — must be untouched

## Risk register

| Risk | Likelihood | Mitigation |
|---|---|---|
| `EntityCollector` interface needs methods I didn't enumerate | Medium-high | Discover during implementation; expand interface as needed. The interface is internal to the dispatch package, so adding methods is non-breaking. |
| Some cross-entity operation can't be type-erased cleanly (e.g., closures that build typed slices) | Medium | The `LoadTotalCallback` and per-edge closure construction stays in the sub-package (where types are local). Dispatch returns the closure; the parent passes nodes as `any` and the implementation asserts back. |
| Hidden cross-entity ref in shared helpers I haven't seen | Low | Implementation begins with a full read of the existing `collection.tmpl` body to enumerate every cross-entity reference site |
| `collectiondispatch.Get(name)` returns nil for an unregistered entity | Low | Every entity's `init()` registers; the only failure mode is a code-generation bug. Add a clear `fmt.Errorf` rather than a nil dereference |
| Pager / Cursor type placement uncertainty | Medium | Resolved during implementation by usage inspection; documented in spec section above. Each is either kept-in-gen-as-exported or moved-to-subpkg with the rest of that entity's types |

## Open questions to resolve during implementation

1. Exact final `EntityCollector` interface — the sketch above lists the operations I can identify from one read of `gql_collection_profile.go`. Other entities may need additional dispatch operations (e.g., for entities with custom edge cardinalities, where-input handling, etc.).
2. Where `Cursor`, `OrderField`, `OrderDirection`, `PaginateOption` finally live — gen or subpkg.
3. Whether `LoadTotalCallback` is one method or several (per edge cardinality variant).
4. Whether the `collector{}` instance can be a package-level `var collector = collectorImpl{}` instead of a per-call `collector{}` literal, to reduce GC pressure.

These are deferred to the implementation plan, not blockers for design approval.

## Out of scope

- Migrating `node_entity.tmpl`, `pagination_entity.tmpl`, `edge_entity.tmpl` to the dispatch pattern. They emit free functions and entity-local types in gen — no alias-method bug — and don't trigger Bug 9.
- Backporting the dispatch pattern to ent's upstream entgql. This fork carries the change; upstream sync (if ever) is a separate effort.
- Compile-time benchmarking of the parallelism win. Measured as a follow-up using `go build -p` and timing comparisons.
- gqlgen-side template changes. None needed — consumer API preserved.

## Acceptance criteria

1. `WithSplitGoFiles(true)` generates `<gen>/internal/collectiondispatch/dispatch.go` plus `<gen>/<entity>/gql_collection.go` and `<gen>/<entity>/gql_collection_dispatch.go` for every entity
2. The generated output `go build`s on a real ent schema (validated by the integration test, validated by the consumer bench in `service-api-go`)
3. `q.CollectFields(ctx)` is callable from consumer code via the existing type aliases without source changes
4. Existing entgql tests pass unchanged
5. The old `gql_collection_<entity>.go` files are no longer generated; `collection_entity.tmpl` and `generateCollectionEntityFile` are removed
6. `WithSplitGoFiles(false)` users see no change
