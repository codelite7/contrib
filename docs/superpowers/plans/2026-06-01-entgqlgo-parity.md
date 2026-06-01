# entgqlgo Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the `entgqlgo` extension to schema-shape parity with entgql+gqlgen, per the approved spec at `docs/superpowers/specs/2026-06-01-entgqlgo-parity-design.md`.

**Architecture:** `entgqlgo` is an entc extension that generates Go code targeting `github.com/graphql-go/graphql` (code-first; no SDL files). Generated code lives in the consumer's `ent/gqlgo/` package. Templates live in `entgqlgo/template/*.tmpl`; template helper functions in `entgqlgo/template.go`; annotations in `entgqlgo/annotation.go`. The example/test app is `entgqlgo/internal/todo/`. Ent's generated client does all querying — generated resolvers only parse GraphQL args, call ent, and shape results.

**Tech Stack:** Go, ent (entc codegen, text/template), github.com/graphql-go/graphql v0.8.1, github.com/stretchr/testify, github.com/vektah/gqlparser/v2 (Task 8 only).

---

## Critical context (read before starting any task)

### The desync problem

**The committed generated code in `entgqlgo/internal/todo/ent/gqlgo/` was NOT produced by the templates at HEAD.** Previous work edited generated files and templates independently. Running `go generate ./...` in `entgqlgo/internal/todo/` produces ~400 lines of diff against the committed code. Tests currently pass against the *committed* code, not against template output.

Where they differ, this is who is right (verified against entgql's generated SDL at `entgql/internal/todo/ent.graphql`):

| Difference | Committed code | Template output | Parity-correct |
|---|---|---|---|
| Mutation field names | `createTodo` | `CreateTodo` | **Committed** (camelCase) |
| Enum value names | `IN_PROGRESS` | `InProgress` | **Committed** (values, not Go names) |
| NonNull wrappers on required fields | absent | present | **Template** |
| Time scalar | `DateTime` | `Time` | **Template** |
| RelayConnection edge fields | plain lists | Connection types | **Template** |
| `node`/`nodes` resolver tx-awareness | plain client | `clientFromContext` | **Template** |

Task 1 fixes the two template bugs, then regenerates and makes templates the source of truth forever.

### Key commands

```bash
# Regenerate the example app's code from templates (run from repo root):
cd entgqlgo/internal/todo && go generate ./... && cd ../../..

# Run all entgqlgo tests (unit + integration):
cd entgqlgo && go test ./... && cd ..

# Run one test:
cd entgqlgo && go test ./internal/todo/ -run TestQueryTodos -v && cd ..

# Refresh the introspection golden file (test recreates it when missing):
rm entgqlgo/internal/todo/testdata/schema_introspection.json
cd entgqlgo && go test ./internal/todo/ -run TestSchemaIntrospection && cd ..
```

### Key files

| File | Role |
|---|---|
| `entgqlgo/template/schema.tmpl` | Root Query/Mutation types, NewSchema, mutation input parsing (657 lines) |
| `entgqlgo/template/types.tmpl` | Per-entity `graphql.Object` types + edge resolvers (170 lines) |
| `entgqlgo/template/pagination.tmpl` | Connection/Edge structs + GraphQL types + paginate functions (391 lines) |
| `entgqlgo/template/enum.tmpl` | GraphQL enum types (56 lines) |
| `entgqlgo/template/ordering.tmpl` | Order enums/inputs, `Parse*Order`, `Apply*OrderList` |
| `entgqlgo/template/where_input.tmpl` | `*WhereInput` types, `Parse*WhereInput`, `.Filter(query)`, `.P()` |
| `entgqlgo/template/collection.tmpl` | Eager-loading: `{T}QueryCollectFields`, `{T}QueryCollectFieldsConnection` |
| `entgqlgo/template.go` | Template func registry (`TemplateFuncs`), `AllTemplates` list |
| `entgqlgo/annotation.go` | `Annotation` struct + annotation constructors |
| `entgqlgo/extension.go` | `Extension`, `NewExtension`, options |
| `entgqlgo/internal/todo/ent/schema/{todo,category}.go` | Example ent schemas |
| `entgqlgo/internal/todo/ent/entc.go` | Codegen entrypoint (`go:build ignore`) |
| `entgqlgo/internal/todo/todo_test.go` | Integration tests (47 test funcs, 2965 lines) |
| `entgqlgo/internal/todo/introspection_test.go` | Golden-file schema test |

### Verified API facts (do not re-derive)

- graphql-go v0.8.1 `Field` struct has `Subscribe FieldResolveFn` and `DeprecationReason string` (definition.go:604-612). `EnumValueConfig` has `DeprecationReason` (definition.go:936-938). `Object.AddFieldConfig(name, *Field)` exists (definition.go:408). `InterfacesThunk` exists (definition.go:368) and `ObjectConfig.Interfaces` accepts it (definition.go:451). `graphql.Subscribe(Params) chan *Result` exists (subscription.go:27).
- Generated ent package has `FromContext` (ent.go:55), `NewContext` (ent.go:61), `Client.Tx(ctx)` (client.go:137).
- Generated WhereInput has `.Filter(query) (query, error)` and `.P() (predicate, error)`.
- Collection helpers generated per type: `{T}QueryCollectFields(ctx, info, query)`, `{T}QueryCollectFieldsConnection(ctx, info, query)` (for selections under `edges { node { ... } }`).
- Ordering generated per type: GraphQL input named `TodoOrder` (Go var `TodoOrderInputType`), enum named `TodoOrderField`, parse helpers `ParseTodoOrder(map)`, `ParseTodoOrderList([]interface{})`, `ApplyTodoOrderList(query, orders)`.
- Pagination naming helper `gqlgoNodePaginationNames $n` returns `.Connection` (`TodoConnection`), `.Edge` (`TodoEdge`), `.Order` (`TodoOrder`), `.OrderField`, `.WhereInput` (`TodoWhereInput`).
- Both entgql and entgqlgo register their annotation under the key `"EntGQL"`, with compatible JSON field names. entgql's `Implements` and `DeprecatedEnumValues` JSON keys are `"Implements"` and `"DeprecatedEnumValues"` — entgqlgo must use the same names (Tasks 6–7).
- entgql parity targets (from `entgql/internal/todo/ent.graphql`):
  - Connection root query: `todos(after: Cursor, first: Int, before: Cursor, last: Int, orderBy: [TodoOrder!], where: TodoWhereInput): TodoConnection!`
  - Plain-list root query (QueryField without RelayConnection): `billProducts: [BillProduct!]!` — **no arguments**
  - `TodoConnection { edges: [TodoEdge], pageInfo: PageInfo!, totalCount: Int! }`, `TodoEdge { node: Todo, cursor: Cursor! }`
  - Mutations: `createTodo(input: CreateTodoInput!): Todo!`, `updateTodo(id: ID!, input: UpdateTodoInput!): Todo!`

### Conventions

- Per repo owner's rules: **`git add` and `git commit` are always separate commands — never chain with `&&`.**
- After every template change, regenerate before running integration tests.
- Generated-code diffs are part of each commit (templates + regenerated code + golden file move together).

---

### Task 1: Re-sync templates and generated code

The atomic foundation task. After this task: templates are the source of truth, `go generate` produces zero diff, all tests pass.

**Files:**
- Modify: `entgqlgo/template.go` (remove ResolversTemplate from AllTemplates)
- Delete: `entgqlgo/template/resolvers.tmpl`
- Modify: `entgqlgo/template/schema.tmpl` (mutation field naming)
- Modify: `entgqlgo/template/enum.tmpl` (enum value keys)
- Modify: `entgqlgo/template/pagination.tmpl` (query-based paginate function)
- Modify: `entgqlgo/template/types.tmpl` (RelayConnection edge resolvers)
- Regenerate: `entgqlgo/internal/todo/ent/gqlgo/*` (resolvers.go gets deleted)
- Modify: `entgqlgo/internal/todo/todo_test.go` (edge-query tests)
- Regenerate: `entgqlgo/internal/todo/testdata/schema_introspection.json`

- [ ] **Step 1: Confirm the desync (baseline evidence)**

Run:
```bash
cd entgqlgo/internal/todo && go generate ./... && cd ../../..
git status --short entgqlgo/
```
Expected: ~7 modified files under `entgqlgo/internal/todo/ent/gqlgo/`.

Run:
```bash
cd entgqlgo && go test ./internal/todo/ 2>&1 | tail -20 && cd ..
```
Expected: FAIL (regenerated code doesn't match test/golden expectations — this is the desync).

Restore before making template fixes:
```bash
git checkout -- entgqlgo/internal/todo/
```

- [ ] **Step 2: Remove the dead ResolversTemplate**

The generated `Resolvers` struct (`resolvers.go`, 313 lines) is referenced nowhere — verified via `grep -rn "NewResolvers" entgqlgo/ --include="*.go"` returning only the generated file itself. It duplicates schema.tmpl's resolver logic and will drift.

In `entgqlgo/template.go`:
1. Delete the var declaration block:
```go
	// ResolversTemplate generates resolver functions using ent queries.
	ResolversTemplate = parseT("template/resolvers.tmpl")
```
2. Delete the `ResolversTemplate,` line from the `AllTemplates` slice.
3. Delete the template file:
```bash
rm entgqlgo/template/resolvers.tmpl
```
4. Build to confirm no references break:
```bash
cd entgqlgo && go build ./... && cd ..
```
Expected: builds cleanly. (If `extension.go` or tests reference `ResolversTemplate`, remove those references too — `grep -rn ResolversTemplate entgqlgo/`.)

- [ ] **Step 3: Fix mutation field naming in schema.tmpl**

In `entgqlgo/template/schema.tmpl`, three field-name keys use PascalCase. Change them to camelCase (entgql/gqlgen parity — committed generated code already uses camelCase):

| Line (approx) | Old | New |
|---|---|---|
| 192 | `"Create{{ $n.Name }}": &graphql.Field{` | `"create{{ $n.Name }}": &graphql.Field{` |
| 219 | `"Update{{ $n.Name }}": &graphql.Field{` | `"update{{ $n.Name }}": &graphql.Field{` |
| 265 | `"Delete{{ $n.Name }}": &graphql.Field{` | `"delete{{ $n.Name }}": &graphql.Field{` |

Do NOT change `Name: "Create{{ $n.Name }}Input"` (line ~311) or `Name: "Update{{ $n.Name }}Input"` (line ~348) — input *type* names are PascalCase in entgql too.

- [ ] **Step 4: Fix enum value names in enum.tmpl**

In `entgqlgo/template/enum.tmpl`, the non-`UseEnumNames` branch uses the Go enum *name* as the GraphQL enum value. Parity requires the *value*. Change the `{{- else }}` branch of the `{{- if $useEnumNames }}` conditional from:

```
		{{- else }}
		"{{ gqlgoTrimPrefix $e.Name (pascal $f.Name) }}": &graphql.EnumValueConfig{
			Value: "{{ $e.Value }}",
		},
		{{- end }}
```

to:

```
		{{- else }}
		"{{ $e.Value }}": &graphql.EnumValueConfig{
			Value: "{{ $e.Value }}",
		},
		{{- end }}
```

(The `{{- if $useEnumNames }}` branch keeps using the trimmed name — that is the documented purpose of `UseEnumNames`.)

- [ ] **Step 5: Replace client-based Paginate functions with a query-based one in pagination.tmpl**

The existing `Paginate{{ plural $n.Name }}` / `Paginate{{ plural $n.Name }}WithOrder` functions (pagination.tmpl lines ~161–388) take a `*ent.Client` and build their own query, so they cannot paginate edge queries (`source.QueryChildren()`) or pre-filtered queries. They are also referenced by nothing (verified). Replace everything from the line `// {{ $n.QueryName }}Option is a function that modifies a {{ $n.QueryName }}.` (line ~161) through the end of the per-node `{{- range $n := $gqlNodes }}` block (line ~388, just before the closing `{{- end }}` of the range and the final `{{ end }}`) with:

```
// paginate{{ $n.Name }}Query applies Relay-style cursor pagination to the given query.
// The query may already have filters and eager-loading applied; ent executes all queries.
func paginate{{ $n.Name }}Query(
	ctx context.Context,
	query *{{ $entPkg }}.{{ $n.QueryName }},
	after, before *entgqlgo.Cursor[{{ $idType }}],
	first, last *int,
{{- if $gqlgoOrderFields }}
{{- if $multiOrder }}
	orders []*{{ $names.Order }},
{{- else }}
	order *{{ $names.Order }},
{{- end }}
{{- end }}
) (*{{ $names.Connection }}, error) {
	// Total count before cursor/limit constraints.
	totalCount, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}

	// Apply cursor predicates.
	if after != nil {
		query = query.Where({{ $n.Package }}.IDGT(after.ID))
	}
	if before != nil {
		query = query.Where({{ $n.Package }}.IDLT(before.ID))
	}

	// Apply ordering and limit. Ordering must be applied together with the
	// pagination direction to avoid conflicting orders.
	limit := 0
	if first != nil {
		limit = *first + 1 // +1 to detect whether more items exist
{{- if $gqlgoOrderFields }}
{{- if $multiOrder }}
		if len(orders) > 0 {
			query = Apply{{ $names.Order }}List(query, orders)
		} else {
			query = query.Order({{ $n.Package }}.ByID())
		}
{{- else }}
		if order != nil {
			query = query.Order(order.ToOrderOption())
		} else {
			query = query.Order({{ $n.Package }}.ByID())
		}
{{- end }}
{{- else }}
		query = query.Order({{ $n.Package }}.ByID())
{{- end }}
		query = query.Limit(limit)
	} else if last != nil {
		limit = *last + 1
{{- if $gqlgoOrderFields }}
{{- if $multiOrder }}
		if len(orders) > 0 {
			reversed := make([]*{{ $names.Order }}, len(orders))
			for i, o := range orders {
				reversed[i] = &{{ $names.Order }}{Field: o.Field, Direction: o.Direction.Reverse()}
			}
			query = Apply{{ $names.Order }}List(query, reversed)
		} else {
			query = query.Order({{ $n.Package }}.ByID(sql.OrderDesc()))
		}
{{- else }}
		if order != nil {
			reversed := &{{ $names.Order }}{Field: order.Field, Direction: order.Direction.Reverse()}
			query = query.Order(reversed.ToOrderOption())
		} else {
			query = query.Order({{ $n.Package }}.ByID(sql.OrderDesc()))
		}
{{- end }}
{{- else }}
		query = query.Order({{ $n.Package }}.ByID(sql.OrderDesc()))
{{- end }}
		query = query.Limit(limit)
	} else {
{{- if $gqlgoOrderFields }}
{{- if $multiOrder }}
		if len(orders) > 0 {
			query = Apply{{ $names.Order }}List(query, orders)
		} else {
			query = query.Order({{ $n.Package }}.ByID())
		}
{{- else }}
		if order != nil {
			query = query.Order(order.ToOrderOption())
		} else {
			query = query.Order({{ $n.Package }}.ByID())
		}
{{- end }}
{{- else }}
		query = query.Order({{ $n.Package }}.ByID())
{{- end }}
	}

	nodes, err := query.All(ctx)
	if err != nil {
		return nil, err
	}

	conn := &{{ $names.Connection }}{TotalCount: totalCount}

	// Trim the +1 lookahead row.
	hasMore := len(nodes) > 0 && limit > 0 && len(nodes) == limit
	if hasMore {
		nodes = nodes[:len(nodes)-1]
	}

	// Restore requested order for backward pagination.
	if last != nil {
		for i, j := 0, len(nodes)-1; i < j; i, j = i+1, j-1 {
			nodes[i], nodes[j] = nodes[j], nodes[i]
		}
	}

	conn.Edges = make([]*{{ $names.Edge }}, len(nodes))
	for i, node := range nodes {
		conn.Edges[i] = &{{ $names.Edge }}{
			Node:   node,
			Cursor: entgqlgo.Cursor[{{ $idType }}]{ID: node.ID},
		}
	}

	if len(conn.Edges) > 0 {
		conn.PageInfo.StartCursor = &conn.Edges[0].Cursor
		conn.PageInfo.EndCursor = &conn.Edges[len(conn.Edges)-1].Cursor
	}
	conn.PageInfo.HasNextPage = (first != nil && hasMore) || (last != nil && before != nil)
	conn.PageInfo.HasPreviousPage = (last != nil && hasMore) || (first != nil && after != nil)

	return conn, nil
}
```

Keep everything ABOVE that point in the per-node range (the `{{ $names.Edge }}`/`{{ $names.Connection }}` struct definitions and their `graphql.NewObject` types) unchanged. Keep the file's closing `{{- end }}` (range) and `{{ end }}` (define).

Note the import list at the top of pagination.tmpl already includes `context`, `entgo.io/ent/dialect/sql`, `entgo.io/contrib/entgqlgo`, and the per-node packages — all still needed. Remove the now-unused `"errors"` import ONLY if `validatePaginationArgs` (which uses it) was also removed — it was not, so leave imports alone.

- [ ] **Step 6: Fix RelayConnection edge resolvers in types.tmpl**

In `entgqlgo/template/types.tmpl`, the edge resolver section (lines ~132–168) generates the same plain-list resolver for all edges, but RelayConnection edges have `Type: {{ $edgeNames.Connection }}Type` — a type/value mismatch. Replace the whole resolver-generation block:

```
{{- /* Generate edge resolvers that check for eager-loaded data first */ -}}
{{- range $n := $gqlNodes }}
{{- range $e := gqlgoFilterEdges $n.Edges (gqlgoSkipMode "type") }}

// resolve{{ $n.Name }}{{ $e.StructField }} resolves the {{ $e.Name }} edge for {{ $n.Name }}.
// It checks if the edge was already eager-loaded to avoid N+1 queries.
func resolve{{ $n.Name }}{{ $e.StructField }}(p graphql.ResolveParams) (interface{}, error) {
	... existing body ...
}
{{- end }}
{{- end }}
```

with:

```
{{- /* Generate edge resolvers */ -}}
{{- range $n := $gqlNodes }}
{{- range $e := gqlgoFilterEdges $n.Edges (gqlgoSkipMode "type") }}
{{- $isRC := gqlgoIsRelayConn $e }}
{{- if $isRC }}
{{- $edgeNames := gqlgoNodePaginationNames $e.Type }}
{{- $edgeOrderFields := gqlgoOrderFields $e.Type }}
{{- $edgeMultiOrder := $e.Type.Annotations.EntGQL.MultiOrder }}
{{- $edgeHasWhere := gqlgoHasWhereInput $e }}

// resolve{{ $n.Name }}{{ $e.StructField }} resolves the {{ $e.Name }} edge for {{ $n.Name }}
// as a Relay connection with cursor pagination.
func resolve{{ $n.Name }}{{ $e.StructField }}(p graphql.ResolveParams) (interface{}, error) {
	source, ok := p.Source.(*{{ $entPkg }}.{{ $n.Name }})
	if !ok {
		return nil, nil
	}
	args, err := ParsePaginationArgs(p)
	if err != nil {
		return nil, err
	}
	query := source.Query{{ $e.StructField }}()
	{{- if $edgeHasWhere }}
	// Apply where filter.
	if whereArg, ok := p.Args["where"].(map[string]interface{}); ok {
		whereInput, err := Parse{{ $edgeNames.WhereInput }}(whereArg)
		if err != nil {
			return nil, fmt.Errorf("parsing where input: %w", err)
		}
		query, err = whereInput.Filter(query)
		if err != nil {
			return nil, fmt.Errorf("applying where filter: %w", err)
		}
	}
	{{- end }}
	// Eager-load nested edges selected under edges { node { ... } }.
	query = {{ $e.Type.QueryName }}CollectFieldsConnection(p.Context, p.Info, query)
	{{- if $edgeOrderFields }}
	{{- if $edgeMultiOrder }}
	var orders []*{{ $edgeNames.Order }}
	if orderByArg, ok := p.Args["orderBy"].([]interface{}); ok {
		orders, err = Parse{{ $edgeNames.Order }}List(orderByArg)
		if err != nil {
			return nil, fmt.Errorf("parsing orderBy: %w", err)
		}
	}
	return paginate{{ $e.Type.Name }}Query(p.Context, query, args.After, args.Before, args.First, args.Last, orders)
	{{- else }}
	var order *{{ $edgeNames.Order }}
	if orderByArg, ok := p.Args["orderBy"].(map[string]interface{}); ok {
		order, err = Parse{{ $edgeNames.Order }}(orderByArg)
		if err != nil {
			return nil, fmt.Errorf("parsing orderBy: %w", err)
		}
	}
	return paginate{{ $e.Type.Name }}Query(p.Context, query, args.After, args.Before, args.First, args.Last, order)
	{{- end }}
	{{- else }}
	return paginate{{ $e.Type.Name }}Query(p.Context, query, args.After, args.Before, args.First, args.Last)
	{{- end }}
}
{{- else }}

// resolve{{ $n.Name }}{{ $e.StructField }} resolves the {{ $e.Name }} edge for {{ $n.Name }}.
// It checks if the edge was already eager-loaded to avoid N+1 queries.
func resolve{{ $n.Name }}{{ $e.StructField }}(p graphql.ResolveParams) (interface{}, error) {
	source, ok := p.Source.(*{{ $entPkg }}.{{ $n.Name }})
	if !ok {
		return nil, nil
	}
	{{- if $e.Unique }}
	// Check if edge was already loaded via eager loading
	if edge := source.Edges.{{ $e.StructField }}; edge != nil {
		return edge, nil
	}
	// Fall back to query
	edge, err := source.Query{{ $e.StructField }}().Only(p.Context)
	if err != nil {
		// For optional edges, not found is not an error - return nil
		if {{ $entPkg }}.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return edge, nil
	{{- else }}
	// Check if edge was already loaded via eager loading
	if edges := source.Edges.{{ $e.StructField }}; edges != nil {
		return edges, nil
	}
	// Fall back to query
	return source.Query{{ $e.StructField }}().All(p.Context)
	{{- end }}
}
{{- end }}
{{- end }}
{{- end }}
```

Also add `"fmt"` and `"context"` to types.tmpl's import block if not present (the connection resolver uses `fmt.Errorf`; `context` comes via collection helpers' signature being called with `p.Context`, which needs no import — only add `"fmt"`):

```
import (
	"fmt"

	"{{ $.Config.Package }}"
	"github.com/graphql-go/graphql"
)
```

(The `{{ if $isRC }}` template branch must be checked after generation: if a node has no RelayConnection edges, `fmt` would be an unused import and the generated package won't compile. To avoid this, use the conditional import pattern already used elsewhere in the templates — check how schema.tmpl imports `strconv`/`uuid` unconditionally and follow suit; if `goimports` runs as part of entc generation (it does — entc pipes generated code through `imports.Process`), unused imports are stripped automatically and this is a non-issue. Verify by generating: if compilation fails on imports, entc did not strip them and the import must be made conditional with `{{ if }}` blocks.)

- [ ] **Step 7: Regenerate and inspect**

```bash
cd entgqlgo/internal/todo && go generate ./... && cd ../../..
```
Expected: succeeds. Then verify the four fixes took effect:

```bash
grep -n '"createTodo"\|"createCategory"' entgqlgo/internal/todo/ent/gqlgo/schema.go   # camelCase mutations present
grep -n '"IN_PROGRESS"' entgqlgo/internal/todo/ent/gqlgo/enum.go                      # enum values as keys
grep -n 'func paginateTodoQuery\|func paginateCategoryQuery' entgqlgo/internal/todo/ent/gqlgo/pagination.go  # new paginate funcs
grep -n 'paginateTodoQuery(p.Context' entgqlgo/internal/todo/ent/gqlgo/types.go       # connection edge resolvers
ls entgqlgo/internal/todo/ent/gqlgo/resolvers.go                                       # should NOT exist
```

If `resolvers.go` still exists (entc doesn't delete files for removed templates), delete it:
```bash
rm entgqlgo/internal/todo/ent/gqlgo/resolvers.go
```

Build:
```bash
cd entgqlgo && go build ./... && cd ..
```
Expected: PASS. Fix template syntax errors if not (error messages name the template and line).

- [ ] **Step 8: Update edge-query integration tests**

Edge fields with `RelayConnection()` (`Todo.children`, `Category.todos`) are now Connection-typed. Tests that query them as plain lists fail with errors like `Cannot query field "id" on type "TodoConnection"`.

Run the tests to get the failure list:
```bash
cd entgqlgo && go test ./internal/todo/ -run 'TestTodoSuite|TestQueryCategories|TestEagerLoad|TestNestedEagerLoad|TestNoEagerLoad' 2>&1 | grep -E '^\s+---|FAIL|Cannot query' && cd ..
```

Affected tests (every test whose GraphQL query selects `children { ... }` or a nested `todos { ... }` under a Category selection): `TestQueryCategories` (suite + standalone at line ~323), `TestQueryTodos`/`TestQueryEmpty` if they select nested edges, `TestQuerySingleTodo` (~193), `TestEagerLoadEdges` (~2084), `TestNoEagerLoadWhenNotSelected` (~2158), `TestNestedEagerLoad` (~2228), `TestEagerLoadEdgesList` (~2366), `TestEagerLoadCategoryTodos` (~2472).

Apply this transformation to each affected query and its assertions.

Query transformation:
```graphql
# OLD                                    # NEW
categories {                             categories {
  id                                       id
  text                                     text
  todos {                                  todos {
    id                                       totalCount
    text                                     edges {
  }                                            node {
}                                                id
                                                 text
                                               }
                                             }
                                           }
                                         }
```

Assertion transformation (Go):
```go
// OLD
todos := category["todos"].([]interface{})
first := todos[0].(map[string]interface{})
require.Equal(t, "Todo 1", first["text"])

// NEW
todosConn := category["todos"].(map[string]interface{})
edges := todosConn["edges"].([]interface{})
firstNode := edges[0].(map[string]interface{})["node"].(map[string]interface{})
require.Equal(t, "Todo 1", firstNode["text"])
```

Complete worked example — `TestEagerLoadCategoryTodos` (line ~2472). The test creates a category with todos and queries `categories { id text todos { id text } }`, asserting on `category["todos"].([]interface{})`. It becomes:

```go
func TestEagerLoadCategoryTodos(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	category := client.Category.Create().
		SetText("Work").
		SaveX(ctx)
	for i := 1; i <= 3; i++ {
		client.Todo.Create().
			SetText(fmt.Sprintf("Todo %d", i)).
			SetStatus(todo.StatusInProgress).
			SetCategory(category).
			SaveX(ctx)
	}

	schema, err := gqlgo.NewSchema(client)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			categories {
				id
				text
				todos {
					totalCount
					edges {
						node {
							id
							text
						}
					}
				}
			}
		}`,
		Context: ctx,
	})
	require.Empty(t, result.Errors)

	data := result.Data.(map[string]interface{})
	categories := data["categories"].([]interface{})
	require.Len(t, categories, 1)

	cat := categories[0].(map[string]interface{})
	require.Equal(t, "Work", cat["text"])

	todosConn := cat["todos"].(map[string]interface{})
	require.Equal(t, 3, todosConn["totalCount"])
	edges := todosConn["edges"].([]interface{})
	require.Len(t, edges, 3)
	for i, e := range edges {
		node := e.(map[string]interface{})["node"].(map[string]interface{})
		require.Equal(t, fmt.Sprintf("Todo %d", i+1), node["text"])
	}
}
```

(Adapt the seeding/assertion details to what each existing test actually does — keep each test's intent, change only the query shape and result navigation. The suite-based tests use `s.executeQuery(...)` / `s.Require()` instead of `graphql.Do` / `require`.)

- [ ] **Step 9: Refresh the introspection golden file**

```bash
rm entgqlgo/internal/todo/testdata/schema_introspection.json
rm -f entgqlgo/internal/todo/testdata/schema_introspection_actual.json
cd entgqlgo && go test ./internal/todo/ -run TestSchemaIntrospection -v && cd ..
```
Expected: PASS with log line `Created golden file: testdata/schema_introspection.json`.

Sanity-check the new golden file reflects the fixes:
```bash
grep -c '"name": "createTodo"' entgqlgo/internal/todo/testdata/schema_introspection.json   # 1
grep -c '"name": "TodoConnection"' entgqlgo/internal/todo/testdata/schema_introspection.json # >= 1
grep -c '"name": "Time"' entgqlgo/internal/todo/testdata/schema_introspection.json           # >= 1 (Time scalar)
grep -c '"name": "DateTime"' entgqlgo/internal/todo/testdata/schema_introspection.json       # 0
```

- [ ] **Step 10: Full test run**

```bash
cd entgqlgo && go test ./... && cd ..
```
Expected: `ok` for `entgo.io/contrib/entgqlgo` and `entgo.io/contrib/entgqlgo/internal/todo`. Fix any remaining failures (they will be edge-query tests missed in Step 8 — same transformation).

Also verify regeneration is now a no-op (the invariant this task establishes):
```bash
cd entgqlgo/internal/todo && go generate ./... && cd ../../..
git status --short entgqlgo/
```
Expected: no modified files.

- [ ] **Step 11: Commit**

```bash
git add entgqlgo/ 
```
```bash
git commit -m "fix(entgqlgo): re-sync templates with generated code, make templates source of truth

- camelCase mutation field names (createTodo) for gqlgen parity
- GraphQL enum values use ent values (IN_PROGRESS), not Go names
- replace orphaned client-based Paginate funcs with query-based paginate<T>Query
- RelayConnection edge fields now resolve to real Connection objects
- remove dead ResolversTemplate
- regenerate example app + introspection golden file"
```

---

### Task 2: QueryField gating and plain-list root queries

Root queries are currently generated for every non-skipped type with `first`/`offset` args. entgql generates root queries only for `QueryField()`-annotated types: plain lists (no args) without `RelayConnection()`, connections with it. This task adds the gating and the plain-list shape; Task 3 converts the relay branch.

**Files:**
- Modify: `entgqlgo/template.go` (new template helpers)
- Test: `entgqlgo/template_test.go`
- Modify: `entgqlgo/template/schema.tmpl` (root query gating)
- Create: `entgqlgo/internal/todo/ent/schema/billproduct.go` (plain-list example entity)
- Modify: `entgqlgo/internal/todo/todo_test.go` (new test)
- Regenerate: `entgqlgo/internal/todo/ent/*`, golden file

- [ ] **Step 1: Write failing unit tests for the new template helpers**

Append to `entgqlgo/template_test.go`:

```go
func TestQueryFieldName(t *testing.T) {
	t.Parallel()

	// Node without QueryField annotation -> no root query field.
	node := &gen.Type{Name: "Todo", Annotations: gen.Annotations{}}
	name, err := queryFieldName(node)
	require.NoError(t, err)
	require.Empty(t, name)

	// Node with QueryField() -> default name: camel(snake(plural(type))).
	node = &gen.Type{Name: "Todo", Annotations: gen.Annotations{
		"EntGQL": map[string]interface{}{
			"QueryField": map[string]interface{}{},
		},
	}}
	name, err = queryFieldName(node)
	require.NoError(t, err)
	require.Equal(t, "todos", name)

	// Node with QueryField("customName") -> custom name.
	node = &gen.Type{Name: "Todo", Annotations: gen.Annotations{
		"EntGQL": map[string]interface{}{
			"QueryField": map[string]interface{}{"Name": "allTodos"},
		},
	}}
	name, err = queryFieldName(node)
	require.NoError(t, err)
	require.Equal(t, "allTodos", name)
}

func TestQueryFieldDescription(t *testing.T) {
	t.Parallel()

	node := &gen.Type{Name: "Todo", Annotations: gen.Annotations{
		"EntGQL": map[string]interface{}{
			"QueryField": map[string]interface{}{"Description": "All the todos"},
		},
	}}
	desc, err := queryFieldDescription(node)
	require.NoError(t, err)
	require.Equal(t, "All the todos", desc)
}

func TestIsRelayConnNode(t *testing.T) {
	t.Parallel()

	node := &gen.Type{Name: "Todo", Annotations: gen.Annotations{}}
	rc, err := isRelayConnNode(node)
	require.NoError(t, err)
	require.False(t, rc)

	node = &gen.Type{Name: "Todo", Annotations: gen.Annotations{
		"EntGQL": map[string]interface{}{"RelayConnection": true},
	}}
	rc, err = isRelayConnNode(node)
	require.NoError(t, err)
	require.True(t, rc)
}
```

(Match the existing test file's import style — it already imports `gen` and `require`; check the top of `entgqlgo/template_test.go` and reuse its patterns for constructing `gen.Type`.)

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd entgqlgo && go test . -run 'TestQueryFieldName|TestQueryFieldDescription|TestIsRelayConnNode' -v && cd ..
```
Expected: FAIL — `undefined: queryFieldName`, `undefined: queryFieldDescription`, `undefined: isRelayConnNode`.

- [ ] **Step 3: Implement the helpers**

Append to `entgqlgo/template.go`:

```go
// queryFieldName returns the root Query field name for the node, or "" if the
// node has no QueryField annotation.
func queryFieldName(t *gen.Type) (string, error) {
	gqlType, ant, err := gqlTypeFromNode(t)
	if err != nil {
		return "", err
	}
	if ant.QueryField == nil {
		return "", nil
	}
	return ant.QueryField.fieldName(gqlType), nil
}

// queryFieldDescription returns the description of the node's root Query field.
func queryFieldDescription(t *gen.Type) (string, error) {
	_, ant, err := gqlTypeFromNode(t)
	if err != nil || ant.QueryField == nil {
		return "", err
	}
	return ant.QueryField.Description, nil
}

// isRelayConnNode reports whether the node itself (not an edge) has the
// RelayConnection annotation.
func isRelayConnNode(t *gen.Type) (bool, error) {
	ant, err := annotation(t.Annotations)
	if err != nil {
		return false, err
	}
	return ant.RelayConnection, nil
}
```

Register them in the `TemplateFuncs` map in the same file:

```go
		"gqlgoQueryFieldName":        queryFieldName,
		"gqlgoQueryFieldDescription": queryFieldDescription,
		"gqlgoIsRelayConnNode":       isRelayConnNode,
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd entgqlgo && go test . -run 'TestQueryFieldName|TestQueryFieldDescription|TestIsRelayConnNode' -v && cd ..
```
Expected: PASS.

- [ ] **Step 5: Add the BillProduct example entity (plain-list QueryField)**

Create `entgqlgo/internal/todo/ent/schema/billproduct.go`:

```go
// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package schema

import (
	"entgo.io/contrib/entgqlgo"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// BillProduct defines a simple type exposed as a plain (non-paginated) list,
// mirroring entgql's billProducts example.
type BillProduct struct {
	ent.Schema
}

// Fields returns billproduct fields.
func (BillProduct) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty(),
		field.String("sku").
			NotEmpty(),
		field.Int("quantity").
			Default(0),
	}
}

// Annotations returns BillProduct annotations.
// QueryField without RelayConnection -> plain list root query with no arguments.
func (BillProduct) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgqlgo.QueryField(),
	}
}
```

- [ ] **Step 6: Update the root-query section of schema.tmpl**

In `entgqlgo/template/schema.tmpl`, the root Query loop (starting `{{- range $n := $gqlNodes }}` at line ~52 and ending at the matching `{{- end }}` at line ~142) currently generates one list query per node unconditionally. Wrap it with QueryField gating and split by node-level RelayConnection:

```
			{{- range $n := $gqlNodes }}
			{{- $names := gqlgoNodePaginationNames $n }}
			{{- $gqlgoOrderFields := gqlgoOrderFields $n }}
			{{- $multiOrder := $n.Annotations.EntGQL.MultiOrder }}
			{{- $queryFieldName := gqlgoQueryFieldName $n }}
			{{- if $queryFieldName }}
			{{- $queryFieldDesc := gqlgoQueryFieldDescription $n }}
			{{- $isRelayConn := gqlgoIsRelayConnNode $n }}
			{{- if $isRelayConn }}
			"{{ $queryFieldName }}": &graphql.Field{
				{{/* This branch is the EXISTING list-query field body, moved verbatim.
				     Take the current field body (everything between `"{{ camel (plural $n.Name) }}": &graphql.Field{`
				     and its closing `},` — the Type, Args (first/offset/where/orderBy), and Resolve func)
				     and paste it here unchanged, with exactly two edits:
				     1. The field key is now "{{ $queryFieldName }}" (shown above) instead of
				        "{{ camel (plural $n.Name) }}".
				     2. Delete the old line `Description: "Query all {{ plural $n.Name }}.",` from inside
				        the body — the conditional Description block below replaces it. (Two Description
				        keys in one struct literal will not compile.)
				     Task 3 will replace this entire branch with the Connection shape. */}}
				{{- if $queryFieldDesc }}
				Description: "{{ $queryFieldDesc }}",
				{{- else }}
				Description: "Query all {{ plural $n.Name }}.",
				{{- end }}
			},
			{{- else }}
			"{{ $queryFieldName }}": &graphql.Field{
				Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull({{ $n.Name }}Type))),
				{{- if $queryFieldDesc }}
				Description: "{{ $queryFieldDesc }}",
				{{- end }}
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c := clientFromContext(p.Context, client)
					query := c.{{ $n.Name }}.Query()
					query = {{ $n.QueryName }}CollectFields(p.Context, p.Info, query)
					return query.All(p.Context)
				},
			},
			{{- end }}
			{{- end }}
			{{- end }}
```

Notes:
- The `<<< KEEP ... >>>` marker means: preserve the current Type/Args/Resolve body of the existing list query; only its key and Description handling change in this task.
- The existing Description line `Description: "Query all {{ plural $n.Name }}.",` inside the old body must be REMOVED (it's replaced by the conditional Description block shown above) — otherwise the field has two Description keys and the generated Go won't compile.

- [ ] **Step 7: Regenerate, write integration test, refresh golden**

```bash
cd entgqlgo/internal/todo && go generate ./... && cd ../../..
cd entgqlgo && go build ./... && cd ..
```
Expected: builds. (BillProduct now generates `BillProductType`, where inputs, etc.)

Append to `entgqlgo/internal/todo/todo_test.go`:

```go
// TestPlainListQueryField verifies that a QueryField type without RelayConnection
// is exposed as a plain non-null list with no arguments (entgql parity).
func TestPlainListQueryField(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	for i := 1; i <= 2; i++ {
		client.BillProduct.Create().
			SetName(fmt.Sprintf("Product %d", i)).
			SetSku(fmt.Sprintf("SKU-%d", i)).
			SetQuantity(i * 10).
			SaveX(ctx)
	}

	schema, err := gqlgo.NewSchema(client)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			billProducts {
				id
				name
				sku
				quantity
			}
		}`,
		Context: ctx,
	})
	require.Empty(t, result.Errors)

	data := result.Data.(map[string]interface{})
	products := data["billProducts"].([]interface{})
	require.Len(t, products, 2)
	first := products[0].(map[string]interface{})
	require.Equal(t, "Product 1", first["name"])

	// Plain-list fields accept no arguments (parity with entgql's billProducts).
	result = graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { billProducts(first: 1) { id } }`,
		Context:       ctx,
	})
	require.NotEmpty(t, result.Errors, "billProducts must not accept pagination arguments")
}
```

Run it (must fail BEFORE regen if written first; with codegen tasks the regen happens first, so verify it passes now):
```bash
cd entgqlgo && go test ./internal/todo/ -run TestPlainListQueryField -v && cd ..
```
Expected: PASS.

Refresh golden (schema changed: billProducts added):
```bash
rm entgqlgo/internal/todo/testdata/schema_introspection.json
cd entgqlgo && go test ./internal/todo/ -run TestSchemaIntrospection && cd ..
```

Full run:
```bash
cd entgqlgo && go test ./... && cd ..
```
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add entgqlgo/
```
```bash
git commit -m "feat(entgqlgo): gate root queries on QueryField annotation, add plain-list parity

Types are exposed under Query only when annotated with QueryField().
Without RelayConnection() the field is a plain [T!]! list with no args,
matching entgql's billProducts shape."
```

---

### Task 3: Relay Connection root queries

Convert the RelayConnection branch of root queries from `first`/`offset` lists to real Relay connections, matching entgql's SDL exactly.

**Files:**
- Modify: `entgqlgo/template/schema.tmpl` (relay branch of root queries)
- Modify: `entgqlgo/internal/todo/todo_test.go` (~30 tests change query shape)
- Regenerate: generated code + golden file

- [ ] **Step 1: Write the new connection test first**

Append to `entgqlgo/internal/todo/todo_test.go`:

```go
// TestRootConnectionQuery verifies Relay-style cursor pagination on root queries
// (entgql parity: todos(after, first, before, last, orderBy, where): TodoConnection!).
func TestRootConnectionQuery(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		client.Todo.Create().
			SetText(fmt.Sprintf("Todo %d", i)).
			SetStatus(todo.StatusInProgress).
			SetPriority(i).
			SaveX(ctx)
	}

	schema, err := gqlgo.NewSchema(client)
	require.NoError(t, err)

	// Page 1: first 2.
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(first: 2) {
				totalCount
				edges {
					node { id text }
					cursor
				}
				pageInfo {
					hasNextPage
					hasPreviousPage
					endCursor
				}
			}
		}`,
		Context: ctx,
	})
	require.Empty(t, result.Errors)

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	require.Equal(t, 5, conn["totalCount"])

	edges := conn["edges"].([]interface{})
	require.Len(t, edges, 2)
	require.Equal(t, "Todo 1", edges[0].(map[string]interface{})["node"].(map[string]interface{})["text"])

	pageInfo := conn["pageInfo"].(map[string]interface{})
	require.True(t, pageInfo["hasNextPage"].(bool))
	require.False(t, pageInfo["hasPreviousPage"].(bool))
	endCursor := pageInfo["endCursor"].(string)
	require.NotEmpty(t, endCursor)

	// Page 2: first 2 after endCursor.
	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			todos(first: 2, after: %q) {
				edges { node { text } }
				pageInfo { hasNextPage hasPreviousPage }
			}
		}`, endCursor),
		Context: ctx,
	})
	require.Empty(t, result.Errors)

	data = result.Data.(map[string]interface{})
	conn = data["todos"].(map[string]interface{})
	edges = conn["edges"].([]interface{})
	require.Len(t, edges, 2)
	require.Equal(t, "Todo 3", edges[0].(map[string]interface{})["node"].(map[string]interface{})["text"])

	pageInfo = conn["pageInfo"].(map[string]interface{})
	require.True(t, pageInfo["hasNextPage"].(bool))
	require.True(t, pageInfo["hasPreviousPage"].(bool))

	// where + orderBy still work on connections.
	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(where: {priorityGT: 2}, orderBy: [{field: PRIORITY, direction: DESC}]) {
				totalCount
				edges { node { text } }
			}
		}`,
		Context: ctx,
	})
	require.Empty(t, result.Errors)

	data = result.Data.(map[string]interface{})
	conn = data["todos"].(map[string]interface{})
	require.Equal(t, 3, conn["totalCount"])
	edges = conn["edges"].([]interface{})
	require.Equal(t, "Todo 5", edges[0].(map[string]interface{})["node"].(map[string]interface{})["text"])
}
```

- [ ] **Step 2: Run it to verify it fails**

```bash
cd entgqlgo && go test ./internal/todo/ -run TestRootConnectionQuery -v && cd ..
```
Expected: FAIL — `Cannot query field "totalCount" on type ...` / `Unknown argument "after"` (root todos is still a plain list with first/offset).

- [ ] **Step 3: Replace the relay branch in schema.tmpl**

In the root Query loop of `entgqlgo/template/schema.tmpl`, replace the entire `{{- if $isRelayConn }}` branch body (the preserved first/offset list field from Task 2) with the connection field:

```
			{{- if $isRelayConn }}
			"{{ $queryFieldName }}": &graphql.Field{
				Type: graphql.NewNonNull({{ $names.Connection }}Type),
				{{- if $queryFieldDesc }}
				Description: "{{ $queryFieldDesc }}",
				{{- end }}
				Args: graphql.FieldConfigArgument{
					"after": &graphql.ArgumentConfig{
						Type:        CursorScalar,
						Description: "Returns the elements in the list that come after the specified cursor.",
					},
					"first": &graphql.ArgumentConfig{
						Type:        graphql.Int,
						Description: "Returns the first _n_ elements from the list.",
					},
					"before": &graphql.ArgumentConfig{
						Type:        CursorScalar,
						Description: "Returns the elements in the list that come before the specified cursor.",
					},
					"last": &graphql.ArgumentConfig{
						Type:        graphql.Int,
						Description: "Returns the last _n_ elements from the list.",
					},
					{{- if $gqlgoOrderFields }}
					"orderBy": &graphql.ArgumentConfig{
						{{- if $multiOrder }}
						Type:        graphql.NewList(graphql.NewNonNull({{ $names.Order }}InputType)),
						{{- else }}
						Type:        {{ $names.Order }}InputType,
						{{- end }}
						Description: "Ordering options for {{ plural $n.Name }} returned from the connection.",
					},
					{{- end }}
					"where": &graphql.ArgumentConfig{
						Type:        {{ $names.WhereInput }}Type,
						Description: "Filtering options for {{ plural $n.Name }} returned from the connection.",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					args, err := ParsePaginationArgs(p)
					if err != nil {
						return nil, err
					}
					c := clientFromContext(p.Context, client)
					query := c.{{ $n.Name }}.Query()
					// Apply where filter.
					if whereArg, ok := p.Args["where"].(map[string]interface{}); ok {
						whereInput, err := Parse{{ $names.WhereInput }}(whereArg)
						if err != nil {
							return nil, fmt.Errorf("parsing where input: %w", err)
						}
						query, err = whereInput.Filter(query)
						if err != nil {
							return nil, fmt.Errorf("applying where filter: %w", err)
						}
					}
					// Eager-load edges selected under edges { node { ... } }.
					query = {{ $n.QueryName }}CollectFieldsConnection(p.Context, p.Info, query)
					{{- if $gqlgoOrderFields }}
					{{- if $multiOrder }}
					var orders []*{{ $names.Order }}
					if orderByArg, ok := p.Args["orderBy"].([]interface{}); ok {
						orders, err = Parse{{ $names.Order }}List(orderByArg)
						if err != nil {
							return nil, fmt.Errorf("parsing orderBy: %w", err)
						}
					}
					return paginate{{ $n.Name }}Query(p.Context, query, args.After, args.Before, args.First, args.Last, orders)
					{{- else }}
					var order *{{ $names.Order }}
					if orderByArg, ok := p.Args["orderBy"].(map[string]interface{}); ok {
						order, err = Parse{{ $names.Order }}(orderByArg)
						if err != nil {
							return nil, fmt.Errorf("parsing orderBy: %w", err)
						}
					}
					return paginate{{ $n.Name }}Query(p.Context, query, args.After, args.Before, args.First, args.Last, order)
					{{- end }}
					{{- else }}
					return paginate{{ $n.Name }}Query(p.Context, query, args.After, args.Before, args.First, args.Last)
					{{- end }}
				},
			},
			{{- end }}
```

After this change, the old inline eager-loading block (`fields := entgqlgo.CollectFields(p.Info)` / `contains(...)`) is no longer used by root queries. Check whether the `contains` helper function at the bottom of schema.tmpl and the `entgqlgo.CollectFields` import are still referenced anywhere in the template; if not, remove the `contains` function definition (entc strips unused imports automatically, but unused Go functions in generated code are a compile-time non-issue — still, remove `contains` if orphaned to keep generated output clean).

- [ ] **Step 4: Regenerate and run the new test**

```bash
cd entgqlgo/internal/todo && go generate ./... && cd ../../..
cd entgqlgo && go test ./internal/todo/ -run TestRootConnectionQuery -v && cd ..
```
Expected: PASS.

- [ ] **Step 5: Update existing root-query tests**

All tests querying `todos(...)` / `categories(...)` as plain lists now fail. Affected (from the test inventory): `TestQueryTodos` (suite ~86 + standalone ~274), `TestQueryCategories` (~119, ~323), `TestQueryTodoFields` (~153, ~375), `TestQuerySingleTodo` (~193), `TestQueryTodosPagination` (~227), `TestFilterByStatus` (~445), `TestFilterByText` (~517), `TestFilterAnd` (~589), `TestFilterOr` (~659), `TestFilterNot` (~740), `TestFilterByEdge` (~812), `TestOrderByPriority` (~911), `TestOrderByCreatedAt` (~985), `TestOrderDesc` (~1055), `TestMultiOrder` (~1129), `TestOrderByText` (~1242), `TestOrderWithPagination` (~1315), `TestEagerLoadEdges` (~2084), `TestNoEagerLoadWhenNotSelected` (~2158), `TestNestedEagerLoad` (~2228), `TestEagerLoadEdgesList` (~2366), `TestEagerLoadCategoryTodos` (~2472), `TestNullsDirection` (~2564), `TestCustomScalarDateTime` (~2650), `TestEnumValues` (~2707), `TestSkipWhereInputVerifyGenerated` (~2809), and any mutation test that re-queries via root queries.

Transformation (same as Task 1 Step 8 but at the root):

```graphql
# OLD                                          # NEW
todos(where: {status: COMPLETED}) {            todos(where: {status: COMPLETED}) {
  id                                              edges {
  text                                              node {
}                                                     id
                                                      text
                                                    }
                                                  }
                                                }
```

```go
// OLD assertion navigation:
todos := data["todos"].([]interface{})
item := todos[0].(map[string]interface{})

// NEW:
conn := data["todos"].(map[string]interface{})
edges := conn["edges"].([]interface{})
item := edges[0].(map[string]interface{})["node"].(map[string]interface{})
```

`TestQueryTodosPagination` (~227) uses `todos(first: 2)` and `todos(offset: 3)`. The `offset` variant has no parity equivalent — rewrite that subtest to use cursor pagination (`after:` with the endCursor from the first page), exactly like `TestRootConnectionQuery` does.

Run iteratively until green:
```bash
cd entgqlgo && go test ./internal/todo/ 2>&1 | grep -E '^---|^--- FAIL|FAIL|^ok' && cd ..
```

- [ ] **Step 6: Refresh golden file and full test run**

```bash
rm entgqlgo/internal/todo/testdata/schema_introspection.json
cd entgqlgo && go test ./... && cd ..
```
Expected: all PASS. Verify the golden now shows connection root queries:
```bash
grep -A2 '"name": "todos"' entgqlgo/internal/todo/testdata/schema_introspection.json | head -20
```
Expected: return type `TodoConnection` (NON_NULL wrapper around it).

- [ ] **Step 7: Commit**

```bash
git add entgqlgo/
```
```bash
git commit -m "feat(entgqlgo): Relay Connection root queries for entgql parity

QueryField types with RelayConnection() now generate
todos(after, first, before, last, orderBy, where): TodoConnection!
root queries instead of offset-paginated plain lists."
```

---

### Task 4: Schema extensibility (SchemaConfig)

**Files:**
- Modify: `entgqlgo/template/schema.tmpl` (extract SchemaConfig)
- Create: `entgqlgo/internal/todo/extensibility_test.go`
- Regenerate: generated code (golden file unchanged — no schema shape change)

- [ ] **Step 1: Write failing tests**

Create `entgqlgo/internal/todo/extensibility_test.go`:

```go
// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package todo

import (
	"context"
	"testing"
	"time"

	"entgo.io/contrib/entgqlgo/internal/todo/ent/enttest"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/gqlgo"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/todo"

	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// TestCustomQueryField verifies users can add custom root query fields
// alongside the generated ones (code-first equivalent of extra .graphql files).
func TestCustomQueryField(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	cfg := gqlgo.SchemaConfig(client)
	cfg.Query.AddFieldConfig("ping", &graphql.Field{
		Type: graphql.NewNonNull(graphql.String),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return "pong", nil
		},
	})
	schema, err := graphql.NewSchema(cfg)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { ping }`,
		Context:       context.Background(),
	})
	require.Empty(t, result.Errors)
	data := result.Data.(map[string]interface{})
	require.Equal(t, "pong", data["ping"])

	// Generated fields still work on the same schema.
	result = graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { todos { totalCount } }`,
		Context:       context.Background(),
	})
	require.Empty(t, result.Errors)
}

// TestCustomMutationField verifies users can add custom mutations that use
// generated types and the ent client.
func TestCustomMutationField(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	client.Todo.Create().SetText("a").SetStatus(todo.StatusInProgress).SaveX(ctx)
	client.Todo.Create().SetText("b").SetStatus(todo.StatusInProgress).SaveX(ctx)

	cfg := gqlgo.SchemaConfig(client)
	cfg.Mutation.AddFieldConfig("clearTodos", &graphql.Field{
		Type:        graphql.NewNonNull(graphql.Int),
		Description: "Delete all todos and return the number deleted.",
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return client.Todo.Delete().Exec(p.Context)
		},
	})
	schema, err := graphql.NewSchema(cfg)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { clearTodos }`,
		Context:       ctx,
	})
	require.Empty(t, result.Errors)
	data := result.Data.(map[string]interface{})
	require.Equal(t, 2, data["clearTodos"])
	require.Zero(t, client.Todo.Query().CountX(ctx))
}

// TestCustomSubscription verifies users can attach a Subscription root type and
// stream events via graphql.Subscribe.
func TestCustomSubscription(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	events := make(chan interface{}, 1)
	cfg := gqlgo.SchemaConfig(client)
	cfg.Subscription = graphql.NewObject(graphql.ObjectConfig{
		Name: "Subscription",
		Fields: graphql.Fields{
			"todoEvents": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return p.Source, nil
				},
				Subscribe: func(p graphql.ResolveParams) (interface{}, error) {
					return events, nil
				},
			},
		},
	})
	schema, err := graphql.NewSchema(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := graphql.Subscribe(graphql.Params{
		Schema:        schema,
		RequestString: `subscription { todoEvents }`,
		Context:       ctx,
	})

	events <- "todo-created"

	select {
	case res := <-results:
		require.Empty(t, res.Errors)
		data := res.Data.(map[string]interface{})
		require.Equal(t, "todo-created", data["todoEvents"])
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for subscription event")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd entgqlgo && go test ./internal/todo/ -run 'TestCustomQueryField|TestCustomMutationField|TestCustomSubscription' -v && cd ..
```
Expected: FAIL — `gqlgo.SchemaConfig undefined`.

- [ ] **Step 3: Add SchemaConfig to schema.tmpl**

In `entgqlgo/template/schema.tmpl`, replace the `NewSchema` function:

```
// NewSchema creates a new GraphQL schema with Query and Mutation types.
// The client is used for database operations in resolvers.
func NewSchema(client *{{ $entPkg }}.Client) (graphql.Schema, error) {
	return graphql.NewSchema(graphql.SchemaConfig{
		Query:    newQueryType(client),
		Mutation: newMutationType(client),
	})
}
```

with:

```
// SchemaConfig returns the graphql.SchemaConfig used to build the generated schema.
// Callers may modify the returned config before building the schema with
// graphql.NewSchema — e.g. add custom query/mutation fields with
// cfg.Query.AddFieldConfig(...) or attach a Subscription root type.
func SchemaConfig(client *{{ $entPkg }}.Client) graphql.SchemaConfig {
	return graphql.SchemaConfig{
		Query:    newQueryType(client),
		Mutation: newMutationType(client),
	}
}

// NewSchema creates a new GraphQL schema with Query and Mutation types.
// The client is used for database operations in resolvers.
func NewSchema(client *{{ $entPkg }}.Client) (graphql.Schema, error) {
	return graphql.NewSchema(SchemaConfig(client))
}
```

- [ ] **Step 4: Regenerate and run tests**

```bash
cd entgqlgo/internal/todo && go generate ./... && cd ../../..
cd entgqlgo && go test ./internal/todo/ -run 'TestCustomQueryField|TestCustomMutationField|TestCustomSubscription' -v && cd ..
```
Expected: PASS.

```bash
cd entgqlgo && go test ./... && cd ..
```
Expected: all PASS (introspection golden unchanged — `SchemaConfig` adds no schema types).

- [ ] **Step 5: Commit**

```bash
git add entgqlgo/
```
```bash
git commit -m "feat(entgqlgo): expose SchemaConfig for custom queries, mutations, and subscriptions"
```

---

### Task 5: Transactions

entgql's `Transactioner` wraps mutations in a transaction at the gqlgen middleware layer. graphql-go has no middleware, so wrapping happens at resolver level in generated code.

**Files:**
- Modify: `entgqlgo/template/schema.tmpl` (SchemaOption, WithTransactions, WithTx, wrap mutation resolvers)
- Create: `entgqlgo/internal/todo/transaction_test.go`
- Regenerate: generated code

- [ ] **Step 1: Write failing tests**

Create `entgqlgo/internal/todo/transaction_test.go`:

```go
// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package todo

import (
	"context"
	"errors"
	"testing"

	"entgo.io/contrib/entgqlgo/internal/todo/ent/enttest"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/gqlgo"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/todo"

	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// TestWithTxCommit verifies WithTx commits the transaction when the wrapped
// resolver succeeds.
func TestWithTxCommit(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	cfg := gqlgo.SchemaConfig(client)
	cfg.Mutation.AddFieldConfig("seedTodo", &graphql.Field{
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: gqlgo.WithTx(client, func(p graphql.ResolveParams) (interface{}, error) {
			// The transactional client is in the context; generated resolvers and
			// custom code both retrieve it the same way.
			tc := gqlgo.ClientFromContext(p.Context, client)
			_, err := tc.Todo.Create().
				SetText("inside tx").
				SetStatus(todo.StatusInProgress).
				Save(p.Context)
			return err == nil, err
		}),
	})
	schema, err := graphql.NewSchema(cfg)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { seedTodo }`,
		Context:       ctx,
	})
	require.Empty(t, result.Errors)
	require.Equal(t, 1, client.Todo.Query().CountX(ctx), "committed row must be visible")
}

// TestWithTxRollback verifies WithTx rolls back the transaction when the
// wrapped resolver returns an error.
func TestWithTxRollback(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	cfg := gqlgo.SchemaConfig(client)
	cfg.Mutation.AddFieldConfig("failingMutation", &graphql.Field{
		Type: graphql.Boolean,
		Resolve: gqlgo.WithTx(client, func(p graphql.ResolveParams) (interface{}, error) {
			tc := gqlgo.ClientFromContext(p.Context, client)
			// Write succeeds inside the tx...
			_, err := tc.Todo.Create().
				SetText("will be rolled back").
				SetStatus(todo.StatusInProgress).
				Save(p.Context)
			require.NoError(t, err)
			// ...then the resolver fails.
			return nil, errors.New("boom")
		}),
	})
	schema, err := graphql.NewSchema(cfg)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { failingMutation }`,
		Context:       ctx,
	})
	require.NotEmpty(t, result.Errors)
	require.Zero(t, client.Todo.Query().CountX(ctx), "rolled-back row must not be visible")
}

// TestGeneratedMutationsInTx verifies WithTransactions() wraps generated
// mutation resolvers so they run inside a transaction.
func TestGeneratedMutationsInTx(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	schema, err := gqlgo.NewSchema(client, gqlgo.WithTransactions())
	require.NoError(t, err)

	// A successful generated mutation commits.
	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { createTodo(input: {text: "tx todo", status: IN_PROGRESS}) { id text } }`,
		Context:       ctx,
	})
	require.Empty(t, result.Errors)
	require.Equal(t, 1, client.Todo.Query().CountX(ctx))

	// A failing generated mutation (validation error: empty text) leaves no row.
	result = graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { createTodo(input: {text: "", status: IN_PROGRESS}) { id } }`,
		Context:       ctx,
	})
	require.NotEmpty(t, result.Errors)
	require.Equal(t, 1, client.Todo.Query().CountX(ctx), "failed mutation must not add rows")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd entgqlgo && go test ./internal/todo/ -run 'TestWithTx|TestGeneratedMutationsInTx' -v && cd ..
```
Expected: FAIL — `gqlgo.WithTx undefined`, `gqlgo.ClientFromContext undefined`, `gqlgo.WithTransactions undefined`.

- [ ] **Step 3: Add transaction support to schema.tmpl**

Three changes in `entgqlgo/template/schema.tmpl`:

**(a)** Rename/export the context helper. Replace:

```
// clientFromContext returns the client from context if available, otherwise falls back to the provided client.
// This enables transactional operations when the context contains a transactional client.
func clientFromContext(ctx context.Context, fallback *{{ $entPkg }}.Client) *{{ $entPkg }}.Client {
	if c := {{ $entPkg }}.FromContext(ctx); c != nil {
		return c
	}
	return fallback
}
```

with:

```
// ClientFromContext returns the client stored in the context if present,
// otherwise falls back to the provided client. This enables transactional
// operations when the context carries a transactional client (see WithTx).
func ClientFromContext(ctx context.Context, fallback *{{ $entPkg }}.Client) *{{ $entPkg }}.Client {
	if c := {{ $entPkg }}.FromContext(ctx); c != nil {
		return c
	}
	return fallback
}

// clientFromContext is kept as an internal alias used by generated resolvers.
func clientFromContext(ctx context.Context, fallback *{{ $entPkg }}.Client) *{{ $entPkg }}.Client {
	return ClientFromContext(ctx, fallback)
}
```

**(b)** Add schema options + WithTx after the `NewSchema` function:

```
// SchemaOption configures the generated schema.
type SchemaOption func(*schemaOptions)

type schemaOptions struct {
	transactions bool
}

// WithTransactions wraps every generated mutation resolver in a database
// transaction. The transactional client is propagated via context, so all ent
// operations inside the resolver participate in the same transaction.
// Equivalent to entgql's Transactioner middleware.
func WithTransactions() SchemaOption {
	return func(o *schemaOptions) {
		o.transactions = true
	}
}

// WithTx wraps a resolver so it runs inside a transaction opened on client.
// On success the transaction is committed; on error or panic it is rolled back.
// The transactional client is stored in the resolver context and can be
// retrieved with ClientFromContext.
func WithTx(client *{{ $entPkg }}.Client, resolve graphql.FieldResolveFn) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (result interface{}, err error) {
		tx, err := client.Tx(p.Context)
		if err != nil {
			return nil, fmt.Errorf("opening transaction: %w", err)
		}
		defer func() {
			if r := recover(); r != nil {
				_ = tx.Rollback()
				panic(r)
			}
		}()
		p.Context = {{ $entPkg }}.NewContext(p.Context, tx.Client())
		result, err = resolve(p)
		if err != nil {
			if rerr := tx.Rollback(); rerr != nil {
				err = fmt.Errorf("%w: rolling back transaction: %v", err, rerr)
			}
			return nil, err
		}
		if cerr := tx.Commit(); cerr != nil {
			return nil, fmt.Errorf("committing transaction: %w", cerr)
		}
		return result, nil
	}
}

// maybeWithTx wraps the resolver in a transaction when transactions are enabled.
func maybeWithTx(client *{{ $entPkg }}.Client, o *schemaOptions, resolve graphql.FieldResolveFn) graphql.FieldResolveFn {
	if o == nil || !o.transactions {
		return resolve
	}
	return WithTx(client, resolve)
}
```

**(c)** Thread options through. Update signatures and call sites:

```
func SchemaConfig(client *{{ $entPkg }}.Client, opts ...SchemaOption) graphql.SchemaConfig {
	o := &schemaOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return graphql.SchemaConfig{
		Query:    newQueryType(client),
		Mutation: newMutationType(client, o),
	}
}

func NewSchema(client *{{ $entPkg }}.Client, opts ...SchemaOption) (graphql.Schema, error) {
	return graphql.NewSchema(SchemaConfig(client, opts...))
}
```

Change `newMutationType`'s signature from `func newMutationType(client *{{ $entPkg }}.Client) *graphql.Object` to `func newMutationType(client *{{ $entPkg }}.Client, o *schemaOptions) *graphql.Object`, and wrap each of the three generated mutation resolvers (create/update/delete) by changing:

```
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
```

to:

```
				Resolve: maybeWithTx(client, o, func(p graphql.ResolveParams) (interface{}, error) {
```

and the matching closing brace of each resolver from `				},` to `				}),`. (Three resolvers: `"create{{ $n.Name }}"`, `"update{{ $n.Name }}"`, `"delete{{ $n.Name }}"`.)

- [ ] **Step 4: Regenerate and run tests**

```bash
cd entgqlgo/internal/todo && go generate ./... && cd ../../..
cd entgqlgo && go test ./internal/todo/ -run 'TestWithTx|TestGeneratedMutationsInTx' -v && cd ..
```
Expected: PASS.

```bash
cd entgqlgo && go test ./... && cd ..
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add entgqlgo/
```
```bash
git commit -m "feat(entgqlgo): add transaction support (WithTransactions, WithTx)

Generated mutation resolvers can opt into automatic transaction wrapping,
matching entgql's Transactioner. Custom resolvers use gqlgo.WithTx directly."
```

---

### Task 6: Implements annotation (custom interfaces)

**Files:**
- Modify: `entgqlgo/annotation.go` (Implements field + constructor + Merge)
- Test: `entgqlgo/annotation_test.go`
- Modify: `entgqlgo/template/types.tmpl` (InterfacesThunk + CustomInterfaces map)
- Modify: `entgqlgo/internal/todo/ent/schema/category.go` (example usage)
- Modify: `entgqlgo/internal/todo/introspection_test.go` and `todo_test.go` (shared schema helper)
- Create: `entgqlgo/internal/todo/implements_test.go`
- Regenerate: generated code + golden file

- [ ] **Step 1: Write failing annotation unit test**

Append to `entgqlgo/annotation_test.go`:

```go
func TestImplementsAnnotation(t *testing.T) {
	t.Parallel()

	a := Implements("NamedNode", "Entity")
	require.Equal(t, []string{"NamedNode", "Entity"}, a.Implements)

	// Merge accumulates.
	merged := Implements("NamedNode").Merge(Implements("Entity")).(Annotation)
	require.Equal(t, []string{"NamedNode", "Entity"}, merged.Implements)

	// JSON round-trip uses the same key as entgql for annotation compatibility.
	decoded := Annotation{}
	require.NoError(t, decoded.Decode(map[string]interface{}{
		"Implements": []interface{}{"NamedNode"},
	}))
	require.Equal(t, []string{"NamedNode"}, decoded.Implements)
}
```

Run:
```bash
cd entgqlgo && go test . -run TestImplementsAnnotation -v && cd ..
```
Expected: FAIL — `undefined: Implements` / `a.Implements undefined`.

- [ ] **Step 2: Implement the annotation**

In `entgqlgo/annotation.go`:

1. Add to the `Annotation` struct (after the `UseEnumNames` field):
```go
		// Implements defines a list of additional GraphQL interfaces (besides Node)
		// implemented by the type. Interface definitions must be registered in the
		// generated package's CustomInterfaces map before building the schema.
		Implements []string `json:"Implements,omitempty"`
```

2. Add the constructor (after `UseEnumNames()`):
```go
// Implements returns an annotation stating the type implements the given
// custom GraphQL interfaces, in addition to the Node interface.
func Implements(interfaces ...string) Annotation {
	return Annotation{Implements: interfaces}
}
```

3. Add merge handling inside `Merge` (after the `UseEnumNames` block):
```go
	if len(ant.Implements) > 0 {
		a.Implements = append(a.Implements, ant.Implements...)
	}
```

Run:
```bash
cd entgqlgo && go test . -run TestImplementsAnnotation -v && cd ..
```
Expected: PASS.

- [ ] **Step 3: Wire interfaces into types.tmpl**

In `entgqlgo/template/types.tmpl`:

1. Add the registry var right after the existing `var (...)` block of type declarations:

```
// CustomInterfaces maps interface names referenced by entgqlgo.Implements
// annotations to their graphql.Interface definitions. Populate this map before
// building the schema (it is read lazily when graphql.NewSchema is called).
var CustomInterfaces = map[string]*graphql.Interface{}
```

2. Replace the static interface list in each object config:

```
		Interfaces: []*graphql.Interface{
			NodeInterface,
		},
```

with a thunk that appends registered custom interfaces:

```
		Interfaces: graphql.InterfacesThunk(func() []*graphql.Interface {
			ifaces := []*graphql.Interface{NodeInterface}
			{{- range $iface := $n.Annotations.EntGQL.Implements }}
			if iface, ok := CustomInterfaces["{{ $iface }}"]; ok {
				ifaces = append(ifaces, iface)
			}
			{{- end }}
			return ifaces
		}),
```

Note: `$n.Annotations.EntGQL.Implements` is nil-safe in templates only when the node HAS an EntGQL annotation (all `$gqlNodes` in the example do, since they carry QueryField/Skip annotations). For robustness add a template helper instead if generation fails on a node without any EntGQL annotation: `gqlgoImplements` returning `ant.Implements` via the `annotation()` func — same pattern as `isRelayConnNode` from Task 2.

- [ ] **Step 4: Use it in the example schema and add a shared schema helper**

In `entgqlgo/internal/todo/ent/schema/category.go`, add to `Annotations()`:

```go
		entgqlgo.Implements("NamedNode"),
```

Because `CustomInterfaces` is package-level state read at schema build time, ALL tests must register the example's interfaces before building a schema, or the introspection golden file becomes order-dependent. Create the helper in `entgqlgo/internal/todo/schema_helper_test.go`:

```go
// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package todo

import (
	"entgo.io/contrib/entgqlgo/internal/todo/ent"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/gqlgo"

	"github.com/graphql-go/graphql"
)

// namedNodeInterface is the example custom interface implemented by Category
// via the entgqlgo.Implements("NamedNode") annotation.
var namedNodeInterface = graphql.NewInterface(graphql.InterfaceConfig{
	Name:        "NamedNode",
	Description: "An object with a text name.",
	Fields: graphql.Fields{
		"text": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
	},
	ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
		switch p.Value.(type) {
		case *ent.Category:
			return gqlgo.CategoryType
		default:
			return nil
		}
	},
})

func init() {
	gqlgo.CustomInterfaces["NamedNode"] = namedNodeInterface
}

// newTestSchema builds the generated schema with the example's custom
// interfaces registered. All tests in this package use this instead of
// calling gqlgo.NewSchema directly, so schema shape is consistent.
func newTestSchema(client *ent.Client, opts ...gqlgo.SchemaOption) (graphql.Schema, error) {
	return gqlgo.NewSchema(client, opts...)
}
```

Then update `todo_test.go`, `introspection_test.go`, `extensibility_test.go`, and `transaction_test.go`: replace every `gqlgo.NewSchema(client)` / `gqlgo.NewSchema(s.client)` call with `newTestSchema(client)` / `newTestSchema(s.client)` (the `init()` registration makes this mostly a consistency measure — the registration applies process-wide regardless; the helper exists so future per-schema setup has one home). For `gqlgo.SchemaConfig(...)` call sites, no change needed — registration is global.

- [ ] **Step 5: Write the integration test**

Create `entgqlgo/internal/todo/implements_test.go`:

```go
// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package todo

import (
	"context"
	"testing"

	"entgo.io/contrib/entgqlgo/internal/todo/ent/enttest"

	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// TestImplementsInterface verifies that Category (annotated with
// entgqlgo.Implements("NamedNode")) exposes the interface in the schema and
// supports inline fragments on it.
func TestImplementsInterface(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	client.Category.Create().SetText("Work").SaveX(ctx)

	schema, err := newTestSchema(client)
	require.NoError(t, err)

	// Introspection: Category lists NamedNode among its interfaces.
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			__type(name: "Category") {
				interfaces { name }
			}
		}`,
		Context: ctx,
	})
	require.Empty(t, result.Errors)

	data := result.Data.(map[string]interface{})
	typeInfo := data["__type"].(map[string]interface{})
	interfaces := typeInfo["interfaces"].([]interface{})

	names := make([]string, 0, len(interfaces))
	for _, i := range interfaces {
		names = append(names, i.(map[string]interface{})["name"].(string))
	}
	require.Contains(t, names, "Node")
	require.Contains(t, names, "NamedNode")

	// Inline fragments on the custom interface work.
	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			categories {
				edges {
					node {
						... on NamedNode {
							text
						}
					}
				}
			}
		}`,
		Context: ctx,
	})
	require.Empty(t, result.Errors)
	conn := result.Data.(map[string]interface{})["categories"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	require.Len(t, edges, 1)
	node := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	require.Equal(t, "Work", node["text"])
}
```

- [ ] **Step 6: Regenerate, run tests, refresh golden**

```bash
cd entgqlgo/internal/todo && go generate ./... && cd ../../..
cd entgqlgo && go test ./internal/todo/ -run TestImplementsInterface -v && cd ..
```
Expected: PASS.

The golden file changes (Category now lists NamedNode interface):
```bash
rm entgqlgo/internal/todo/testdata/schema_introspection.json
cd entgqlgo && go test ./... && cd ..
```
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add entgqlgo/
```
```bash
git commit -m "feat(entgqlgo): add Implements annotation for custom GraphQL interfaces"
```

---

### Task 7: DeprecatedEnumValues annotation

**Files:**
- Modify: `entgqlgo/annotation.go` (DeprecatedEnumValues field + constructor + Merge)
- Test: `entgqlgo/annotation_test.go`
- Modify: `entgqlgo/template.go` (gqlgoIsDeprecatedEnumValue helper)
- Modify: `entgqlgo/template/enum.tmpl` (DeprecationReason)
- Modify: `entgqlgo/internal/todo/ent/schema/category.go` (example usage)
- Create: test in `entgqlgo/internal/todo/todo_test.go`
- Regenerate: generated code + golden file

- [ ] **Step 1: Write failing unit tests**

Append to `entgqlgo/annotation_test.go`:

```go
func TestDeprecatedEnumValuesAnnotation(t *testing.T) {
	t.Parallel()

	a := DeprecatedEnumValues("DISABLED")
	require.Equal(t, []string{"DISABLED"}, a.DeprecatedEnumValues)

	merged := DeprecatedEnumValues("A").Merge(DeprecatedEnumValues("B")).(Annotation)
	require.Equal(t, []string{"A", "B"}, merged.DeprecatedEnumValues)

	decoded := Annotation{}
	require.NoError(t, decoded.Decode(map[string]interface{}{
		"DeprecatedEnumValues": []interface{}{"DISABLED"},
	}))
	require.Equal(t, []string{"DISABLED"}, decoded.DeprecatedEnumValues)
}
```

Append to `entgqlgo/template_test.go`:

```go
func TestIsDeprecatedEnumValue(t *testing.T) {
	t.Parallel()

	f := &gen.Field{Annotations: gen.Annotations{
		"EntGQL": map[string]interface{}{
			"DeprecatedEnumValues": []interface{}{"DISABLED"},
		},
	}}
	dep, err := isDeprecatedEnumValue(f, "DISABLED")
	require.NoError(t, err)
	require.True(t, dep)

	dep, err = isDeprecatedEnumValue(f, "ENABLED")
	require.NoError(t, err)
	require.False(t, dep)
}
```

Run:
```bash
cd entgqlgo && go test . -run 'TestDeprecatedEnumValues|TestIsDeprecatedEnumValue' -v && cd ..
```
Expected: FAIL — undefined symbols.

- [ ] **Step 2: Implement annotation + helper**

In `entgqlgo/annotation.go`:

1. Struct field (after `Implements`):
```go
		// DeprecatedEnumValues is a list of enum VALUES that should be marked
		// as deprecated in the GraphQL schema.
		DeprecatedEnumValues []string `json:"DeprecatedEnumValues,omitempty"`
```

2. Constructor:
```go
// DeprecatedEnumValues returns an annotation marking the given enum values as
// deprecated in the GraphQL schema.
func DeprecatedEnumValues(values ...string) Annotation {
	return Annotation{DeprecatedEnumValues: values}
}
```

3. Merge handling:
```go
	if len(ant.DeprecatedEnumValues) > 0 {
		a.DeprecatedEnumValues = append(a.DeprecatedEnumValues, ant.DeprecatedEnumValues...)
	}
```

In `entgqlgo/template.go`:

```go
// isDeprecatedEnumValue reports whether the given enum value is listed in the
// field's DeprecatedEnumValues annotation.
func isDeprecatedEnumValue(f *gen.Field, value string) (bool, error) {
	ant, err := annotation(f.Annotations)
	if err != nil {
		return false, err
	}
	return slices.Contains(ant.DeprecatedEnumValues, value), nil
}
```

Register in `TemplateFuncs`:
```go
		"gqlgoIsDeprecatedEnumValue": isDeprecatedEnumValue,
```

Run:
```bash
cd entgqlgo && go test . -run 'TestDeprecatedEnumValues|TestIsDeprecatedEnumValue' -v && cd ..
```
Expected: PASS.

- [ ] **Step 3: Wire into enum.tmpl**

In `entgqlgo/template/enum.tmpl`, update both branches of the value generation to include the deprecation reason. The default (non-UseEnumNames) branch becomes:

```
		{{- else }}
		"{{ $e.Value }}": &graphql.EnumValueConfig{
			Value: "{{ $e.Value }}",
			{{- if gqlgoIsDeprecatedEnumValue $f $e.Value }}
			DeprecationReason: "No longer supported",
			{{- end }}
		},
		{{- end }}
```

and the UseEnumNames branch gets the same `{{- if gqlgoIsDeprecatedEnumValue $f $e.Value }}DeprecationReason: ...{{- end }}` lines added inside its `EnumValueConfig`.

- [ ] **Step 4: Example usage + integration test**

In `entgqlgo/internal/todo/ent/schema/category.go`, update the `status` field:

```go
		field.Enum("status").
			NamedValues(
				"Enabled", "ENABLED",
				"Disabled", "DISABLED",
			).
			Default("ENABLED").
			Annotations(
				entgqlgo.DeprecatedEnumValues("DISABLED"),
			),
```

Append to `entgqlgo/internal/todo/todo_test.go`:

```go
// TestDeprecatedEnumValue verifies enum values listed in DeprecatedEnumValues
// are marked deprecated in the schema.
func TestDeprecatedEnumValue(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			__type(name: "CategoryStatus") {
				enumValues(includeDeprecated: true) {
					name
					isDeprecated
					deprecationReason
				}
			}
		}`,
		Context: context.Background(),
	})
	require.Empty(t, result.Errors)

	values := result.Data.(map[string]interface{})["__type"].(map[string]interface{})["enumValues"].([]interface{})
	byName := map[string]map[string]interface{}{}
	for _, v := range values {
		vm := v.(map[string]interface{})
		byName[vm["name"].(string)] = vm
	}
	require.False(t, byName["ENABLED"]["isDeprecated"].(bool))
	require.True(t, byName["DISABLED"]["isDeprecated"].(bool))
	require.Equal(t, "No longer supported", byName["DISABLED"]["deprecationReason"])
}
```

- [ ] **Step 5: Regenerate, test, refresh golden, commit**

```bash
cd entgqlgo/internal/todo && go generate ./... && cd ../../..
cd entgqlgo && go test ./internal/todo/ -run TestDeprecatedEnumValue -v && cd ..
```
Expected: PASS.

```bash
rm entgqlgo/internal/todo/testdata/schema_introspection.json
cd entgqlgo && go test ./... && cd ..
```
Expected: all PASS.

```bash
git add entgqlgo/
```
```bash
git commit -m "feat(entgqlgo): add DeprecatedEnumValues annotation"
```

---

### Task 8: Parity acceptance test

Build the same ent schema with BOTH extensions and mechanically compare the schemas. entgql writes SDL (`ent.graphql`); entgqlgo's schema is introspected at runtime. Differences must be in an explicit allowlist.

**Files:**
- Create: `entgqlgo/internal/parity/gen.go`
- Create: `entgqlgo/internal/parity/ent/entc.go`
- Create: `entgqlgo/internal/parity/ent/schema/todo.go`
- Create: `entgqlgo/internal/parity/ent/schema/category.go`
- Create: `entgqlgo/internal/parity/parity_test.go`
- Generated: `entgqlgo/internal/parity/ent/*` (both extensions' output)

- [ ] **Step 1: Create the shared ent schema (annotated with entgql annotations only)**

Both extensions read the `"EntGQL"` annotation key, so a single schema annotated with `entgql.*` drives both generators.

Create `entgqlgo/internal/parity/ent/schema/category.go`:

```go
// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Category is annotated with entgql annotations only; both entgql and entgqlgo
// read the shared "EntGQL" annotation key.
type Category struct {
	ent.Schema
}

func (Category) Fields() []ent.Field {
	return []ent.Field{
		field.String("text").
			NotEmpty().
			Annotations(entgql.OrderField("TEXT")),
		field.Enum("status").
			NamedValues(
				"Enabled", "ENABLED",
				"Disabled", "DISABLED",
			).
			Default("ENABLED"),
	}
}

func (Category) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("todos", Todo.Type).
			Annotations(entgql.RelayConnection()),
	}
}

func (Category) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entgql.Mutations(entgql.MutationCreate(), entgql.MutationUpdate()),
	}
}
```

Create `entgqlgo/internal/parity/ent/schema/todo.go`:

```go
// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package schema

import (
	"time"

	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Todo mirrors the entgqlgo example schema but uses entgql annotations.
type Todo struct {
	ent.Schema
}

func (Todo) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(
				entgql.OrderField("CREATED_AT"),
				entgql.Skip(entgql.SkipMutationCreateInput),
			),
		field.Enum("status").
			NamedValues(
				"InProgress", "IN_PROGRESS",
				"Completed", "COMPLETED",
				"Pending", "PENDING",
			).
			Annotations(entgql.OrderField("STATUS")),
		field.Int("priority").
			Default(0).
			Annotations(entgql.OrderField("PRIORITY")),
		field.Text("text").
			NotEmpty().
			Annotations(entgql.OrderField("TEXT")),
	}
}

func (Todo) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("children", Todo.Type).
			Annotations(entgql.RelayConnection()).
			From("parent").
			Unique(),
		edge.From("category", Category.Type).
			Ref("todos").
			Unique(),
	}
}

func (Todo) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entgql.Mutations(entgql.MutationCreate(), entgql.MutationUpdate()),
		entgql.MultiOrder(),
	}
}
```

- [ ] **Step 2: Create the dual-extension codegen entrypoint**

Create `entgqlgo/internal/parity/gen.go`:

```go
// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parity

//go:generate go run ./ent/entc.go
```

Create `entgqlgo/internal/parity/ent/entc.go`:

```go
// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build ignore
// +build ignore

package main

import (
	"log"

	"entgo.io/contrib/entgql"
	"entgo.io/contrib/entgqlgo"
	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	// entgql writes the SDL to ent.graphql; entgqlgo generates the gqlgo package.
	// Both run over the same schema in one Generate call.
	gqlEx, err := entgql.NewExtension(
		entgql.WithSchemaGenerator(),
		entgql.WithSchemaPath("./ent.graphql"),
		entgql.WithWhereInputs(true),
	)
	if err != nil {
		log.Fatalf("creating entgql extension: %v", err)
	}
	gqlgoEx, err := entgqlgo.NewExtension()
	if err != nil {
		log.Fatalf("creating entgqlgo extension: %v", err)
	}
	err = entc.Generate("./ent/schema", &gen.Config{}, entc.Extensions(gqlEx, gqlgoEx))
	if err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
```

Generate:
```bash
cd entgqlgo/internal/parity && go generate ./... && cd ../../..
cd entgqlgo && go build ./... && cd ..
```
Expected: succeeds, producing `entgqlgo/internal/parity/ent/` (ent client + entgql's gql_*.go + gqlgo/ subpackage) and `entgqlgo/internal/parity/ent.graphql`.

If the two extensions conflict in one Generate call (template name or feature collisions), fall back to two sequential `entc.Generate` calls in the same `main()` — first with `entc.Extensions(gqlEx)`, then with `entc.Extensions(gqlgoEx)` — both targeting `./ent/schema`. The second run regenerates the ent client identically and adds the gqlgo package.

- [ ] **Step 3: Write the comparison test**

Create `entgqlgo/internal/parity/parity_test.go`:

```go
// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parity

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"entgo.io/contrib/entgqlgo/internal/parity/ent/enttest"
	"entgo.io/contrib/entgqlgo/internal/parity/ent/gqlgo"

	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// allowlistedDifferences are schema elements that intentionally differ between
// entgql+gqlgen and entgqlgo. Each entry documents why.
var allowlistedDifferences = map[string]string{
	// gqlgen binding directives have no graphql-go equivalent (code-first).
	"directive:goField":  "gqlgen-specific binding directive",
	"directive:goModel":  "gqlgen-specific binding directive",
	// graphql-go has no SDL extend mechanism; Query helper fields differ.
	"scalar:Map":  "entgql maps JSON fields to Map scalar; entgqlgo uses String unless annotated",
	"scalar:Uint64": "entgql custom scalar; not used by the parity schema",
}

// fieldSignature renders "name(arg:Type, ...): ReturnType" for comparison.
func fieldSignature(name string, args []string, ret string) string {
	sort.Strings(args)
	return fmt.Sprintf("%s(%s): %s", name, strings.Join(args, ", "), ret)
}

// sdlQueryFields parses ent.graphql and returns the signature set of root Query fields.
func sdlQueryFields(t *testing.T) map[string]bool {
	t.Helper()
	sdl, err := os.ReadFile("ent.graphql")
	require.NoError(t, err)

	doc, gqlErr := gqlparser.LoadSchema(&ast.Source{Name: "ent.graphql", Input: string(sdl)})
	require.Nil(t, gqlErr)

	fields := map[string]bool{}
	for _, f := range doc.Query.Fields {
		if strings.HasPrefix(f.Name, "__") {
			continue
		}
		args := make([]string, 0, len(f.Arguments))
		for _, a := range f.Arguments {
			args = append(args, fmt.Sprintf("%s:%s", a.Name, a.Type.String()))
		}
		fields[fieldSignature(f.Name, args, f.Type.String())] = true
	}
	return fields
}

// introspectionQueryFields introspects the gqlgo schema and returns the
// signature set of root Query fields, rendered in SDL type notation.
func introspectionQueryFields(t *testing.T) map[string]bool {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := gqlgo.NewSchema(client)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			__schema {
				queryType {
					fields {
						name
						args { name type { ...T } }
						type { ...T }
					}
				}
			}
		}
		fragment T on __Type {
			kind name
			ofType { kind name ofType { kind name ofType { kind name } } }
		}`,
		Context: context.Background(),
	})
	require.Empty(t, result.Errors)

	fields := map[string]bool{}
	queryType := result.Data.(map[string]interface{})["__schema"].(map[string]interface{})["queryType"].(map[string]interface{})
	for _, f := range queryType["fields"].([]interface{}) {
		fm := f.(map[string]interface{})
		name := fm["name"].(string)
		args := []string{}
		for _, a := range fm["args"].([]interface{}) {
			am := a.(map[string]interface{})
			args = append(args, fmt.Sprintf("%s:%s", am["name"].(string), typeRefToSDL(am["type"].(map[string]interface{}))))
		}
		fields[fieldSignature(name, args, typeRefToSDL(fm["type"].(map[string]interface{})))] = true
	}
	return fields
}

// typeRefToSDL converts an introspection type ref to SDL notation ([Todo!]!, Cursor, etc).
func typeRefToSDL(ref map[string]interface{}) string {
	kind, _ := ref["kind"].(string)
	switch kind {
	case "NON_NULL":
		return typeRefToSDL(ref["ofType"].(map[string]interface{})) + "!"
	case "LIST":
		return "[" + typeRefToSDL(ref["ofType"].(map[string]interface{})) + "]"
	default:
		name, _ := ref["name"].(string)
		return name
	}
}

// TestRootQueryParity compares root Query fields between entgql's SDL and
// entgqlgo's introspected schema.
func TestRootQueryParity(t *testing.T) {
	sdlFields := sdlQueryFields(t)
	gqlgoFields := introspectionQueryFields(t)

	var missing, extra []string
	for sig := range sdlFields {
		if !gqlgoFields[sig] {
			missing = append(missing, sig)
		}
	}
	for sig := range gqlgoFields {
		if !sdlFields[sig] {
			extra = append(extra, sig)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	// Filter out allowlisted differences (match by field name prefix).
	missing = filterAllowlisted(missing)
	extra = filterAllowlisted(extra)

	if len(missing) > 0 || len(extra) > 0 {
		t.Errorf("Root Query parity mismatch.\nIn entgql SDL but not in entgqlgo:\n  %s\nIn entgqlgo but not in entgql SDL:\n  %s",
			strings.Join(missing, "\n  "), strings.Join(extra, "\n  "))
	}
}

func filterAllowlisted(sigs []string) []string {
	out := make([]string, 0, len(sigs))
	for _, s := range sigs {
		allowed := false
		for key := range allowlistedDifferences {
			name := strings.TrimPrefix(strings.TrimPrefix(key, "field:"), "directive:")
			if strings.HasPrefix(s, name+"(") || strings.HasPrefix(s, name+":") {
				allowed = true
				break
			}
		}
		if !allowed {
			out = append(out, s)
		}
	}
	return out
}
```

- [ ] **Step 4: Run the parity test and triage**

```bash
cd entgqlgo && go test ./internal/parity/ -run TestRootQueryParity -v && cd ..
```

Two possible outcomes:

**(a) PASS** — root queries are at parity. Done.

**(b) FAIL with a mismatch list** — each line is either:
   - A real entgqlgo bug → fix the template (in `entgqlgo/template/`), regenerate BOTH example apps (`internal/todo` and `internal/parity`), and re-run.
   - An intentional difference (e.g. extra `deleteTodo` mutations entgqlgo generates that entgql doesn't, naming-convention deltas in nested arg types) → add to `allowlistedDifferences` with a one-line justification.

Iterate until PASS. Known likely differences to expect and triage:
   - entgqlgo generates `deleteX` mutations; entgql does not → allowlist (entgqlgo superset).
   - Argument descriptions differ → signatures above ignore descriptions, no action.
   - `orderBy` nullability/list-wrapping differences → real bugs; fix templates.

Mutation parity: add a second test `TestRootMutationParity` to the same file. It is the Query test with three substitutions — copy `sdlQueryFields` to `sdlMutationFields` and change `doc.Query.Fields` to `doc.Mutation.Fields`; copy `introspectionQueryFields` to `introspectionMutationFields` and change `queryType` to `mutationType` in both the GraphQL request string and the result-navigation line; copy `TestRootQueryParity` to `TestRootMutationParity` calling the two new helpers. Then add to the allowlist:

```go
	// entgqlgo generates delete mutations; entgql does not (superset, not a gap).
	"field:deleteTodo":     "entgqlgo generates delete mutations",
	"field:deleteCategory": "entgqlgo generates delete mutations",
```

- [ ] **Step 5: Full test run and commit**

```bash
cd entgqlgo && go test ./... && cd ..
```
Expected: all PASS.

```bash
git add entgqlgo/
```
```bash
git commit -m "test(entgqlgo): add parity acceptance test against entgql SDL

Generates the same ent schema with both extensions and mechanically diffs
root Query/Mutation fields, with an explicit allowlist for intentional
differences."
```

---

### Task 9: README

**Files:**
- Create: `entgqlgo/README.md`

- [ ] **Step 1: Write the README**

Create `entgqlgo/README.md` with this structure and content (expand each code block from the working example in `internal/todo/`):

````markdown
# entgqlgo

An [ent](https://entgo.io) extension that generates a code-first GraphQL API for
[github.com/graphql-go/graphql](https://github.com/graphql-go/graphql).

It is the graphql-go counterpart of [entgql](../entgql): instead of generating
`.graphql` SDL files and gqlgen bindings, it generates Go code that builds a
`graphql.Schema` at runtime. Ent's generated client does all querying — the
generated resolvers only parse GraphQL arguments, call ent, and shape results.

## Quick start

1. Annotate your ent schema:

```go
func (Todo) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgqlgo.RelayConnection(),
        entgqlgo.QueryField(),
        entgqlgo.Mutations(entgqlgo.MutationCreate(), entgqlgo.MutationUpdate()),
    }
}
```

2. Register the extension in your entc.go:

```go
ex, err := entgqlgo.NewExtension()
// ...
err = entc.Generate("./ent/schema", &gen.Config{}, entc.Extensions(ex))
```

3. Serve the generated schema:

```go
schema, err := gqlgo.NewSchema(client)
h := handler.New(&handler.Config{Schema: &schema, GraphiQL: true})
http.Handle("/graphql", h)
```

## Annotations

| Annotation | Applies to | Effect |
|---|---|---|
| `QueryField()` | type | Expose the type as a root Query field |
| `RelayConnection()` | type | Root query field is a Relay Connection (cursor pagination) |
| `RelayConnection()` | edge | Edge field is a Relay Connection |
| `Mutations(MutationCreate(), MutationUpdate())` | type | Generate create/update/delete mutations |
| `OrderField("NAME")` | field | Field can be used in orderBy |
| `MultiOrder()` | type | orderBy accepts a list of order terms |
| `Skip(...)` | type/field | Exclude from schema (SkipType, SkipWhereInput, ...) |
| `Type("Name")` | field | Override the GraphQL type |
| `UseEnumNames()` | enum field | GraphQL enum values use Go names instead of values |
| `Implements("Iface")` | type | Type implements a custom interface (register in `CustomInterfaces`) |
| `DeprecatedEnumValues("VAL")` | enum field | Mark enum values as deprecated |

<each annotation gets a before/after schema snippet — copy the shapes from
internal/todo's introspection golden file>

## Pagination

Types/edges with `RelayConnection()` follow the Relay Cursor Connections spec:
`todos(after, first, before, last, orderBy, where): TodoConnection!` with
`edges { node cursor }`, `pageInfo`, and `totalCount`.

## Filtering and ordering

WhereInput types are generated by default (`WithWhereInputs(false)` to disable).
`orderBy` accepts `{field, direction}` terms; with `MultiOrder()`, a list of them.

## Extending the schema

```go
cfg := gqlgo.SchemaConfig(client)
cfg.Query.AddFieldConfig("ping", &graphql.Field{ /* ... */ })
cfg.Mutation.AddFieldConfig("clearTodos", &graphql.Field{ /* ... */ })
cfg.Subscription = graphql.NewObject(/* ... use graphql.Subscribe to serve */)
schema, err := graphql.NewSchema(cfg)
```

## Transactions

```go
// All generated mutations run in a transaction:
schema, err := gqlgo.NewSchema(client, gqlgo.WithTransactions())

// Custom mutations:
Resolve: gqlgo.WithTx(client, func(p graphql.ResolveParams) (interface{}, error) {
    tc := gqlgo.ClientFromContext(p.Context, client)
    // all tc operations are transactional
}),
```

## Differences from entgql + gqlgen

| Feature | entgql+gqlgen | entgqlgo |
|---|---|---|
| Schema definition | SDL files (`.graphql`) + gqlgen codegen | Go code, built at runtime |
| Custom directives (`@hasPermissions`) | Supported via SDL | Not supported — graphql-go fields cannot carry applied directives; wrap resolvers instead |
| Query complexity limits | gqlgen runtime feature | Not available |
| `@goField` / `@goModel` | gqlgen binding directives | Not applicable |
| Schema extension | extra `.graphql` files | `SchemaConfig()` + `AddFieldConfig` |
| Delete mutations | not generated | generated for `Mutations()` types |

## Regenerating the example

```bash
cd internal/todo && go generate ./...
cd internal/parity && go generate ./...
```

Templates in `template/` are the source of truth; never edit files under
`internal/*/ent/gqlgo/` by hand.
````

Replace the `<each annotation gets...>` placeholder with actual before/after snippets pulled from the working example before committing — every annotation row in the table must have a corresponding example in the doc body.

- [ ] **Step 2: Verify all README code snippets compile against the example**

Every Go snippet in the README must match real API. Cross-check:
```bash
grep -n "func NewSchema\|func SchemaConfig\|func WithTransactions\|func WithTx\|func ClientFromContext" entgqlgo/internal/todo/ent/gqlgo/schema.go
grep -n "func QueryField\|func RelayConnection\|func Implements\|func DeprecatedEnumValues" entgqlgo/annotation.go
```
Expected: every symbol referenced in the README appears in these listings.

- [ ] **Step 3: Commit**

```bash
git add entgqlgo/README.md
```
```bash
git commit -m "docs(entgqlgo): add README with setup, annotations, and entgql differences"
```

---

## Final verification (after all tasks)

```bash
# Everything passes:
cd entgqlgo && go test ./... && cd ..

# Templates are the source of truth (zero diff after regen):
cd entgqlgo/internal/todo && go generate ./... && cd ../../..
cd entgqlgo/internal/parity && go generate ./... && cd ../../..
git status --short entgqlgo/
# Expected: empty

# The whole repo still builds:
go build ./...
```
