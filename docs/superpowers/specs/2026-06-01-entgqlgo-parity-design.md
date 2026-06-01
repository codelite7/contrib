# entgqlgo: Parity with entgql+gqlgen — Design

**Date:** 2026-06-01
**Status:** Approved
**Branch:** `graphql-go`

## Context

`entgqlgo` is an entc extension (sibling package to `entgql`) that generates Go code
targeting `github.com/graphql-go/graphql` v0.8.1 instead of `.graphql` SDL files for
gqlgen. It already implements: Relay node interface, WhereInput filtering, ordering
(incl. multi-order), create/update/delete mutations, enums, custom scalars, edge
eager-loading via field collection, and context-based client injection.

**Scope boundary:** the package only replaces the GraphQL resolver/schema layer that
gqlgen provides. Ent's generated client does all querying and mutating. No data-layer
features beyond what entgql+gqlgen exposes.

**Definition of done:** a user of entgql+gqlgen could switch to entgqlgo and serve the
same schema — same queries, mutations, filtering, ordering, and pagination.

### Key finding driving this design

Commit `62347dc9` removed Relay Connection root queries from the schema template,
claiming this matched gqlgen behavior. It does not. entgql generates Connection-typed
root queries with cursor pagination:

```graphql
# entgql (parity target)                      # entgqlgo (current — wrong)
todos(after: Cursor, first: Int,              todos(first: Int, offset: Int,
      before: Cursor, last: Int,                    orderBy: [TodoOrder!],
      orderBy: [TodoOrder!],                        where: TodoWhereInput): [Todo!]!
      where: TodoWhereInput): TodoConnection!
```

The Relay machinery (Cursor scalar, PageInfo type, generic `Cursor[T]`, connection
collection helpers) still exists in the generated code but is no longer wired into the
schema. Restoring it is the core of this work.

## Non-goals

Documented in the README as known differences; not implemented:

- **`Directives` annotation** — graphql-go fields cannot carry applied schema
  directives (verified against v0.8.1: `graphql.Field` has no directives slot). The
  graphql-go idiom is resolver wrapping; the README documents this alternative.
- **SDL file output, gqlgen config integration, `@goField`/`@goModel`** — gqlgen-specific.
- **Query complexity analysis** — gqlgen runtime feature.
- **Generated subscriptions** — entgql generates none either. Custom subscription
  fields are supported via schema extensibility (below).

## Design

### 1. Relay Connection restoration

Rewire the orphaned Relay machinery into `template/schema.tmpl` and `template/types.tmpl`:

- Entity with `RelayConnection()` + `QueryField()` → root query
  `todos(after: Cursor, first: Int, before: Cursor, last: Int, orderBy: [TodoOrder!], where: TodoWhereInput): TodoConnection!`
- Generated per-entity types: `TodoConnection { edges: [TodoEdge], pageInfo: PageInfo!, totalCount: Int! }`,
  `TodoEdge { node: Todo, cursor: Cursor! }`. Shared `PageInfo` type and `Cursor`
  scalar already exist in `scalars.go` (currently unused).
- Edge with `RelayConnection()` → connection field on the parent type
  (`Todo.children(after, first, before, last, orderBy, where): TodoConnection!`).
- `QueryField()` *without* `RelayConnection()` → plain list with no pagination args,
  matching entgql (`billProducts: [BillProduct!]!`). The current `first`/`offset` args
  on plain-list queries are removed.
- `orderBy` arg type follows entgql: `[TodoOrder!]` when `MultiOrder()`, `TodoOrder`
  otherwise.
- Resolvers handle cursor encode/decode and limit+1 lookahead inside the generated
  gqlgo package; ent executes the actual queries.

### 2. Schema extensibility

Code-first equivalent of entgql users adding extra `.graphql` files. The generated
package exposes composition points instead of a sealed `NewSchema`:

```go
// Simple path — unchanged:
schema, err := gqlgo.NewSchema(client)

// Extensible path:
cfg := gqlgo.SchemaConfig(client)              // generated: returns graphql.SchemaConfig
cfg.Query.AddFieldConfig("ping", &graphql.Field{ /* ... */ })
cfg.Mutation.AddFieldConfig("clearTodos", &graphql.Field{ /* ... */ })
cfg.Subscription = graphql.NewObject(/* ... */) // served via graphql.Subscribe
schema, err := graphql.NewSchema(cfg)
```

- Generated object types (`TodoType`, connection types, where inputs) remain exported
  package vars so custom resolvers can reference them.
- `AddFieldConfig` is existing graphql-go API; no wrapper API is invented.
- `NewSchema(client)` is implemented as `graphql.NewSchema(SchemaConfig(client))`.

### 3. Transactions

entgql wraps mutations in a transaction via gqlgen middleware (`Transactioner`).
graphql-go has no middleware layer, so wrapping happens at resolver level:

```go
schema, err := gqlgo.NewSchema(client, gqlgo.WithTransactions())
```

- When enabled, each generated mutation resolver: opens `client.Tx(ctx)` → puts the tx
  client in context (existing `clientFromContext()` lookup already reads it) → runs the
  mutation body → commits on success, rolls back on error or panic.
- Custom mutations opt in via an exported helper: `gqlgo.WithTx(client, resolverFunc)`.
- Default off, matching entgql where `Transactioner` is opt-in.

### 4. Annotation parity

- **`Implements(...string)`** — entity types implement custom GraphQL interfaces. The
  user registers the interface type via `SchemaConfig`; generated object types list it
  in `Interfaces` alongside `Node`.
- **`DeprecatedEnumValues`** — maps to graphql-go `EnumValueConfig.DeprecationReason`
  (verified present in v0.8.1).
- **`Directives`** — explicitly not implemented (see Non-goals).

### 5. Documentation

`entgqlgo/README.md` covering:

- Setup: `entc.go` wiring, generation, serving with `graphql-go/handler`
- Every annotation with before/after schema output
- Extensibility worked example: custom query + custom subscription via `graphql.Subscribe`
- Transactions
- "Differences from entgql+gqlgen" table (the Non-goals above)

## Testing

- Each feature keeps unit tests in `entgqlgo/` plus integration tests in
  `internal/todo` (SQLite, real `graphql.Do()` execution).
- The example schema grows per feature: connections need an entity with
  `RelayConnection()` edges in both directions (exists: `Todo.children`/`parent`) plus
  a plain-list `QueryField()` entity (add one, mirroring entgql's `billProducts`).
- **Acceptance test:** build the same ent schema under both entgql and entgqlgo, render
  both to SDL (entgql natively; entgqlgo via introspection→SDL printing), and diff the
  in-scope parts — connections, inputs, enums, mutations. Differences must appear in an
  explicit allowlist (gqlgen-specific directives, etc.).
  - Mechanism: both packages register their annotation under the same key (`"EntGQL"`,
    verified in `entgql/annotation.go:118` and `entgqlgo/annotation.go:101`), so the
    test schema is annotated once with `entgql.*` annotations and read by both
    extensions. entgqlgo's `Annotation` struct must remain field-compatible with
    entgql's for the fields it supports.
- The introspection golden file (`testdata/schema_introspection.json`) stays as
  regression protection and is regenerated as features land.

## Build order

Each step is independently shippable:

1. **Relay Connections** — root queries, edge fields, plain-list parity (the schema-shape fix)
2. **Schema extensibility** — `SchemaConfig()`; small, unblocks custom-resolver users
3. **Transactions** — `WithTransactions()`, `WithTx()`
4. **Annotations** — `Implements`, `DeprecatedEnumValues`
5. **Parity acceptance test**
6. **README**
