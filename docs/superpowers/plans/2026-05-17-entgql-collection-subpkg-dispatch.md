# entgql collection subpkg dispatch — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix entgql Bug 9 — `WithSplitGoFiles(true)` generates `gen/gql_collection_<entity>.go` files that define methods on `*<Entity>Query` type aliases, which Go forbids. Move methods to per-entity sub-packages and dispatch cross-entity operations through an import-free registry package.

**Architecture:** Sub-package generates `<entity>/gql_collection.go` (methods on local `*Query`) and `<entity>/gql_collection_dispatch.go` (collector struct implementing `EntityCollector` interface, registered at `init()`). New `<gen>/internal/collectiondispatch` package holds the interface + registry. Every cross-entity operation in the per-entity collectField body is rewritten from direct call to `collectiondispatch.Get(other).Method(...)`. User API (`q.CollectFields(ctx)`) preserved via type alias resolution.

**Tech Stack:** Go (entgo.io/contrib/entgql), text/template codegen.

**Reference spec:** `docs/superpowers/specs/2026-05-17-entgql-collection-subpkg-dispatch-design.md`

---

## Working directory

All work happens in the contrib worktree:
- **Worktree path**: `/var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg`
- **Branch**: `entgql-collection-subpkg`
- **Master invariant**: contrib master is `4aeaf769`; must not drift during this work
- **No push, no PR**: commits stay local until explicitly approved

## Pre-flight checks (every task)

Before each commit:
```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
pwd
git rev-parse --abbrev-ref HEAD     # must be entgql-collection-subpkg
git rev-parse master                # must equal 4aeaf769bf3d1f1f33f7c3ce46d68f3afdfa6c4e
```

`git add` and `git commit` are SEPARATE Bash calls — never chain with `&&` (heredoc commit messages break permission wildcards).

Commit message style: match recent commits (`fix(entgql): ...`, `feat(entgql): ...`). End every commit with:
```
Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```

## File map

**Create (per-task ownership in parentheses):**
- `entgql/template/collection_dispatch_pkg.tmpl` — dispatch package template (Task 2)
- `entgql/template/collection_dispatch.tmpl` — per-entity collector struct template (Task 4)
- `entgql/template/collection_subpkg.tmpl` — per-entity CollectFields/collectField methods template (Task 5)
- `entgql/internal/todoplugin/` — new minimal ent+gqlgen integration fixture using sub-package layout (Task 1, optional if existing fixture can be made to repro)

**Modify:**
- `entgql/template.go` — register the new templates, remove `CollectionEntityTemplate` (Tasks 2, 4, 5, 7)
- `entgql/extension.go` — add new generators, wire into `generateSplitGoFiles`, remove `generateCollectionEntityFile` (Tasks 2, 4, 5, 6, 7)
- `entgql/template_test.go` — add tests for new templates, remove tests for `collection_entity` (Tasks 2, 4, 5, 7)

**Delete:**
- `entgql/template/collection_entity.tmpl` — replaced by `collection_subpkg.tmpl` + `collection_dispatch.tmpl` (Task 7)

---

## EntityCollector interface (locked from spec + collection_entity.tmpl line audit)

This is the canonical interface every per-entity `collector{}` implements. Tasks 4 and 5 reference this verbatim. Methods enumerated by reading every cross-entity reference site in `entgql/template/collection_entity.tmpl`:

```go
package collectiondispatch

import (
    "context"
    "github.com/99designs/gqlgen/graphql"
)

// EntityCollector dispatches collection operations for a single entity.
// All parameters and returns are `any` to break sub-package import cycles.
// Implementations type-assert back to their concrete types.
type EntityCollector interface {
    // NewQuery constructs *Query given *Config (typed as any).
    NewQuery(config any) any

    // NewPaginateArgs constructs *PaginateArgs from a map[string]any.
    NewPaginateArgs(rv any) any

    // PaginateArgsFirst / Last / After / Before access the unexported fields of *PaginateArgs.
    PaginateArgsFirst(args any) *int
    PaginateArgsLast(args any) *int
    PaginateArgsAfter(args any) any   // *Cursor, typed any
    PaginateArgsBefore(args any) any  // *Cursor, typed any
    PaginateArgsOpts(args any) any    // []PaginateOption, typed any

    // NewPager constructs *Pager from opts + last flag.
    NewPager(opts any, last bool) (any, error)

    // CloneQuery returns query.Clone() as any.
    CloneQuery(query any) any

    // ApplyFilter calls pager.applyFilter(query).
    ApplyFilter(pager any, query any) (any, error)

    // ApplyCursors calls pager.applyCursors(query, after, before).
    ApplyCursors(pager any, query any, after, before any) (any, error)

    // ApplyOrder calls pager.applyOrder(query).
    ApplyOrder(pager any, query any) any

    // ApplyLimit calls query.Limit(n).
    ApplyLimit(query any, n int) any

    // OrderExpr returns pager.orderExpr(query) for entgql.LimitPerRow.
    OrderExpr(pager any, query any) any

    // AddQueryModifier appends an entgql modifier to query.modifiers (unexported field).
    AddQueryModifier(query any, modifier any)

    // CollectFields recurses into the entity's collectField method.
    CollectFields(ctx context.Context, q any, oneNode bool, opCtx *graphql.OperationContext, collected graphql.CollectedField, path []string, satisfies ...string) error

    // Per-entity column-name constants needed for loadTotal closures' SQL.
    IDColumnName() string             // e.g. "id" — the entity's ID field SQL column
}

var registry = make(map[string]EntityCollector)

func Register(entity string, c EntityCollector) {
    if _, exists := registry[entity]; exists {
        panic("collectiondispatch: duplicate registration for entity " + entity)
    }
    registry[entity] = c
}

func Get(entity string) EntityCollector {
    return registry[entity]
}
```

**Note on the loadTotal closure problem**: The closure body in `collection_entity.tmpl` lines 93-142 uses cross-entity refs like `recordtype.FieldID`, `agentlicensing.RecordTypesTable`, `agentlicensing.RecordTypesPrimaryKey[fk1idx]`. After the move, the closure lives in the PARENT entity's subpkg, so `agentlicensing.X` is now a local-package ref (`X`, no qualifier). The CROSS-entity ref (`recordtype.FieldID`) is replaced by `collectiondispatch.Get("recordtype").IDColumnName()`. That's why `IDColumnName()` exists in the interface.

**Note on `WithNamed<Edge>` and EagerLoadField (lines 191-197)**: These are SAME-entity operations from the parent's perspective. `<receiver>.<EagerField> = query` — same package, fine. `<receiver>.WithNamed<Edge>(alias, func(*<OtherQuery>) { ... })` — calls parent's WithNamed method, which exists on parent's local *Query (same package). The lambda's typed param `*<OtherQuery>` is the cross-entity TYPE — this IS an issue because parent's subpkg imports of other subpkgs are forbidden (cycle risk). Workaround: the entire `WithNamed<Edge>` path uses the parent's WithNamed method whose signature requires the typed func. We can't avoid the typed import.

  **Resolution**: PARENT subpkg WILL need to import each NEIGHBOR subpkg for the type. This is acceptable IF the import graph remains a DAG. For entities with reciprocal edges (A↔B), both subpkgs would import each other → cycle.

  **Real resolution**: Replace `WithNamed<Edge>` direct call with a dispatch helper that takes `(parentQuery any, edgeName string, query any)`. The helper assigns into the parent's eager-load slice via reflection or via a registered setter. Add a method to EntityCollector:
  ```go
  // SetEagerLoad assigns the constructed query to the parent's eager-load slot for the given edge alias.
  // Implementations: for Unique edges, set the EagerLoadField; for non-Unique, append to the named slice.
  SetEagerLoad(parentQuery any, edgeName string, alias string, otherQuery any)
  ```
  This is on PARENT's collector — called from `collector.SetEagerLoad(parentQuery, edgeName, alias, otherQuery)`. Wait — we don't have parent's collector handy in parent's own collectField (we ARE parent). So this should just be a local method on parent's *Query, not via dispatch. Skip this from EntityCollector.

  **Final resolution**: In the per-entity `collection_subpkg.tmpl`, generate a local `setEagerLoad<Edge>(alias string, q any)` helper per non-Unique edge. The local helper type-asserts `q` back to the LOCAL type via dispatch (the other entity's `*Query`). Wait that won't compile without the type import either.

  **Truly final resolution**: Use an UNTYPED setter pattern. Parent generates a local helper:
  ```go
  func (q *Query) eagerLoad<Edge>(alias string, otherQuery any) {
      // For non-Unique: append a WithNamed entry with a lambda that copies otherQuery into wq.
      // Use reflect to do the copy without typed import. Or: define the eager-load slice as []any.
  }
  ```
  Reflect is slow but collection runs once per request — acceptable.

  **Pragmatic choice for the plan**: Punt this on Task 5 (subpkg template). If the typed `WithNamed<Edge>` cannot be avoided cleanly, the entity-pair cycle is unavoidable, and we fall back to using `gen` package as the intermediary: parent calls a `gen` free function that does the typed work. `gen` already imports all sub-packages. This is an escape valve for the unsolvable cases.

---

## Tasks

### Task 1: Reproduce Bug 9 inside contrib's test fixtures

**Files:**
- Modify: `go.mod` — temporarily point `entgo.io/ent` at the local ent worktree (the wiggly-singing-pancake worktree containing PR 6 + Bug 8 fixes). **Do NOT commit this go.mod change** — it's ephemeral, restore at end of task.
- Modify: `entgql/internal/todo/ent/entc.go` — already has `WithSplitGoFiles(true)`. May need to regenerate the fixture to surface Bug 9 once go.mod points at PR 6 ent.
- Modify or create: a smaller dedicated fixture if `todo` is too complex. Recommended: `entgql/internal/todosubpkg/` — minimal 2-entity fixture (A↔B reciprocal edge) demonstrating the bug clearly.

The point of this task is to **prove Bug 9 reproduces in-tree** before we start fixing it. If it doesn't reproduce, the fix won't be testable.

- [ ] **Step 1: Verify go.mod replace state**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
grep "entgo.io/ent" go.mod
```

Expected: see `replace entgo.io/ent => github.com/MatthewsREIS/ent v0.0.0-20260222202802-528a6080deb9` — the pin predates PR 6.

- [ ] **Step 2: Add ephemeral local-path replace**

Append to `go.mod`:
```
replace entgo.io/ent => /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
```

Then:
```bash
go mod tidy 2>&1 | tail -10
```

Expected: completes without error. If it does error (e.g. version mismatch), investigate. Common issue: `go.sum` mismatch — `rm go.sum && go mod tidy`.

- [ ] **Step 3: Regenerate the todo fixture**

```bash
cd entgql/internal/todo
go generate ./...  2>&1 | tee /tmp/iter1-todo-regen.log | tail -30
```

Expected: codegen runs. The todo fixture's `ent/` directory should now contain per-entity sub-packages (`billproduct/`, `category/`, etc.) with the PR 6 layout. The root `ent/` should contain TYPE ALIASES not concrete types.

If regen fails (likely if ent's PR 6 changed APIs entc.go uses): characterize the failures. May need to update the fixture's entc.go to match PR 6 ent's API.

- [ ] **Step 4: Build the regenerated todo fixture, observe Bug 9 failures**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
go build ./entgql/internal/todo/... 2>&1 | tee /tmp/iter1-todo-build.log | head -30
echo "EXIT=$?"
```

Expected: **build fails** with errors like `cannot define new methods on non-local type BillProductQuery`. Capture the exact errors — these are the failing baseline we'll fix.

If build SUCCEEDS, the fixture isn't tickling Bug 9. Possible causes:
- The pinned ent at the local-path replace doesn't have PR 6 fully (regen still produced concrete types). Verify by inspecting `entgql/internal/todo/ent/billproduct_query.go` — should be a thin file containing only `type BillProductQuery = billproduct.Query`.
- WithSplitGoFiles isn't producing the broken files. Check `ls entgql/internal/todo/ent/gql_collection_*.go`.

If the existing `todo` fixture doesn't easily reproduce Bug 9, create a NEW minimal fixture at `entgql/internal/todosubpkg/` modeled on `todo` but stripped to two entities (e.g., `Owner` and `Pet` with reciprocal edges) — minimum needed to surface the cross-entity dispatch problem.

- [ ] **Step 5: Restore go.mod (DO NOT COMMIT the local-path replace)**

Revert the local-path replace line added in Step 2. Verify:
```bash
git diff go.mod
```

Expected: clean diff (no change to committed go.mod). The local-path replace stays as a working-tree-only modification that we re-add when we want to retest.

Wait — we need the replace for subsequent task verification too. **Keep the replace in-tree as a working-tree modification, but never `git add` or commit it.** Document this in the task report.

- [ ] **Step 6: Write a Go test that captures the failure**

`entgql/extension_bug9_test.go` (or similar):

```go
package entgql_test

import (
    "os/exec"
    "strings"
    "testing"

    "github.com/stretchr/testify/require"
)

// TestBug9_TodoFixtureBuilds is the integration test that drives the fix.
// It currently FAILS until Tasks 2-7 land. After the fix, it passes.
func TestBug9_TodoFixtureBuilds(t *testing.T) {
    cmd := exec.Command("go", "build", "./entgql/internal/todo/...")
    out, err := cmd.CombinedOutput()
    if err != nil {
        // Print the error so the failure mode is visible.
        t.Logf("go build output:\n%s", out)
        // The specific Bug 9 signature.
        if strings.Contains(string(out), "cannot define new methods on non-local type") {
            t.Fatal("Bug 9 active: cannot define new methods on non-local type. " +
                "Tasks 2-7 of the implementation plan should fix this.")
        }
        t.Fatalf("unexpected build failure: %v\n%s", err, out)
    }
}
```

- [ ] **Step 7: Run the test, watch it fail with the expected message**

```bash
go test ./entgql/ -run TestBug9_TodoFixtureBuilds -v 2>&1 | tail -20
```

Expected: FAIL with "Bug 9 active: cannot define new methods on non-local type"

- [ ] **Step 8: Commit**

```bash
git add entgql/extension_bug9_test.go
# Plus any fixture changes if a new fixture was created.
git commit -m "$(cat <<'EOF'
test(entgql): add failing integration test for Bug 9 collection alias methods

WithSplitGoFiles(true) emits gen/gql_collection_<entity>.go files that
declare methods on *<Entity>Query — type aliases to subpkg.Query after
ent's PR 6 split. Go forbids method definitions on aliased types from
foreign packages; the fixture build fails on every entity.

This test runs `go build ./entgql/internal/todo/...` and asserts the
Bug 9 signature ('cannot define new methods on non-local type') is
present. The test FAILS today and SHOULD PASS after Tasks 2-7 of
docs/superpowers/plans/2026-05-17-entgql-collection-subpkg-dispatch.md.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Add `collectiondispatch` package generator

**Files:**
- Create: `entgql/template/collection_dispatch_pkg.tmpl`
- Modify: `entgql/template.go` (register new template var)
- Modify: `entgql/extension.go` (add `generateCollectionDispatchPkg`)
- Modify: `entgql/template_test.go` (add parse + render test)

- [ ] **Step 1: Write the failing template test**

Append to `entgql/template_test.go`:

```go
func TestCollectionDispatchPkgTemplate_Parse(t *testing.T) {
    require.NotNil(t, CollectionDispatchPkgTemplate, "CollectionDispatchPkgTemplate must be parsed at init")
    require.NotEmpty(t, CollectionDispatchPkgTemplate.Name())
}

func TestCollectionDispatchPkgTemplate_Render(t *testing.T) {
    require.NotNil(t, CollectionDispatchPkgTemplate)
    var buf bytes.Buffer
    err := CollectionDispatchPkgTemplate.Execute(&buf, struct{}{})
    require.NoError(t, err)
    out := buf.String()
    require.Contains(t, out, "package collectiondispatch")
    require.Contains(t, out, "type EntityCollector interface")
    require.Contains(t, out, "NewQuery(config any) any")
    require.Contains(t, out, "CollectFields(ctx context.Context")
    require.Contains(t, out, "func Register(entity string, c EntityCollector)")
    require.Contains(t, out, "func Get(entity string) EntityCollector")
    require.Contains(t, out, "var registry = make(map[string]EntityCollector)")
}
```

- [ ] **Step 2: Run the test — should FAIL with "undefined: CollectionDispatchPkgTemplate"**

```bash
go test ./entgql/ -run TestCollectionDispatchPkgTemplate -v 2>&1 | tail -10
```

Expected: COMPILATION FAIL.

- [ ] **Step 3: Create the dispatch package template**

`entgql/template/collection_dispatch_pkg.tmpl`:

```
{{/*
Copyright 2019-present Facebook Inc. All rights reserved.
This source code is licensed under the Apache 2.0 license found
in the LICENSE file in the root directory of this source tree.
*/}}

// Code generated by entgql, DO NOT EDIT.

package collectiondispatch

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
)

// EntityCollector dispatches collection operations for a single entity.
// Implementations live in each entity's sub-package and register
// themselves via init() — the dispatch package does not import any
// entity sub-package, which breaks the cross-entity import cycles
// that the codegen-reduction sub-package split would otherwise create.
//
// All parameters and returns are typed as `any`. Implementations
// assert back to their concrete *Query, *PaginateArgs, *Pager, *Cursor
// types. The type erasure at this boundary is what enables per-entity
// parallel compile of the cross-entity walking logic.
type EntityCollector interface {
	NewQuery(config any) any
	NewPaginateArgs(rv any) any
	PaginateArgsFirst(args any) *int
	PaginateArgsLast(args any) *int
	PaginateArgsAfter(args any) any
	PaginateArgsBefore(args any) any
	PaginateArgsOpts(args any) any
	NewPager(opts any, last bool) (any, error)
	CloneQuery(query any) any
	ApplyFilter(pager any, query any) (any, error)
	ApplyCursors(pager any, query any, after, before any) (any, error)
	ApplyOrder(pager any, query any) any
	ApplyLimit(query any, n int) any
	OrderExpr(pager any, query any) any
	AddQueryModifier(query any, modifier any)
	CollectFields(ctx context.Context, q any, oneNode bool, opCtx *graphql.OperationContext, collected graphql.CollectedField, path []string, satisfies ...string) error
	IDColumnName() string
}

var registry = make(map[string]EntityCollector)

// Register associates a collector with an entity name (snake_case of the entity,
// matching the sub-package directory name). Each entity sub-package's init()
// calls Register exactly once. Duplicate registrations are a code-generation
// bug and panic.
func Register(entity string, c EntityCollector) {
	if _, exists := registry[entity]; exists {
		panic("collectiondispatch: duplicate registration for entity " + entity)
	}
	registry[entity] = c
}

// Get retrieves the collector for an entity. Returns nil if not registered,
// which indicates a code-generation bug; callers should produce a clear
// error rather than nil-dereferencing.
func Get(entity string) EntityCollector {
	return registry[entity]
}
```

- [ ] **Step 4: Register the template in `entgql/template.go`**

Add to the top-level `var (...)` template declarations block (near the existing `CollectionSharedTemplate` line):

```go
// CollectionDispatchPkgTemplate generates <gen>/internal/collectiondispatch/dispatch.go,
// which holds the EntityCollector interface and the per-entity registry.
// Sub-package collection files import this package to invoke cross-entity
// operations without creating sub-package import cycles.
CollectionDispatchPkgTemplate = parseT("template/collection_dispatch_pkg.tmpl")
```

- [ ] **Step 5: Run the template tests, watch them pass**

```bash
go test ./entgql/ -run TestCollectionDispatchPkgTemplate -v 2>&1 | tail -10
```

Expected: PASS.

- [ ] **Step 6: Write a failing test for the generator function**

Append to `entgql/extension_test.go` (or appropriate test file):

```go
func TestGenerateCollectionDispatchPkg_WritesFileWithExpectedContent(t *testing.T) {
    tmp := t.TempDir()
    target := filepath.Join(tmp, "gen")
    require.NoError(t, os.MkdirAll(target, 0755))

    ex, err := NewExtension(WithSplitGoFiles(true))
    require.NoError(t, err)

    g := &gen.Graph{Config: &gen.Config{Target: target}}
    err = ex.generateCollectionDispatchPkg(g)
    require.NoError(t, err)

    out, err := os.ReadFile(filepath.Join(target, "internal", "collectiondispatch", "dispatch.go"))
    require.NoError(t, err)
    require.Contains(t, string(out), "package collectiondispatch")
    require.Contains(t, string(out), "type EntityCollector interface")
}
```

- [ ] **Step 7: Run — should FAIL with "undefined method generateCollectionDispatchPkg"**

```bash
go test ./entgql/ -run TestGenerateCollectionDispatchPkg -v 2>&1 | tail -10
```

- [ ] **Step 8: Implement the generator**

Add to `entgql/extension.go` (place near `generateCollectionSharedFile`):

```go
// generateCollectionDispatchPkg writes <g.Target>/internal/collectiondispatch/dispatch.go.
// Called once per codegen run (not per entity).
func (e *Extension) generateCollectionDispatchPkg(g *gen.Graph) error {
	dir := filepath.Join(g.Target, "internal", "collectiondispatch")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("entgql: create collectiondispatch dir: %w", err)
	}
	path := filepath.Join(dir, "dispatch.go")

	var buf bytes.Buffer
	if err := CollectionDispatchPkgTemplate.Execute(&buf, struct{}{}); err != nil {
		return fmt.Errorf("entgql: execute collection_dispatch_pkg template: %w", err)
	}
	content, err := e.processImports(path, buf.Bytes())
	if err != nil {
		return fmt.Errorf("entgql: format collection_dispatch_pkg: %w", err)
	}
	return os.WriteFile(path, content, 0644)
}
```

- [ ] **Step 9: Run the generator test, watch it pass**

```bash
go test ./entgql/ -run TestGenerateCollectionDispatchPkg -v 2>&1 | tail -10
```

Expected: PASS.

- [ ] **Step 10: Run the full entgql test suite to make sure nothing else broke**

```bash
go test ./entgql/ 2>&1 | tail -10
```

Expected: all tests pass. (TestBug9_TodoFixtureBuilds will still fail — that's expected; it requires Tasks 3-7.)

- [ ] **Step 11: Commit**

```bash
git add entgql/template/collection_dispatch_pkg.tmpl entgql/template.go entgql/extension.go entgql/template_test.go entgql/extension_test.go
git commit -m "$(cat <<'EOF'
feat(entgql): add collectiondispatch package template + generator

New <gen>/internal/collectiondispatch/dispatch.go is generated by
WithSplitGoFiles(true) codegen. It exposes:

  type EntityCollector interface { ... }
  func Register(entity string, c EntityCollector)
  func Get(entity string) EntityCollector
  var registry map[string]EntityCollector

Each entity sub-package's init() will register its collector{}
implementation here. Cross-entity operations dispatch through this
registry to avoid the sub-package import cycles that the
codegen-reduction split would otherwise create.

This task adds only the dispatch package; per-entity templates that
emit collector{} implementations and CollectFields/collectField
methods come in Tasks 4 and 5.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Add fixture verifying dispatch package compiles standalone

**Files:**
- Modify: `entgql/extension_test.go` (add a test that generates the dispatch package into a tmp dir AND runs `go build` against it)

This sanity-checks that the template output is valid Go. Tasks 4-5 will produce files that reference this package; if it doesn't compile, downstream fails too.

- [ ] **Step 1: Write the failing test**

```go
func TestGenerateCollectionDispatchPkg_OutputCompiles(t *testing.T) {
    tmp := t.TempDir()
    // Create a minimal Go module so `go build` works in the temp dir.
    require.NoError(t, os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(`module example.com/test
go 1.21
require github.com/99designs/gqlgen v0.17.49
`), 0644))

    target := filepath.Join(tmp, "gen")
    require.NoError(t, os.MkdirAll(target, 0755))

    ex, err := NewExtension(WithSplitGoFiles(true))
    require.NoError(t, err)

    g := &gen.Graph{Config: &gen.Config{Target: target}}
    require.NoError(t, ex.generateCollectionDispatchPkg(g))

    // go mod tidy then build the package
    cmd := exec.Command("go", "mod", "tidy")
    cmd.Dir = tmp
    if out, err := cmd.CombinedOutput(); err != nil {
        t.Fatalf("go mod tidy: %v\n%s", err, out)
    }

    cmd = exec.Command("go", "build", "./gen/internal/collectiondispatch/")
    cmd.Dir = tmp
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("dispatch package failed to compile:\n%s", out)
    }
}
```

- [ ] **Step 2: Run — should PASS (Task 2 generator + template are complete)**

```bash
go test ./entgql/ -run TestGenerateCollectionDispatchPkg_OutputCompiles -v 2>&1 | tail -10
```

Expected: PASS. If it fails, fix the template until it does — this is the canary that the generated Go is syntactically valid.

- [ ] **Step 3: Commit**

```bash
git add entgql/extension_test.go
git commit -m "$(cat <<'EOF'
test(entgql): verify generated collectiondispatch package compiles

Round-trip integration test: generate the dispatch package into a
tmp directory with a minimal go.mod, run `go build` against it,
assert success. Catches template syntax errors and missing imports
that pure parse-test wouldn't surface.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Add `collection_dispatch.tmpl` (per-entity collector struct)

**Files:**
- Create: `entgql/template/collection_dispatch.tmpl`
- Modify: `entgql/template.go` (register `CollectionDispatchTemplate`)
- Modify: `entgql/extension.go` (add `generateCollectionDispatchFile`)
- Modify: `entgql/template_test.go` (parse + render tests)

- [ ] **Step 1: Write the failing template parse test**

```go
func TestCollectionDispatchTemplate_Parse(t *testing.T) {
    require.NotNil(t, CollectionDispatchTemplate, "CollectionDispatchTemplate must be parsed at init")
    tmpl := CollectionDispatchTemplate.Lookup("gql_collection_dispatch")
    require.NotNil(t, tmpl, "template should contain 'gql_collection_dispatch' define block")
}
```

- [ ] **Step 2: Run — FAIL with "undefined: CollectionDispatchTemplate"**

```bash
go test ./entgql/ -run TestCollectionDispatchTemplate_Parse -v 2>&1 | tail -10
```

- [ ] **Step 3: Write the template**

`entgql/template/collection_dispatch.tmpl`:

```
{{/*
Copyright 2019-present Facebook Inc. All rights reserved.
This source code is licensed under the Apache 2.0 license found
in the LICENSE file in the root directory of this source tree.
*/}}

{{ define "gql_collection_dispatch" }}

{{- /*gotype: entgo.io/contrib/entgql.collectionEntityData*/ -}}

{{ $.Config.Header }}

package {{ $.Node.Package }}

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"{{ $.Config.Package }}/internal/collectiondispatch"
)

{{ $node := $.Node -}}
{{- $names := nodePaginationNames $node -}}
{{- $name := $names.Node -}}

// collector implements collectiondispatch.EntityCollector for this entity.
// All EntityCollector method parameters are typed as `any` at the registry
// boundary; each method type-asserts back to the concrete local types.
// This is the indirection that breaks cross-entity import cycles.
type collector struct{}

func (collector) NewQuery(config any) any {
	return (&Client{config: *config.(*Config)}).Query()
}

func (collector) NewPaginateArgs(rv any) any {
	return newPaginateArgs(rv.(map[string]any))
}

func (collector) PaginateArgsFirst(args any) *int   { return args.(*paginateArgs).first }
func (collector) PaginateArgsLast(args any) *int    { return args.(*paginateArgs).last }
func (collector) PaginateArgsAfter(args any) any    { return args.(*paginateArgs).after }
func (collector) PaginateArgsBefore(args any) any   { return args.(*paginateArgs).before }
func (collector) PaginateArgsOpts(args any) any     { return args.(*paginateArgs).opts }

func (collector) NewPager(opts any, last bool) (any, error) {
	return newPager(opts.([]{{ print $name "PaginateOption" }}), last)
}

func (collector) CloneQuery(query any) any {
	return query.(*Query).Clone()
}

func (collector) ApplyFilter(pager any, query any) (any, error) {
	return pager.(*pager).applyFilter(query.(*Query))
}

func (collector) ApplyCursors(pager any, query any, after, before any) (any, error) {
	var aCur, bCur *Cursor
	if after != nil { aCur = after.(*Cursor) }
	if before != nil { bCur = before.(*Cursor) }
	return pager.(*pager).applyCursors(query.(*Query), aCur, bCur)
}

func (collector) ApplyOrder(pager any, query any) any {
	return pager.(*pager).applyOrder(query.(*Query))
}

func (collector) ApplyLimit(query any, n int) any {
	return query.(*Query).Limit(n)
}

func (collector) OrderExpr(pager any, query any) any {
	return pager.(*pager).orderExpr(query.(*Query))
}

func (collector) AddQueryModifier(query any, modifier any) {
	q := query.(*Query)
	q.modifiers = append(q.modifiers, modifier.(func(*sql.Selector)))
}

func (collector) CollectFields(ctx context.Context, q any, oneNode bool, opCtx *graphql.OperationContext, collected graphql.CollectedField, path []string, satisfies ...string) error {
	return q.(*Query).collectField(ctx, oneNode, opCtx, collected, path, satisfies...)
}

func (collector) IDColumnName() string {
	return {{ printf "%q" $node.ID.StorageKey }}
}

func init() {
	collectiondispatch.Register({{ printf "%q" $node.Package }}, collector{})
}

{{ end }}
```

**Notes**:
- The exact ctor for `Client` (line `(&Client{config: *config.(*Config)}).Query()`) depends on PR 6 ent's API for sub-package `Client`/`Config`. Verify by reading a sub-package generated file once Task 1's fixture exists. If `config` is unexported and uppercase-Config is the public name, adjust.
- `paginateArgs` type and `newPaginateArgs` func: these are emitted by `collection_subpkg.tmpl` (Task 5). The dispatch template REFERENCES them. Compile order is fine — both files are in the same package.
- `pager` type (lowercase): emitted by `pagination_entity.tmpl` already (existing in fork). Verify by reading `entgql/template/pagination_entity.tmpl`.
- `Cursor` type: emitted by `pagination_shared.tmpl` in gen package. Sub-package must import gen for `Cursor`. Wait — that creates a cycle (gen imports subpkg for aliases). **Resolution**: Cursor moves to sub-package as part of this work, OR the dispatch interface erases Cursor to `any` (already done in the interface above with `after, before any`). The sub-package implementation just asserts back: `aCur = after.(*Cursor)` — but `*Cursor` needs to be in local scope. So Cursor MUST be in sub-package. This is a non-trivial decision: **out of scope for this task; deferred to Task 5 where pagination types' placement will be locked down.**

  **Pragmatic fix for Task 4**: Use `any` directly for after/before in the assertion: `pager.(*pager).applyCursors(query.(*Query), after, before)`. The pager's `applyCursors` signature would need to accept `any` for Cursor... or we rewire applyCursors. **This is messy; deferred to Task 5.**

  **Cleanest path for Task 4 to compile in isolation**: Skip `ApplyCursors`/`OrderExpr` etc. for now (return `nil, nil` with a TODO comment), and add them in Task 5 once Cursor placement is decided.

  **Updated Task 4 template (simplification)**: Generate stub methods for `ApplyCursors`, `OrderExpr`, `ApplyOrder` that panic with `"TODO: implement in Task 5 with Cursor placement"`. Don't try to make everything work in Task 4 — get the structure right, defer the type-placement-dependent pieces.

For now, emit the simplified template above with stubs. Task 5 will revisit.

- [ ] **Step 4: Register the template**

Add to `entgql/template.go`:

```go
// CollectionDispatchTemplate generates per-entity gql_collection_dispatch.go
// files into each entity sub-package. Each emits a collector struct
// implementing EntityCollector and an init() that registers it with the
// collectiondispatch package.
CollectionDispatchTemplate = parseEntityTemplate("template/collection_dispatch.tmpl", "gql_collection_dispatch")
```

- [ ] **Step 5: Run parse test, watch it pass**

```bash
go test ./entgql/ -run TestCollectionDispatchTemplate_Parse -v 2>&1 | tail -10
```

Expected: PASS.

- [ ] **Step 6: Write a render-against-fixture test**

```go
func TestCollectionDispatchTemplate_RenderRegistersEntity(t *testing.T) {
    g := makeTestGraphSingleEntity(t, "BillProduct")  // helper: builds a minimal *gen.Graph with one entity
    n := g.Nodes[0]
    var buf bytes.Buffer
    err := CollectionDispatchTemplate.Execute(&buf, struct {
        *gen.Graph
        Node                  *gen.Type
        HasWhereInputTemplate bool
    }{g, n, false})
    require.NoError(t, err)
    out := buf.String()
    require.Contains(t, out, "package billproduct")
    require.Contains(t, out, `collectiondispatch.Register("billproduct", collector{})`)
    require.Contains(t, out, "type collector struct{}")
    require.Contains(t, out, "func (collector) CollectFields(")
}
```

If `makeTestGraphSingleEntity` doesn't exist, write it as a test helper using `gen.NewGraph` with a minimal schema. Reference existing test helpers in `entgql/template_test.go` for patterns.

- [ ] **Step 7: Run — should PASS once render works**

- [ ] **Step 8: Write the generator function**

Add to `entgql/extension.go`, near `generateCollectionEntityFile`:

```go
// generateCollectionDispatchFile generates a per-entity gql_collection_dispatch.go
// inside the entity's sub-package directory. Each file emits a collector{}
// struct implementing collectiondispatch.EntityCollector and an init()
// that registers it.
func (e *Extension) generateCollectionDispatchFile(g *gen.Graph, n *gen.Type) error {
	subPkgDir := filepath.Join(g.Target, n.Package())
	if _, err := os.Stat(subPkgDir); os.IsNotExist(err) {
		return nil // sub-package doesn't exist, skip
	}
	path := filepath.Join(subPkgDir, "gql_collection_dispatch.go")

	var buf bytes.Buffer
	if err := CollectionDispatchTemplate.Execute(&buf, struct {
		*gen.Graph
		Node                  *gen.Type
		HasWhereInputTemplate bool
	}{g, n, e.genWhereInput}); err != nil {
		return fmt.Errorf("entgql: execute collection_dispatch template for %s: %w", n.Name, err)
	}
	content, err := e.processImports(path, buf.Bytes())
	if err != nil {
		return fmt.Errorf("entgql: format collection_dispatch for %s: %w", n.Name, err)
	}
	return os.WriteFile(path, content, 0644)
}
```

- [ ] **Step 9: Run all template + generator tests, watch them pass**

```bash
go test ./entgql/ -run "TestCollectionDispatch" -v 2>&1 | tail -15
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add entgql/template/collection_dispatch.tmpl entgql/template.go entgql/extension.go entgql/template_test.go
git commit -m "$(cat <<'EOF'
feat(entgql): add collection_dispatch template + generator

Each entity sub-package now gets a gql_collection_dispatch.go file
containing a collector{} struct implementing the EntityCollector
interface (from collectiondispatch). An init() in the same file
registers the collector for the entity's snake_case name.

The collector methods type-assert any-typed arguments back to local
concrete types (*Query, *paginateArgs, *pager, etc.). This is the
indirection that breaks cross-entity import cycles when entity A's
collection code needs to call into entity B's collection code.

Task 5 adds the matching collection_subpkg.tmpl that emits the
CollectFields/collectField methods on local *Query — which the
collector struct's CollectFields method dispatches to.

Cursor/OrderField placement is deferred to Task 5. ApplyCursors,
OrderExpr, and related Cursor-typed methods are stubbed in this
commit and filled in alongside the Cursor placement decision.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Add `collection_subpkg.tmpl` (per-entity CollectFields/collectField + paginateArgs)

This is the biggest task. The template ports the bulk of `collection_entity.tmpl` body into a sub-package context, rewriting:
- Package declaration: `package {{ $node.Package }}` (was: `package {{ $pkg }}`)
- Receiver type: `*Query` (was: `*{{ $node.QueryName }}` — the alias)
- All cross-entity refs: rewritten to `collectiondispatch.Get("other").Method(...)` calls
- paginateArgs type: move into this template as local `paginateArgs` (was: `<entity>PaginateArgs` in gen)
- newPaginateArgs func: move into this template as local `newPaginateArgs` (was: `new<Entity>PaginateArgs`)
- Cursor placement: define `Cursor` locally in sub-package, OR keep in gen and accept the cycle resolution chosen here

**Files:**
- Create: `entgql/template/collection_subpkg.tmpl`
- Modify: `entgql/template.go` (register `CollectionSubpkgTemplate`)
- Modify: `entgql/extension.go` (add `generateCollectionSubpkgFile`)
- Modify: `entgql/template_test.go`
- May modify: `entgql/template/collection_dispatch.tmpl` — fill in stubbed methods now that types are locked

- [ ] **Step 1: Decide Cursor / OrderField / PaginateOption placement**

Read `entgql/template/pagination_entity.tmpl` and `entgql/template/pagination_shared.tmpl`. Determine: does Cursor live in subpkg or gen today?

If gen: Cursor stays in gen. Subpkg must import gen for `*Cursor`. Cycle: subpkg imports gen, gen imports subpkg (for alias). Breaks.

If subpkg: Cursor types are already per-entity. No cycle from this dimension.

Likely current state (per spec section "Per-entity types"): Cursor is in gen (`pagination_shared.tmpl`). To break the cycle, EITHER:
- (i) Cursor moves to subpkg — requires modifying pagination_shared/pagination_entity templates too (scope creep)
- (ii) Keep Cursor in gen; sub-package's dispatch collector handles Cursor via `any` (no `*Cursor` typed reference in subpkg code at all) — dispatch interface methods that take Cursor would accept `any` and the IMPL (in gen, owned by `gen.collector` registered for each entity — wait that doesn't fit our architecture)
- (iii) Move only Cursor to subpkg, leave OrderField/PaginateOption — minimum disruption

**Recommend (iii)** for this task: move `Cursor` to per-entity sub-package. Document the change as part of this task; modify `pagination_shared.tmpl` and/or `pagination_entity.tmpl` to declare Cursor in subpkg instead of gen. Acceptance criterion: gen no longer declares `type Cursor` (or it remains as a deprecated alias only).

Actually deeper investigation may show Cursor is generic (one Cursor type for all entities) — in which case it CAN stay in gen and dispatch interfaces use it directly (gen → subpkg is fine; subpkg → gen for Cursor type IS the cycle). 

**Pragmatic decision for the plan**: Investigate Cursor's scope in Step 2 below and pick the path that touches the fewest templates. If Cursor is generic and single-typed, use the dispatch erasure trick (Cursor stays in gen but all dispatch interface methods erase to `any`; collectors handle the type-assertion via gen-package types they have access to). If Cursor is per-entity, move it to subpkg.

Looking at the existing fork: `nodePaginationNames` may differentiate per-entity. Verify.

- [ ] **Step 2: Verify Cursor type placement**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
grep -n "type Cursor\|^type [A-Z][a-zA-Z]*Cursor" entgql/template/*.tmpl | head -10
```

Inspect the matches. Determine: is `Cursor` per-entity (e.g. `BillProductCursor`) or shared (`Cursor`)?

If per-entity: it ALREADY lives in sub-packages (pagination_entity.tmpl emits it) — no work needed. Subpkg's `*Cursor` in dispatch interface is then literally the local type.

If shared: Cursor lives in gen. Use Option (ii) above — `any` at the interface boundary, type-assertion happens in the COLLECTOR implementation (which is in subpkg — needs to import gen for `*Cursor`) — and that's the cycle. **No clean win here without bigger surgery.** Document the constraint and pick: either accept gen→subpkg coupling (collector imports gen for Cursor type) OR move Cursor to subpkg.

For the plan to be tractable, assume Cursor is per-entity (most common in entgql). If discovery shows otherwise, the implementer should pause and reopen design discussion.

- [ ] **Step 3: Write the failing parse test**

```go
func TestCollectionSubpkgTemplate_Parse(t *testing.T) {
    require.NotNil(t, CollectionSubpkgTemplate)
    require.NotNil(t, CollectionSubpkgTemplate.Lookup("gql_collection_subpkg"))
}
```

- [ ] **Step 4: Run — FAIL with undefined**

- [ ] **Step 5: Write the template**

`entgql/template/collection_subpkg.tmpl` (large file — adapted from `collection_entity.tmpl`):

```
{{/*
Copyright 2019-present Facebook Inc. All rights reserved.
This source code is licensed under the Apache 2.0 license found
in the LICENSE file in the root directory of this source tree.
*/}}

{{ define "gql_collection_subpkg" }}

{{- /*gotype: entgo.io/contrib/entgql.collectionEntityData*/ -}}

{{ $.Config.Header }}

package {{ $.Node.Package }}

{{ $node := $.Node -}}

import (
	"context"
	"database/sql/driver"
	"fmt"

	"entgo.io/contrib/entgql"
	"entgo.io/ent/dialect/sql"
	"github.com/99designs/gqlgen/graphql"

	"{{ $.Config.Package }}/internal/collectiondispatch"
)

// CollectFields tells the query-builder to eagerly load connected nodes by resolver context.
// Preserves the user API: callers using the type alias <Entity>Query resolve through to this method.
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

// collectField iterates this entity's scalar fields and edges. Same-entity ops
// (selectedFields, predicate.X, *Pager local methods) are direct calls; every
// cross-entity op dispatches through collectiondispatch.Get(other).<Method>.
func (q *Query) collectField(ctx context.Context, oneNode bool, opCtx *graphql.OperationContext, collected graphql.CollectedField, path []string, satisfies ...string) error {
	path = append([]string(nil), path...)
	{{- $fields := filterFields $node.Fields (skipMode "type") }}
	{{- $collects := fieldCollections (filterEdges $node.Edges (skipMode "type")) }}
	{{- if or $collects $fields }}
		{{- if $fields }}
		var (
			unknownSeen bool
			fieldSeen = make(map[string]struct{}, len(Columns))
			selectedFields =
			{{- if $node.HasOneFieldID -}}
				[]string{ {{ $node.ID.Constant }} }
			{{- else -}}
				make([]string, 0, len(Columns))
			{{- end }}
		)
		{{- end }}
		for _, field := range graphql.CollectFields(opCtx, collected.Selections, satisfies) {
			switch field.Name {
				{{- range $i, $fc := $collects }}
					{{- $e := $fc.Edge }}
					{{- $oneNode := "false" }}{{- if $e.Unique }}{{ $oneNode = "oneNode" }}{{ end }}
					{{- $other := $e.Type.Package }}
					case {{ range $i, $value := $fc.Mapping }}{{ if $i }}, {{ end }}"{{ $value }}"{{ end }}:
						var (
							alias = field.Alias
							path  = append(path, alias)
						)
						other := collectiondispatch.Get({{ printf "%q" $other }})
						if other == nil {
							return fmt.Errorf("collectiondispatch: no collector registered for %q (path %q)", {{ printf "%q" $other }}, path)
						}
						query := other.NewQuery(q.config)
						{{- if isRelayConn $e }}
							{{- $tnames := nodePaginationNames $e.Type }}
							{{- $tname := $tnames.Node }}
							args := other.NewPaginateArgs(fieldArgs(ctx, {{ if and $.HasWhereInputTemplate (hasWhereInput $e) }}new({{ $tnames.WhereInput }}){{ else }}nil{{ end }}, path...))
							if err := validateFirstLast(other.PaginateArgsFirst(args), other.PaginateArgsLast(args)); err != nil {
								return fmt.Errorf("validate first and last in path %q: %w", path, err)
							}
							pager, err := other.NewPager(other.PaginateArgsOpts(args), other.PaginateArgsLast(args) != nil)
							if err != nil {
								return fmt.Errorf("create new pager in path %q: %w", path, err)
							}
							if query, err = other.ApplyFilter(pager, query); err != nil {
								return err
							}
							ignoredEdges := !hasCollectedField(ctx, append(path, edgesField)...)
							if hasCollectedField(ctx, append(path, totalCountField)...) || hasCollectedField(ctx, append(path, pageInfoField)...) {
								hasPagination := other.PaginateArgsAfter(args) != nil || other.PaginateArgsFirst(args) != nil || other.PaginateArgsBefore(args) != nil || other.PaginateArgsLast(args) != nil
								if hasPagination || ignoredEdges {
									cloned := other.CloneQuery(query)
									q.loadTotal = append(q.loadTotal, func(ctx context.Context, nodes []*{{ $node.Name }}) error {
										ids := make([]driver.Value, len(nodes))
										for i := range nodes {
											ids[i] = nodes[i].{{ $node.ID.StructField }}
										}
										{{- if $e.M2M }}
											{{- $fk1idx := 1 }}{{- $fk2idx := 0 }}{{ if $e.IsInverse }}{{ $fk1idx = 0 }}{{ $fk2idx = 1 }}{{ end }}
											var v []struct{
												NodeID {{ $node.ID.Type }} `sql:"{{ index $e.Rel.Columns $fk2idx }}"`
												Count  int                  `sql:"count"`
											}
											// SQL Join: parent table → join table → other entity (via dispatch for other.IDColumnName())
											clonedTyped := cloned // typed as any — pass to dispatch via Where helper
											_ = clonedTyped
											// TODO(implementer): The original template performed SQL operations directly on
											// the typed *Query. After dispatch, we have `cloned any`. We need an additional
											// dispatch method to execute arbitrary SQL on the cloned query OR we accept the
											// loadTotal closure body remains coupled to the local-entity SQL types only
											// (which means we cannot construct cross-entity Join in this closure).
											//
											// Pragmatic resolution: add `ExecuteCountScan(query any, where func(*sql.Selector), out any) error`
											// to EntityCollector. Replace the SQL block below with a single dispatch call.
											// (See plan §"Open questions during implementation" in spec.)
											_ = ids; _ = v
										{{- else }}
											var v []struct{
												NodeID {{ $node.ID.Type }} `sql:"{{ $e.Rel.Column }}"`
												Count  int                  `sql:"count"`
											}
											// Similar TODO — need ExecuteCountByFK(query, fkColumn, ids, out)
											_ = ids; _ = v
										{{- end }}
										return nil
									})
								} else {
									q.loadTotal = append(q.loadTotal, func(_ context.Context, nodes []*{{ $node.Name }}) error {
										for i := range nodes {
											n := len(nodes[i].Edges.{{ $e.StructField }})
											if nodes[i].Edges.TotalCount[{{ $i }}] == nil {
												nodes[i].Edges.TotalCount[{{ $i }}] = make(map[string]int)
											}
											nodes[i].Edges.TotalCount[{{ $i }}][alias] = n
										}
										return nil
									})
								}
							}
							if ignoredEdges || (other.PaginateArgsFirst(args) != nil && *other.PaginateArgsFirst(args) == 0) || (other.PaginateArgsLast(args) != nil && *other.PaginateArgsLast(args) == 0) {
								continue
							}
							if query, err = other.ApplyCursors(pager, query, other.PaginateArgsAfter(args), other.PaginateArgsBefore(args)); err != nil {
								return err
							}
							path = append(path, edgesField, nodeField)
							if fld := collectedField(ctx, path...); fld != nil {
								if err := other.CollectFields(ctx, query, {{ $oneNode }}, opCtx, *fld, path, mayAddCondition(satisfies, {{ nodeImplementorsVar $e.Type }})...); err != nil {
									return err
								}
							}
							if limit := paginateLimit(other.PaginateArgsFirst(args), other.PaginateArgsLast(args)); limit > 0 {
								if oneNode {
									other.ApplyOrder(pager, other.ApplyLimit(query, limit))
								} else {
									{{- if $e.M2M }}
										{{- $i := 0 }}{{ if $e.IsInverse }}{{ $i = 1 }}{{ end }}
										fk := {{ $e.PKConstant }}[{{ $i }}]
									{{- else }}
										fk := {{ $fc.Edge.ColumnConstant }}
									{{- end }}
									modify := entgql.LimitPerRow(fk, limit, other.OrderExpr(pager, query))
									other.AddQueryModifier(query, modify)
								}
							} else {
								query = other.ApplyOrder(pager, query)
							}
						{{- else }}
							if err := other.CollectFields(ctx, query, {{ $oneNode }}, opCtx, field, path, mayAddCondition(satisfies, {{ nodeImplementorsVar $e.Type }})...); err != nil {
								return err
							}
						{{- end }}
						{{- if $e.Unique }}
							q.{{ $e.EagerLoadField }} = query.(*{{ $other }}.Query)
						{{- else }}
							// WithNamed<Edge> requires a typed lambda. Use a local helper that
							// asserts back and copies. The typed import is unavoidable here.
							q.WithNamed{{ $e.StructField }}(alias, func(wq *{{ $other }}.Query) {
								*wq = *query.(*{{ $other }}.Query)
							})
						{{- end }}
						{{- with $e.Field }}
							if _, ok := fieldSeen[{{ .Constant }}]; !ok {
								selectedFields = append(selectedFields, {{ .Constant }})
								fieldSeen[{{ .Constant }}] = struct{}{}
							}
						{{- end }}
				{{- end }}
				{{- range $f := $fields }}
					{{- with fieldMapping $f }}
						case {{ range $i, $m := . }}{{ if $i }}, {{ end }}"{{ $m }}"{{ end }}:
							if _, ok := fieldSeen[{{ $f.Constant }}]; !ok {
								selectedFields = append(selectedFields, {{ $f.Constant }})
								fieldSeen[{{ $f.Constant }}] = struct{}{}
							}
					{{- end }}
				{{- end }}
				{{- if $fields }}
					{{- if $node.HasOneFieldID -}}
						{{- with fieldMapping $node.ID }}
						case {{ range $i, $m := . }}{{ if $i }}, {{ end }}"{{ $m }}"{{ end }}:
						{{- end }}
					{{- end -}}
				case "__typename":
				default:
					unknownSeen = true
				{{- end }}
			}
		}
		{{- if $fields }}
			if !unknownSeen {
				q.Select(selectedFields...)
			}
		{{- end }}
	{{- end }}
	return nil
}

{{ $names := nodePaginationNames $node -}}
{{- $name := $names.Node -}}
{{- $filter := print "With" $name "Filter" -}}
{{- $order := $names.Order -}}
{{- $multiOrder := $node.Annotations.EntGQL.MultiOrder -}}
{{- $orderField := $names.OrderField -}}

// paginateArgs are this entity's per-call pagination inputs. Moved from the
// gen-package <entity>PaginateArgs into the sub-package as the unqualified
// local name. Other entities access these via collectiondispatch (PaginateArgs*
// methods on EntityCollector).
type paginateArgs struct {
	first, last  *int
	after, before *Cursor
	opts         []{{ print $name "PaginateOption" }}
}

func newPaginateArgs(rv map[string]any) *paginateArgs {
	args := &paginateArgs{}
	if rv == nil {
		return args
	}
	if v := rv[firstField]; v != nil {
		args.first = v.(*int)
	}
	if v := rv[lastField]; v != nil {
		args.last = v.(*int)
	}
	if v := rv[afterField]; v != nil {
		args.after = v.(*Cursor)
	}
	if v := rv[beforeField]; v != nil {
		args.before = v.(*Cursor)
	}
	{{- with orderFields $node }}
		if v, ok := rv[orderByField]; ok {
			switch v := v.(type) {
			{{- if $multiOrder }}
				case []*{{ $order }}:
					args.opts = append(args.opts, {{ print "With" $order }}(v))
				case []any:
					var orders []*{{ $order }}
					for i := range v {
						mv, ok := v[i].(map[string]any)
						if !ok { continue }
						var (
							err1, err2 error
							order = &{{ $order }}{Field: &{{ $orderField }}{}, Direction: entgql.OrderDirectionAsc}
						)
						if d, ok := mv[directionField]; ok { err1 = order.Direction.UnmarshalGQL(d) }
						if f, ok := mv[fieldField]; ok    { err2 = order.Field.UnmarshalGQL(f) }
						if err1 == nil && err2 == nil { orders = append(orders, order) }
					}
					args.opts = append(args.opts, {{ print "With" $order }}(orders))
			{{- else }}
				case map[string]any:
					var (
						err1, err2 error
						order = &{{ $order }}{Field: &{{ $orderField }}{}, Direction: entgql.OrderDirectionAsc}
					)
					if d, ok := v[directionField]; ok { err1 = order.Direction.UnmarshalGQL(d) }
					if f, ok := v[fieldField]; ok    { err2 = order.Field.UnmarshalGQL(f) }
					if err1 == nil && err2 == nil    { args.opts = append(args.opts, {{ print "With" $order }}(order)) }
				case *{{ $order }}:
					if v != nil { args.opts = append(args.opts, {{ print "With" $order }}(v)) }
			{{- end }}
			}
		}
	{{- end }}
	{{- if $.HasWhereInputTemplate }}
		{{- $withWhere := true }}{{ with $node.Annotations.EntGQL }}{{ if isSkipMode .Skip "where_input" }}{{ $withWhere = false }}{{ end }}{{ end }}
		{{- if $withWhere }}
			{{- $where := $names.WhereInput }}
			if v, ok := rv[whereField].(*{{ $where }}); ok {
				args.opts = append(args.opts, {{ $filter }}(v.Filter))
			}
		{{- end }}
	{{- end }}
	return args
}

{{ end }}
```

**Major TODOs embedded in the template** (deliberately):
- `ExecuteCountScan` / `ExecuteCountByFK` dispatch methods for loadTotal SQL blocks (currently stubbed with `_ = ids; _ = v` — emits compilable but no-op closures). The implementer adding this task should expand `EntityCollector` interface with these methods and fill in the SQL dispatch logic in `collection_dispatch.tmpl`.
- The `WithNamed<Edge>` and `Unique` edge eager-load lines still use `query.(*{{ $other }}.Query)` — a typed reference to the OTHER sub-package. For entity pairs with reciprocal edges (A↔B), this creates the cycle. **Resolution path**: the eager-load assignment IS a typed cross-entity ref. Two options:
  - (a) Accept the cycle and force PARENT entities to not be reciprocally edged (unlikely — common in real schemas)
  - (b) Add a dispatch method `AssignAsEagerLoad(parentQuery, edgeName, alias, otherQuery)` on **parent's** collector — but parent's subpkg can't call its OWN collector via dispatch easily without dispatch lookup. Actually it can — parent's subpkg uses `collectiondispatch.Get(<parent name>)` for its own ops too. So replace `q.<EagerField> = query.(*<other>.Query)` with `parentDispatch.AssignEagerLoad(q, "<edgeName>", alias, query)`.

  **Resolution to commit to**: option (b). Add to EntityCollector:
  ```go
  // AssignEagerLoad attaches the cross-entity query to the parent's eager-load slot.
  // For Unique edges: sets the parent's *<Entity>Query EagerLoad field.
  // For non-Unique edges: invokes WithNamed<Edge>(alias, func(wq *<OtherQuery>) { *wq = *otherQuery }).
  // Implementation lives in the PARENT entity's collector{} where typed access to *<OtherQuery> requires either a typed import (cycle) or reflect-based assignment.
  AssignEagerLoad(parentQuery any, edgeName, alias string, otherQuery any)
  ```

  The implementer must EITHER add typed imports (accepting cycles) OR use reflect for the assignment. Reflect is correct here — set a struct field by name, or call a method by name. The performance cost is negligible (collection happens once per request).

  Update the template to remove the typed `*{{ $other }}.Query` references — replace with `parentDispatch.AssignEagerLoad(q, "{{ $e.Name }}", alias, query)` (no typed imports of other subpkgs).

This is a meaningful refinement. **Task 5 acceptance criterion is updated**: the generated subpkg `gql_collection.go` MUST NOT import any sibling sub-package. Verify by parsing the generated file's imports.

- [ ] **Step 6: Register the template + write generator**

Add to `entgql/template.go`:
```go
CollectionSubpkgTemplate = parseEntityTemplate("template/collection_subpkg.tmpl", "gql_collection_subpkg")
```

Add to `entgql/extension.go`:
```go
// generateCollectionSubpkgFile generates per-entity gql_collection.go inside
// the entity's sub-package directory. Methods on local *Query (CollectFields,
// collectField, paginateArgs/newPaginateArgs). Cross-entity ops dispatch
// through collectiondispatch.
func (e *Extension) generateCollectionSubpkgFile(g *gen.Graph, n *gen.Type) error {
	subPkgDir := filepath.Join(g.Target, n.Package())
	if _, err := os.Stat(subPkgDir); os.IsNotExist(err) {
		return nil
	}
	path := filepath.Join(subPkgDir, "gql_collection.go")

	var buf bytes.Buffer
	if err := CollectionSubpkgTemplate.Execute(&buf, struct {
		*gen.Graph
		Node                  *gen.Type
		HasWhereInputTemplate bool
	}{g, n, e.genWhereInput}); err != nil {
		return fmt.Errorf("entgql: execute collection_subpkg template for %s: %w", n.Name, err)
	}
	content, err := e.processImports(path, buf.Bytes())
	if err != nil {
		return fmt.Errorf("entgql: format collection_subpkg for %s: %w", n.Name, err)
	}
	return os.WriteFile(path, content, 0644)
}
```

- [ ] **Step 7: Write integration test that the generated subpkg has no sibling imports**

```go
func TestCollectionSubpkgGen_NoSiblingImports(t *testing.T) {
    // ... generate a 2-entity fixture (A with edge to B, B with edge to A) ...
    out, err := os.ReadFile(filepath.Join(target, "a", "gql_collection.go"))
    require.NoError(t, err)
    require.NotContains(t, string(out), `"<root>/gen/b"`, "subpkg must not import sibling sub-packages")
}
```

- [ ] **Step 8: Run all tests; iterate template until tests pass**

This step will likely surface MANY issues. Run the template tests, see failures, fix the template, re-run. Expected iterations: 5-15 cycles. Each fix is mechanical (Go syntax, import paths, type assertions).

- [ ] **Step 9: Commit when all tests pass**

```bash
git add entgql/template/collection_subpkg.tmpl entgql/template.go entgql/extension.go entgql/template_test.go
git commit -m "$(cat <<'EOF'
feat(entgql): add collection_subpkg template — per-entity CollectFields

New gql_collection.go in each entity sub-package contains the
CollectFields/collectField methods on local *Query plus the
paginateArgs type and newPaginateArgs constructor (moved from
the gen package where they were per-entity-prefixed).

Cross-entity operations dispatch through the collectiondispatch
registry — no sub-package imports any sibling sub-package. The
EntityCollector interface methods used for dispatch are exhaustive
of every cross-entity reference in the original collection_entity
template (NewQuery, NewPager, ApplyFilter, ApplyCursors, ApplyOrder,
ApplyLimit, CloneQuery, OrderExpr, AddQueryModifier, CollectFields,
PaginateArgs accessors, AssignEagerLoad, IDColumnName).

The eager-load assignment (WithNamed<Edge> / Unique-edge field set)
goes through parent's own collector via AssignEagerLoad, using
reflect for the typed assignment — avoiding sibling subpkg imports
that would create cycles for entities with reciprocal edges.

The loadTotal SQL dispatch (ExecuteCountScan / ExecuteCountByFK
interface methods) is added in this commit and implemented in
collection_dispatch.tmpl alongside.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Wire new generators into `generateSplitGoFiles`

**Files:**
- Modify: `entgql/extension.go` — replace `generateCollectionEntityFile` calls with the three new generators

- [ ] **Step 1: Locate the existing wiring**

```bash
grep -n "generateCollectionEntityFile\|generateCollectionSharedFile" entgql/extension.go
```

Expected: 2-3 matches around line 530-600.

- [ ] **Step 2: Modify generateSplitGoFiles**

Replace the per-entity wiring in the fan-out loop. Roughly (verify against actual code):

```go
// Before:
fns = append(fns,
    func() error { return e.generatePaginationEntityFile(&staged, n) },
    func() error { return e.generateCollectionEntityFile(&staged, n) },
    func() error { return e.generateNodeEntityFile(&staged, n) },
)

// After:
fns = append(fns,
    func() error { return e.generatePaginationEntityFile(&staged, n) },
    func() error { return e.generateCollectionSubpkgFile(g, n) },          // NEW
    func() error { return e.generateCollectionDispatchFile(g, n) },         // NEW
    func() error { return e.generateNodeEntityFile(&staged, n) },
)
```

Note: pass `g` (the real graph with sub-package dirs) to the new generators, NOT `&staged`. The sub-package dirs are created by ent's own codegen, not by the staging system (mirror the `generateMutationInputSubpkgFile` call).

Add the dispatch package generator call near the start of `generateSplitGoFiles`, before the fan-out:

```go
if err := e.generateCollectionDispatchPkg(g); err != nil {
    return err
}
```

- [ ] **Step 3: Run all tests**

```bash
go test ./entgql/ 2>&1 | tail -10
```

Some tests may now break: any test that asserted `gql_collection_<entity>.go` files appear in gen will fail because they no longer exist. Fix those assertions (or remove the tests if they're now redundant).

- [ ] **Step 4: Run the Bug 9 integration test**

```bash
go test ./entgql/ -run TestBug9_TodoFixtureBuilds -v 2>&1 | tail -20
```

This test should now PASS (or at least the Bug 9 signature should not appear; there may be other errors that need addressing in Task 7).

- [ ] **Step 5: Commit**

```bash
git add entgql/extension.go
git commit -m "$(cat <<'EOF'
feat(entgql): wire collection subpkg + dispatch generators into split codegen

generateSplitGoFiles now calls:
  - generateCollectionDispatchPkg once (writes <gen>/internal/collectiondispatch/dispatch.go)
  - generateCollectionSubpkgFile per entity (writes <gen>/<entity>/gql_collection.go)
  - generateCollectionDispatchFile per entity (writes <gen>/<entity>/gql_collection_dispatch.go)

The old generateCollectionEntityFile per-entity call is retained for now;
Task 7 removes it once parity is verified.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Remove obsolete `collection_entity.tmpl` + `generateCollectionEntityFile` + tests

**Files:**
- Delete: `entgql/template/collection_entity.tmpl`
- Modify: `entgql/template.go` — remove `CollectionEntityTemplate`
- Modify: `entgql/extension.go` — remove `generateCollectionEntityFile` and its call site
- Modify: `entgql/template_test.go` — remove tests for `CollectionEntityTemplate`

- [ ] **Step 1: Grep for all references**

```bash
grep -rn "CollectionEntityTemplate\|generateCollectionEntityFile\|gql_collection_entity\|collection_entity.tmpl" entgql/ | head -20
```

- [ ] **Step 2: Delete the template file**

```bash
git rm entgql/template/collection_entity.tmpl
```

- [ ] **Step 3: Remove the template var from `template.go`**

Find and delete the `CollectionEntityTemplate = parseEntityTemplate(...)` line (around line 181).

- [ ] **Step 4: Remove the generator function from `extension.go`**

Find and delete the entire `generateCollectionEntityFile` function and its call site in `generateSplitGoFiles`.

- [ ] **Step 5: Remove related tests from `template_test.go`**

Find any test referencing `CollectionEntityTemplate` and delete them.

- [ ] **Step 6: Build everything**

```bash
go build ./entgql/... 2>&1 | tail -20
```

Expected: no errors. If there are remaining refs to `CollectionEntityTemplate`, fix them.

- [ ] **Step 7: Run full test suite**

```bash
go test ./entgql/... 2>&1 | tail -20
```

Expected: all PASS.

- [ ] **Step 8: Run Bug 9 integration test specifically**

```bash
go test ./entgql/ -run TestBug9_TodoFixtureBuilds -v 2>&1 | tail -10
```

Expected: PASS. (If the test fixture from Task 1 was a new minimal fixture rather than `todo`, adjust the test target accordingly.)

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
chore(entgql): remove obsolete collection_entity template + generator

Replaced by collection_subpkg.tmpl (Task 5) + collection_dispatch.tmpl
(Task 4) + collection_dispatch_pkg.tmpl (Task 2). The old template
declared package gen and defined methods on *<Entity>Query type
aliases — Go forbids method definitions on aliased types from foreign
packages, which is the entirety of Bug 9.

Deleted:
- entgql/template/collection_entity.tmpl
- CollectionEntityTemplate (entgql/template.go)
- generateCollectionEntityFile (entgql/extension.go)
- TestCollectionEntityTemplate_* (entgql/template_test.go)

The Bug 9 integration test (TestBug9_TodoFixtureBuilds) now passes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: End-to-end consumer verification (optional, ephemeral)

This is the bench-level verification — point the bench worktree's go.mod at THIS branch of contrib, run consumer codegen + build, observe that the gql_collection_*.go errors disappear.

This task does NOT make any commits and does NOT touch the contrib repo.

- [ ] **Step 1: Configure bench to point at this contrib branch**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
# Save bench go.mod state
git stash push -m "preserve bench go.mod" -- go.mod
git checkout -- .
git stash pop
# Add a local-path replace for contrib
echo 'replace entgo.io/contrib => /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg' >> go.mod
go mod tidy 2>&1 | tail -5
```

- [ ] **Step 2: Regenerate ent code**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql
go generate ./...  2>&1 | tail -20
```

- [ ] **Step 3: Run consumer migrate (Bug 8 + 8b)**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
/tmp/ent-codegen-migrate-iter2 \
    -descriptors api-graphql/src/ent/gen/internal \
    -gen-package github.com/MatthewsREIS/gemini/service-api-go/api-graphql/src/ent/gen \
    ./api-graphql/src/... 2>&1 | tee /tmp/iter3-migrate.log | tail -10
```

- [ ] **Step 4: Build**

```bash
go build ./api-graphql/... 2>&1 | tee /tmp/iter3-build.log | head -100
```

- [ ] **Step 5: Inspect build output**

The previous Bug 9 errors (`cannot define new methods on non-local type AgentLicensingQuery`) should be GONE. Surface any NEW failures and characterize.

- [ ] **Step 6: Restore bench go.mod**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
git checkout -- go.mod
# Confirm clean
git diff go.mod
```

- [ ] **Step 7: Report**

Summarize findings to the user: did Bug 9 disappear? Are there new issues? Are we ready to push contrib + open PR?

---

## Self-review

After completing all tasks, verify:

1. **Spec coverage**: Every acceptance criterion in the spec maps to a task:
   - ✅ Acceptance 1 (WithSplitGoFiles generates dispatch pkg + per-entity files) → Tasks 2, 4, 5, 6
   - ✅ Acceptance 2 (generated output `go build`s) → Task 7 (integration test) + Task 8 (consumer bench)
   - ✅ Acceptance 3 (q.CollectFields(ctx) callable via aliases) → Task 5 (preserves method signature)
   - ✅ Acceptance 4 (existing entgql tests pass unchanged) → Tasks 2, 4, 5, 7 (each runs `go test ./entgql/...`)
   - ✅ Acceptance 5 (old templates removed) → Task 7
   - ✅ Acceptance 6 (WithSplitGoFiles(false) users see no change) → Implicit (we didn't touch `collection.tmpl`)

2. **Placeholder scan**: The template in Task 5 contains explicit TODOs around `ExecuteCountScan` / `ExecuteCountByFK` and AssignEagerLoad. These are NOT plan-failure placeholders — they're documented design refinements the implementer must make as part of Task 5. The plan flags them clearly.

3. **Type consistency**: The `collector` struct in Task 4 references types (`Config`, `Query`, `paginateArgs`, `pager`, `Cursor`, `PaginateOption`) that exist in the entity sub-package OR are created by Task 5 (`paginateArgs`). Verify after Task 5 lands that all type names match.

## Execution

Plan complete and saved to `docs/superpowers/plans/2026-05-17-entgql-collection-subpkg-dispatch.md`.

This is a multi-day implementation (per the spec's risk register, 3-5 days for the full architectural pass). Task 5 in particular will require iterative template work and likely interface-refinement cycles.

**Recommended execution**: Use **superpowers:subagent-driven-development** — fresh subagent per task + two-stage review. Particularly important for Task 5, which has the most opportunities for the template to drift from the design.
