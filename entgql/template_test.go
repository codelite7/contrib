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

package entgql

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/require"
)

var annotationName = Annotation{}.Name()

func TestFilterNodes(t *testing.T) {
	nodes, err := filterNodes([]*gen.Type{
		{
			Name: "Type1",
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{},
			},
		},
		{
			Name:   "Type2",
			Config: &gen.Config{},
		},
		{
			Name: "SkippedType",
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{"Skip": SkipAll},
			},
		},
	}, SkipType)
	require.NoError(t, err)
	require.Equal(t, []*gen.Type{
		{
			Name: "Type1",
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{},
			},
		},
		{
			Name:   "Type2",
			Config: &gen.Config{},
		},
	}, nodes)
}

func TestFilterEdges(t *testing.T) {
	edges, err := filterEdges([]*gen.Edge{
		{
			Name: "Edge1",
			Type: &gen.Type{},
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{},
			},
		},
		{
			Name: "Edge2",
			Type: &gen.Type{},
		},
		{
			Name: "SkippedEdge",
			Type: &gen.Type{},
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{"Skip": SkipAll},
			},
		},
		{
			Name: "SkippedEdgeType",
			Type: &gen.Type{
				Annotations: map[string]interface{}{
					annotationName: map[string]interface{}{"Skip": SkipAll},
				},
			},
		},
	}, SkipType)
	require.NoError(t, err)
	require.Equal(t, []*gen.Edge{
		{
			Name: "Edge1",
			Type: &gen.Type{},
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{},
			},
		},
		{
			Name: "Edge2",
			Type: &gen.Type{},
		},
	}, edges)
}

func TestFieldCollections(t *testing.T) {
	edges := []*gen.Edge{
		{
			Name: "Edge1",
			Type: &gen.Type{
				Name: "Todo",
			},
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{},
			},
		},
		{
			Name: "Edge2",
			Type: &gen.Type{
				Name: "Todo",
			},
		},
		{
			Name: "two_words",
			Type: &gen.Type{
				Name: "Todo",
			},
		},
		{
			Name: "Unbind",
			Type: &gen.Type{
				Name: "Todo",
			},
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{
					"Unbind": true,
				},
			},
		},
		{
			Name: "EdgeMapping",
			Type: &gen.Type{
				Name: "Todo",
			},
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{
					"Unbind":  true,
					"Mapping": []string{"field1", "field2"},
				},
			},
		},
	}
	collect, err := fieldCollections(edges)
	require.NoError(t, err)
	require.Equal(t, []*fieldCollection{
		{
			Edge:    edges[0],
			Mapping: []string{"edge1"},
		},
		{
			Edge:    edges[1],
			Mapping: []string{"edge2"},
		},
		{
			Edge:    edges[2],
			Mapping: []string{"twoWords"},
		},
		{
			Edge:    edges[4],
			Mapping: []string{"field1", "field2"},
		},
	}, collect)

	_, err = fieldCollections([]*gen.Edge{
		{
			Name: "EdgeInvalid",
			Type: &gen.Type{
				Name: "Todo",
			},
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{
					"Unbind":  false,
					"Mapping": []string{"field1", "field2"},
				},
			},
		},
	})
	require.Errorf(t, err, "bind and mapping annotations are mutually exclusive")
}

func TestPaginationSharedTemplateParsed(t *testing.T) {
	// Verify the PaginationSharedTemplate was parsed successfully during init().
	require.NotNil(t, PaginationSharedTemplate, "PaginationSharedTemplate should be parsed during init()")
	// Verify the template has the expected name.
	require.Equal(t, "gql_pagination_shared", PaginationSharedTemplate.Name())
	// Verify it has the expected define block.
	tmpl := PaginationSharedTemplate.Lookup("gql_pagination_shared")
	require.NotNil(t, tmpl, "template should contain 'gql_pagination_shared' define block")
}

func TestPaginationSharedTemplateContent(t *testing.T) {
	// Verify the template source contains the expected shared code elements.
	tmpl := PaginationSharedTemplate.Lookup("gql_pagination_shared")
	require.NotNil(t, tmpl)
	src := tmpl.Tree.Root.String()

	// Verify shared type aliases are present.
	require.Contains(t, src, "Cursor = entgql.Cursor")
	require.Contains(t, src, "PageInfo = entgql.PageInfo")
	require.Contains(t, src, "OrderDirection = entgql.OrderDirection")
	require.Contains(t, src, "NullsDirection = entgql.NullsDirection")

	// Verify shared functions are present. After the gqlpage rewrite, orderFunc
	// and paginateLimit are gone entirely (dead weight -- paginateLimit's logic
	// now lives once in gqlpage.PaginateLimit, called per-entity); validateFirstLast,
	// collectedField and hasCollectedField remain as thin forwarders (kept, not
	// deleted, because entsearch-generated files in the same root package call
	// them unqualified).
	require.Contains(t, src, "func validateFirstLast")
	require.Contains(t, src, "func collectedField")
	require.Contains(t, src, "func hasCollectedField")
	require.NotContains(t, src, "func orderFunc")
	require.NotContains(t, src, "func paginateLimit")

	// Verify the forwarders delegate to gqlpage rather than reimplementing the logic.
	require.Contains(t, src, "gqlpage.ValidateFirstLast(first, last)")
	require.Contains(t, src, "gqlpage.CollectedField(ctx, path...)")
	require.Contains(t, src, "gqlpage.HasCollectedField(ctx, path...)")
	// errInvalidPagination/errcode are gone with the inlined validateFirstLast body.
	require.NotContains(t, src, "errInvalidPagination")

	// Verify shared constants are present. Only edgesField/nodeField remain (the
	// consumer, search_pagination.go, never references pageInfoField/totalCountField).
	// The field constants are generated via a range over list "edges" "node", so
	// check the list items.
	require.Contains(t, src, `"edges"`)
	require.Contains(t, src, `"node"`)
	// Verify the Field suffix pattern is in the template.
	require.Contains(t, src, `Field = "`)

	// Verify NO per-entity code is present.
	require.NotContains(t, src, "Connection struct")
	require.NotContains(t, src, "Edge struct")
	require.NotContains(t, src, "Pager")
	require.NotContains(t, src, "Paginate")
	require.NotContains(t, src, "applyOrder")
	require.NotContains(t, src, "applyCursors")
	require.NotContains(t, src, "applyFilter")
	require.NotContains(t, src, "ToEdge")
}

func TestPaginationSharedTemplateExecution(t *testing.T) {
	// Execute the template against the real todo schema graph and verify
	// the generated output contains expected shared code.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	tmpl := PaginationSharedTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_pagination_shared", struct {
		*gen.Graph
	}{graph})
	require.NoError(t, err)

	output := buf.String()

	// Verify the generated output contains the package declaration.
	require.Contains(t, output, "package ent")

	// Verify shared type aliases are generated.
	require.Contains(t, output, "Cursor = entgql.Cursor[")
	require.Contains(t, output, "PageInfo = entgql.PageInfo[")
	require.Contains(t, output, "OrderDirection = entgql.OrderDirection")
	require.Contains(t, output, "NullsDirection = entgql.NullsDirection")

	// Verify shared functions are generated. orderFunc/paginateLimit are gone
	// (see TestPaginationSharedTemplateContent); validateFirstLast, collectedField
	// and hasCollectedField remain as forwarders to gqlpage.
	require.Contains(t, output, "func validateFirstLast(")
	require.Contains(t, output, "func collectedField(")
	require.Contains(t, output, "func hasCollectedField(")
	require.NotContains(t, output, "func orderFunc(")
	require.NotContains(t, output, "func paginateLimit(")
	require.Contains(t, output, "gqlpage.ValidateFirstLast(first, last)")
	require.Contains(t, output, "gqlpage.CollectedField(ctx, path...)")
	require.Contains(t, output, "gqlpage.HasCollectedField(ctx, path...)")

	// Verify shared constants are generated.
	require.NotContains(t, output, `errInvalidPagination`)
	require.Contains(t, output, `edgesField = "edges"`)
	require.Contains(t, output, `nodeField = "node"`)
	require.NotContains(t, output, `pageInfoField`)
	require.NotContains(t, output, `totalCountField`)

	// Verify NO per-entity code is present (e.g., no TodoConnection, TodoEdge, todoPager).
	require.False(t, strings.Contains(output, "TodoConnection"),
		"shared template should not generate per-entity TodoConnection type")
	require.False(t, strings.Contains(output, "TodoEdge"),
		"shared template should not generate per-entity TodoEdge type")
	require.False(t, strings.Contains(output, "todoPager"),
		"shared template should not generate per-entity todoPager type")
	require.False(t, strings.Contains(output, "func (_m *TodoQuery) Paginate"),
		"shared template should not generate per-entity Paginate method")
}

func TestCollectionSharedTemplateParsed(t *testing.T) {
	// Verify the CollectionSharedTemplate was parsed successfully during init().
	require.NotNil(t, CollectionSharedTemplate, "CollectionSharedTemplate should be parsed during init()")
	// Verify the template has the expected name.
	require.Equal(t, "gql_collection_shared", CollectionSharedTemplate.Name())
	// Verify it has the expected define block.
	tmpl := CollectionSharedTemplate.Lookup("gql_collection_shared")
	require.NotNil(t, tmpl, "template should contain 'gql_collection_shared' define block")
}

func TestCollectionSharedTemplateContent(t *testing.T) {
	// Verify the template contains the expected shared code elements
	// by checking the template tree for key identifiers.
	tmpl := CollectionSharedTemplate.Lookup("gql_collection_shared")
	require.NotNil(t, tmpl)

	// The template source should reference our shared constants and functions.
	// We verify this by checking the template's parse tree root text contains expected strings.
	src := tmpl.Tree.Root.String()

	// The constants are generated via a range over a list, so we verify
	// the list items are present in the template source rather than the
	// expanded constant names (which only appear after template execution).
	require.Contains(t, src, `"after"`)
	require.Contains(t, src, `"first"`)
	require.Contains(t, src, `"before"`)
	require.Contains(t, src, `"last"`)
	require.Contains(t, src, `"orderBy"`)
	require.Contains(t, src, `"direction"`)
	require.Contains(t, src, `"where"`)
	// Verify the Field suffix pattern is in the template.
	require.Contains(t, src, `Field = "`)

	// Verify shared functions are present.
	require.Contains(t, src, "func fieldArgs")
	require.Contains(t, src, "func unmarshalArgs")
	require.Contains(t, src, "func normalizeInputEnums")
	require.Contains(t, src, "func mayAddCondition")

	// Verify NO per-entity code is present.
	require.NotContains(t, src, "CollectFields tells the query-builder")
	require.NotContains(t, src, "collectField")
	require.NotContains(t, src, "PaginateArgs")
	require.NotContains(t, src, "newPaginateArg")
}

func TestCollectionSharedTemplateExecution(t *testing.T) {
	// Execute the template against the real todo schema graph and verify
	// the generated output contains expected shared code.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	tmpl := CollectionSharedTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_collection_shared", struct {
		*gen.Graph
	}{graph})
	require.NoError(t, err)

	output := buf.String()

	// Verify the generated output contains the package declaration.
	require.Contains(t, output, "package ent")

	// Verify shared constants are generated.
	require.Contains(t, output, `afterField = "after"`)
	require.Contains(t, output, `firstField = "first"`)
	require.Contains(t, output, `beforeField = "before"`)
	require.Contains(t, output, `lastField = "last"`)
	require.Contains(t, output, `orderByField = "orderBy"`)
	require.Contains(t, output, `directionField = "direction"`)
	require.Contains(t, output, `fieldField = "field"`)
	require.Contains(t, output, `whereField = "where"`)

	// Verify shared functions are generated.
	require.Contains(t, output, "func fieldArgs(")
	require.Contains(t, output, "func parsedFieldArgs(")
	require.Contains(t, output, "func cloneArgsMap(")
	require.Contains(t, output, "func unmarshalArgs(")
	require.Contains(t, output, "func normalizeInputEnums(")
	require.Contains(t, output, "func mayAddCondition(")
	require.Contains(t, output, "parsedFieldArgs(ctx, path...)")
	require.Contains(t, output, "child, err := fc.Child(ctx, field)")
	require.Contains(t, output, "return cloneArgsMap(fc.Args)")

	// Verify where-input fallback unmarshaling is generated.
	require.Contains(t, output, "json.Marshal(v)")
	require.Contains(t, output, "json.Unmarshal(rawJSON, whereInput)")
	require.Contains(t, output, "normalizeInputEnums(reflect.ValueOf(whereInput))")

	// Verify NO per-entity code is present.
	require.False(t, strings.Contains(output, "CollectFields tells the query-builder"),
		"shared template should not generate per-entity CollectFields method")
	require.False(t, strings.Contains(output, "collectField"),
		"shared template should not generate per-entity collectField method")
	require.False(t, strings.Contains(output, "PaginateArgs"),
		"shared template should not generate per-entity PaginateArgs type")
	require.False(t, strings.Contains(output, "newPaginateArg"),
		"shared template should not generate per-entity newPaginateArgs function")
}

func TestPaginationEntityTemplateParsed(t *testing.T) {
	// Verify the PaginationEntityTemplate was parsed successfully during init().
	require.NotNil(t, PaginationEntityTemplate, "PaginationEntityTemplate should be parsed during init()")
	// Verify the template has the expected name.
	require.Equal(t, "gql_pagination_entity", PaginationEntityTemplate.Name())
	// Verify it has the expected define block.
	tmpl := PaginationEntityTemplate.Lookup("gql_pagination_entity")
	require.NotNil(t, tmpl, "template should contain 'gql_pagination_entity' define block")
}

func TestPaginationEntityTemplateContent(t *testing.T) {
	// Verify the template source contains the expected thin re-export shim elements.
	// After lever B-2, pagination_entity.tmpl emits type aliases and var forwarders only;
	// the full pagination body moved to pagination_subpkg.tmpl.
	tmpl := PaginationEntityTemplate.Lookup("gql_pagination_entity")
	require.NotNil(t, tmpl)
	src := tmpl.Tree.Root.String()

	// Verify type aliases are present.
	require.Contains(t, src, "PaginateOption") // PaginateOption type alias
	require.Contains(t, src, "OrderField")     // OrderField type alias
	require.Contains(t, src, "ToEdge")         // ToEdge var forwarder
	require.Contains(t, src, "Paginate")       // QueryPaginate var forwarder

	// Verify the template is a thin shim (no struct body, no method implementations).
	require.NotContains(t, src, "}} struct") // No Edge/Connection struct declarations
	require.NotContains(t, src, "applyOrder")
	require.NotContains(t, src, "applyCursors")
	require.NotContains(t, src, "applyFilter")
	require.NotContains(t, src, "orderExpr")
	require.NotContains(t, src, "MarshalGQL")
	require.NotContains(t, src, "UnmarshalGQL")

	// Verify no $Scope references remain.
	require.NotContains(t, src, "$.Scope")

	// Verify the template uses $.Node instead of range loop.
	require.Contains(t, src, "$.Node")
	require.NotContains(t, src, "range $node := $gqlNodes")
}

func TestPaginationEntityTemplateExecution(t *testing.T) {
	// Execute the template against the real todo schema graph for one entity
	// and verify the generated output contains expected thin re-export shim code.
	// After lever B-2, pagination_entity.tmpl emits only type aliases + var forwarders.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	// Find the Todo node.
	var todoNode *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "Todo" {
			todoNode = n
			break
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist in the schema")

	tmpl := PaginationEntityTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_pagination_entity", struct {
		*gen.Graph
		Node *gen.Type
	}{graph, todoNode})
	require.NoError(t, err)

	output := buf.String()

	// Verify the generated output contains the package declaration.
	require.Contains(t, output, "package ent")

	// Verify thin re-export shim — type aliases (= <subpkg>.<Type>).
	require.Contains(t, output, "TodoEdge")
	require.Contains(t, output, "TodoConnection")
	require.Contains(t, output, "TodoPaginateOption")
	require.Contains(t, output, "TodoOrderField")
	require.Contains(t, output, "TodoOrder")
	require.Contains(t, output, "DefaultTodoOrder")

	// Verify the type aliases reference the entity subpackage.
	require.Contains(t, output, "todo.TodoEdge")
	require.Contains(t, output, "todo.TodoConnection")
	require.Contains(t, output, "todo.DefaultTodoOrder")

	// Verify var forwarders for functions.
	require.Contains(t, output, "TodoQueryPaginate")
	require.Contains(t, output, "TodoToEdge")

	// Verify order field sentinel forwarders (Todo has OrderField annotations).
	require.Contains(t, output, "TodoOrderFieldCreatedAt")
	require.Contains(t, output, "TodoOrderFieldStatus")
	require.Contains(t, output, "TodoOrderFieldText")
	require.Contains(t, output, "TodoOrderFieldPriority")

	// Verify the thin shim does NOT contain the full body.
	require.NotContains(t, output, "TodoEdge struct")
	require.NotContains(t, output, "todoPager")
	require.NotContains(t, output, "func TodoQueryPaginate(")
	require.NotContains(t, output, "validateFirstLast(first, last)")
	require.NotContains(t, output, "pager.applyOrder")

	// Verify NO shared code is present (those stay in pagination_shared.tmpl).
	require.NotContains(t, output, "Cursor = entgql.Cursor[")
	require.NotContains(t, output, "PageInfo = entgql.PageInfo[")
	require.NotContains(t, output, "func validateFirstLast(")
	require.NotContains(t, output, "func paginateLimit(")
}

func TestPaginationEntityTemplateMultipleEntities(t *testing.T) {
	// Verify the template can be executed for multiple different entities
	// and produces distinct output for each.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	// Find both Todo and Category nodes.
	var todoNode, categoryNode *gen.Type
	for _, n := range graph.Nodes {
		switch n.Name {
		case "Todo":
			todoNode = n
		case "Category":
			categoryNode = n
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist")
	require.NotNil(t, categoryNode, "Category node should exist")

	tmpl := PaginationEntityTemplate

	// Generate for Todo.
	var todoBuf bytes.Buffer
	err = tmpl.ExecuteTemplate(&todoBuf, "gql_pagination_entity", struct {
		*gen.Graph
		Node *gen.Type
	}{graph, todoNode})
	require.NoError(t, err)
	todoOutput := todoBuf.String()

	// Generate for Category.
	var catBuf bytes.Buffer
	err = tmpl.ExecuteTemplate(&catBuf, "gql_pagination_entity", struct {
		*gen.Graph
		Node *gen.Type
	}{graph, categoryNode})
	require.NoError(t, err)
	catOutput := catBuf.String()

	// Verify Todo output has Todo-specific type aliases (thin re-export shim after lever B-2).
	require.Contains(t, todoOutput, "todo.TodoEdge")
	require.Contains(t, todoOutput, "todo.TodoConnection")
	require.NotContains(t, todoOutput, "CategoryEdge")
	require.NotContains(t, todoOutput, "CategoryConnection")

	// Verify Category output has Category-specific type aliases.
	require.Contains(t, catOutput, "category.CategoryEdge")
	require.Contains(t, catOutput, "category.CategoryConnection")
	require.NotContains(t, catOutput, "TodoEdge")
	require.NotContains(t, catOutput, "TodoConnection")
}

func TestPaginationSubpkgTemplateExecution(t *testing.T) {
	// PaginationSubpkgTemplate (798 lines, the single biggest change in the
	// gqlpage rewrite) has no dedicated content/execution test elsewhere --
	// TestGenerateSplitPagination only ever renders the root thin shim, since
	// it runs against a tmpDir with no entity sub-package directory. Render
	// the subpkg template directly for Todo (MultiOrder, plus edge-term order
	// fields CHILDREN_COUNT/PARENT_STATUS/CATEGORY_TEXT, so EdgeTermColumns is
	// exercised) and assert on the emitted text, once with the
	// EntGQLExtension annotation present and once absent -- gemini's app
	// always sets it, so the absent path (R11: MaxPageSize must render as the
	// literal 0, not be skipped or panic) is otherwise never exercised by any
	// real regen.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	var todoNode *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "Todo" {
			todoNode = n
			break
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist in the schema")

	render := func() string {
		var buf bytes.Buffer
		err := PaginationSubpkgTemplate.ExecuteTemplate(&buf, "gql_pagination_subpkg", struct {
			*gen.Graph
			Node *gen.Type
		}{graph, todoNode})
		require.NoError(t, err)
		return buf.String()
	}

	// --- without EntGQLExtension: MaxPageSize must render as 0 ---
	without := render()
	require.Contains(t, without, "package todo")

	// Five type aliases, all using "=" (gqlpage generic instantiations, not
	// hand-rolled struct/type declarations).
	require.Contains(t, without, "TodoEdge            = gqlpage.Edge[Todo,")
	require.Contains(t, without, "TodoConnection      = gqlpage.Connection[Todo,")
	require.Contains(t, without, "TodoOrder           = gqlpage.Order[Todo,")
	require.Contains(t, without, "TodoOrderField      = gqlpage.OrderField[Todo,")
	require.Contains(t, without, "TodoPaginateOption    = gqlpage.Option[TodoQuery, Todo,")

	// The Ops literal, with MultiOrder, MaxPageSize and (Todo has edge-term
	// order fields) EdgeTermColumns.
	require.Contains(t, without, "var TodoOps = &gqlpage.Ops[TodoQuery, Todo,")
	require.Contains(t, without, "MultiOrder:  true")
	require.Contains(t, without, "MaxPageSize: 0")
	require.Contains(t, without, "EdgeTermColumns:")

	// Thin forwarders.
	require.Contains(t, without, "func WithTodoOrder(")
	require.Contains(t, without, "func WithTodoFilter(")
	require.Contains(t, without, "func NewTodoPager(")
	require.Contains(t, without, "func TodoQueryPaginate(")
	require.Contains(t, without, "func TodoToEdge(")
	require.Contains(t, without, "gqlpage.Paginate(")

	// --- with EntGQLExtension: MaxPageSize must reflect the configured value ---
	graph.Annotations = gen.Annotations{
		ExtensionAnnotation{}.Name(): ExtensionAnnotation{MaxPageSize: 50},
	}
	with := render()
	require.Contains(t, with, "MaxPageSize: 50")
	require.NotContains(t, with, "MaxPageSize: 0")
}

func TestMutationInputSiblingTemplateExecution(t *testing.T) {
	// mutation_input_sibling.tmpl no longer hand-rolls each Mutate() body --
	// it emits a `mutate:"<op>:<name>"` struct tag per field/edge and
	// delegates to gqlinput.Mutate (Task 7 of the entgql codegen-reduction
	// project). The struct-tag correctness -- right op, right descriptor
	// name, right pairing/order -- has no other coverage anywhere in this
	// repo, so assert directly on the rendered text for Todo, which exercises
	// every op the walker supports except fa/AppendField (Todo has no
	// JSON-slice field annotated for it; that pairing is covered by
	// gqlinput's own tests and by the Task 7 gemini parity harness against a
	// real entity that has one).
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	var todoNode *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "Todo" {
			todoNode = n
			break
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist in the schema")

	inputs := []*MutationDescriptor{
		{Type: todoNode, IsCreate: true},
		{Type: todoNode, IsCreate: false},
	}
	var buf bytes.Buffer
	err = MutationInputSiblingTemplate.Execute(&buf, struct {
		*gen.Graph
		EntityName string
		Inputs     []*MutationDescriptor
	}{graph, "Todo", inputs})
	require.NoError(t, err)
	out := buf.String()

	// gqlinput, not the old entbuilder.ToAny-based dispatch.
	require.Contains(t, out, `"entgo.io/contrib/entgql/gqlinput"`)
	require.NotContains(t, out, "entbuilder")

	// Every Mutate body collapses to a single delegating call, and the old
	// per-field/edge dispatch (SetField/ClearField/... calls) is gone from
	// the template entirely.
	require.Contains(t, out, "func (i CreateTodoInput) Mutate(m *todo.TodoMutation) {\n        gqlinput.Mutate(i, m)\n    }")
	require.Contains(t, out, "func (i UpdateTodoInput) Mutate(m *todo.TodoMutation) {\n        gqlinput.Mutate(i, m)\n    }")
	require.NotContains(t, out, "SetField(")
	require.NotContains(t, out, "ClearField(")
	require.NotContains(t, out, "SetEdgeID(")

	// "text" is required (NotEmpty, no Default/Optional) on create: no
	// pointer, no ClearOp -- the unconditional f: path.
	require.Contains(t, out, "Text string `mutate:\"f:text\"`")
	require.NotContains(t, out, "ClearText")

	// "init" is Optional on update: fc: immediately precedes f: for the same
	// descriptor name, on adjacent lines -- the Clear-then-Set declaration
	// order the walker's buildPlan relies on.
	require.Contains(t, out, "ClearInit bool `mutate:\"fc:init\"`\n            Init map[string]interface {} `mutate:\"f:init\"`")

	// "children" (non-unique, To Todo) on create: ea: with the []<id-type>ID
	// naming.
	require.Contains(t, out, "ChildIDs []int `mutate:\"ea:children\"`")
	// On update: ec: then ea: then er:, adjacent, same descriptor name.
	require.Contains(t, out, "ClearChildren bool `mutate:\"ec:children\"`\n                    AddChildIDs []int `mutate:\"ea:children\"`\n                    RemoveChildIDs []int `mutate:\"er:children\"`")

	// "parent" (unique, self-referential, optional) on update: ec: then e:,
	// adjacent, same descriptor name -- the edge analogue of the fc:/f: test
	// above.
	require.Contains(t, out, "ClearParent bool `mutate:\"ec:parent\"`\n                ParentID *int `mutate:\"e:parent\"`")

	// "category" (unique, Immutable) is excluded from the update input
	// entirely, but still present -- unpaired with a Clear -- on create.
	require.Contains(t, out, "CategoryID *int `mutate:\"e:category\"`")
	require.Equal(t, 1, strings.Count(out, "category"),
		"the immutable category edge must appear only in CreateTodoInput, not UpdateTodoInput")
}

func TestMutationInputSiblingTemplateExecution_AppendPairing(t *testing.T) {
	// Fix for a review finding on Task 7: TestMutationInputSiblingTemplateExecution's
	// Todo fixture has no JSON-slice field, so it structurally never emits an
	// fa: tag -- the append-guard/value pairing this template must preserve
	// (gqlinput's buildPlan pairs an fa:<name> field with the nearest
	// preceding f:<name> field of the SAME descriptor name, by declaration
	// order) had no persisted coverage of what THIS TEMPLATE emits. Can't add
	// a JSON-slice field to the todo schema (entgql/internal/todo* fixtures
	// must not be regenerated), so build one synthetic append-eligible field
	// directly instead. Reuses a loaded graph purely for its Config
	// (Header/Package) -- nothing else graph-wide is needed on this path.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	widget := &gen.Type{
		Name: "Widget",
		Fields: []*gen.Field{
			{
				Name: "tags",
				Type: &field.TypeInfo{
					Type:     field.TypeJSON,
					Ident:    "[]string",
					Nillable: true,
					RType:    &field.RType{Kind: reflect.Slice},
				},
			},
		},
	}
	inputs := []*MutationDescriptor{
		{Type: widget, IsCreate: false}, // AppendOp only ever fires on update.
	}
	var buf bytes.Buffer
	err = MutationInputSiblingTemplate.Execute(&buf, struct {
		*gen.Graph
		EntityName string
		Inputs     []*MutationDescriptor
	}{graph, "Widget", inputs})
	require.NoError(t, err)
	out := buf.String()

	// The invariant that matters: f:tags immediately precedes fa:tags, same
	// descriptor name, value-then-append order -- exactly what buildPlan
	// requires to pair them (it panics otherwise). Discriminating against
	// all three failure modes at once: wrong op letter, diverged name, or
	// swapped order would all fail this single Contains.
	require.Contains(t, out,
		"Tags []string `mutate:\"f:tags\"`\n                AppendTags []string `mutate:\"fa:tags\"`",
		"f:tags must be immediately followed by its paired fa:tags, same descriptor name")
}

func TestFilterFields(t *testing.T) {
	fields, err := filterFields([]*gen.Field{
		{
			Name: "Field1",
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{},
			},
		},
		{
			Name: "Field2",
		},
		{
			Name: "SkippedField",
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{"Skip": SkipAll},
			},
		},
	}, SkipType)
	require.NoError(t, err)
	require.Equal(t, []*gen.Field{
		{
			Name: "Field1",
			Annotations: map[string]interface{}{
				annotationName: map[string]interface{}{},
			},
		},
		{
			Name: "Field2",
		},
	}, fields)
}

func TestCollectionEntityTemplateParsed(t *testing.T) {
	// Verify the CollectionEntityTemplate was parsed successfully during init().
	require.NotNil(t, CollectionEntityTemplate, "CollectionEntityTemplate should be parsed during init()")
	// Verify the template has the expected name.
	require.Equal(t, "gql_collection_entity", CollectionEntityTemplate.Name())
	// Verify it has the expected define block.
	tmpl := CollectionEntityTemplate.Lookup("gql_collection_entity")
	require.NotNil(t, tmpl, "template should contain 'gql_collection_entity' define block")
}

func TestCollectionEntityTemplateContent(t *testing.T) {
	// After lever B-3d, the entity template is a thin var-forwarder shim;
	// the body lives in CollectionSubpkgTemplate.
	tmpl := CollectionEntityTemplate.Lookup("gql_collection_entity")
	require.NotNil(t, tmpl)
	src := tmpl.Tree.Root.String()

	// The shim references gqlcollections and forwards CollectFields.
	require.Contains(t, src, "gqlcollections")
	require.Contains(t, src, "QueryName")

	// No load_total helper template call.
	require.NotContains(t, src, `template "gql_pagination/helper/load_total"`)

	// No Scope references remain.
	require.NotContains(t, src, "Scope")

	// References $.Node for single entity.
	require.Contains(t, src, "$.Node")
}

func TestCollectionSubpkgTemplateContent(t *testing.T) {
	// Verify the subpkg template (body owner after B-3d) contains the per-entity logic.
	tmpl := CollectionSubpkgTemplate.Lookup("gql_collection_subpkg")
	require.NotNil(t, tmpl)
	src := tmpl.Tree.Root.String()

	require.Contains(t, src, "CollectFields")
	require.Contains(t, src, "collectField")
	require.Contains(t, src, "PaginateArgs")
	require.Contains(t, src, "package gqlcollections")

	// Verify load_total helper is inlined (no template call).
	require.NotContains(t, src, `template "gql_pagination/helper/load_total"`)

	// Verify no $Scope references remain.
	require.NotContains(t, src, "Scope")

	// Verify $.HasWhereInputTemplate is used.
	require.Contains(t, src, "HasWhereInputTemplate")

	// Verify it references $.Node (single entity, not range loop).
	require.Contains(t, src, "$.Node")
}

func TestCollectionEntityTemplateExecution(t *testing.T) {
	// After lever B-3d, the entity template is a thin shim. Verify it forwards
	// to the gqlcollections sibling subpackage.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	var todoNode *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "Todo" {
			todoNode = n
			break
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist in the schema")

	tmpl := CollectionEntityTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_collection_entity", struct {
		*gen.Graph
		Node                  *gen.Type
		HasWhereInputTemplate bool
		HasPaginationSubpkg   bool
	}{graph, todoNode, true, false})
	require.NoError(t, err)

	output := buf.String()

	// Verify the generated output contains the package declaration.
	require.Contains(t, output, "package ent")

	// Verify the shim imports gqlcollections and forwards CollectFields.
	require.Contains(t, output, "gqlcollections")
	require.Contains(t, output, "TodoQueryCollectFields = gqlcollections.TodoQueryCollectFields")
}

func TestCollectionSubpkgTemplateExecution(t *testing.T) {
	// Execute the subpkg template against the real todo schema graph and verify
	// the generated output contains expected per-entity code.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	// Find the Todo node.
	var todoNode *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "Todo" {
			todoNode = n
			break
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist in the schema")

	tmpl := CollectionSubpkgTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_collection_subpkg", struct {
		*gen.Graph
		Node                  *gen.Type
		HasWhereInputTemplate bool
		HasPaginationSubpkg   bool
	}{graph, todoNode, true, false}) // HasPaginationSubpkg=false: todo fixture has no gql_pagination.go in subpkg
	require.NoError(t, err)

	output := buf.String()

	// Verify the generated output contains the gqlcollections package declaration.
	require.Contains(t, output, "package gqlcollections")

	// Verify CollectFields free function is generated for the Todo query.
	require.Contains(t, output, "func TodoQueryCollectFields(_q *todo.TodoQuery, ctx context.Context, satisfies ...string) (*todo.TodoQuery, error)")
	require.Contains(t, output, "func collectFieldTodoQuery(_q *todo.TodoQuery, ctx context.Context, oneNode bool")

	// Verify PaginateArgs struct is generated.
	require.Contains(t, output, "todoPaginateArgs")
	require.Contains(t, output, "newTodoPaginateArgs")

	// Verify no template call to load_total (it should be inlined).
	require.NotContains(t, output, "gql_pagination/helper/load_total")

	// Verify no Scope references in output.
	require.NotContains(t, output, "Scope")

	// Verify where input filter is present (since HasWhereInputTemplate=true).
	require.Contains(t, output, "whereField")

	// Verify per-entity imports.
	require.Contains(t, output, `"entgo.io/contrib/entgql/internal/todo/ent/todo"`)
}

func TestCollectionEntityTemplateNoWhereInput(t *testing.T) {
	// After lever B-3d, the entity template is a thin shim. Verify it forwards
	// to gqlcollections even when HasWhereInputTemplate=false.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	var node *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "BillProduct" {
			node = n
			break
		}
	}
	require.NotNil(t, node, "BillProduct node should exist in the schema")

	tmpl := CollectionEntityTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_collection_entity", struct {
		*gen.Graph
		Node                  *gen.Type
		HasWhereInputTemplate bool
		HasPaginationSubpkg   bool
	}{graph, node, false, false})
	require.NoError(t, err)

	output := buf.String()

	require.Contains(t, output, "package ent")
	require.Contains(t, output, "BillProductQueryCollectFields = gqlcollections.BillProductQueryCollectFields")
}

func TestCollectionSubpkgTemplateNoWhereInput(t *testing.T) {
	// Execute the subpkg template with HasWhereInputTemplate=false and verify
	// the where input filter code is not generated.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	var node *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "BillProduct" {
			node = n
			break
		}
	}
	require.NotNil(t, node, "BillProduct node should exist in the schema")

	tmpl := CollectionSubpkgTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_collection_subpkg", struct {
		*gen.Graph
		Node                  *gen.Type
		HasWhereInputTemplate bool
		HasPaginationSubpkg   bool
	}{graph, node, false, false})
	require.NoError(t, err)

	output := buf.String()

	require.Contains(t, output, "package gqlcollections")

	// Verify CollectFields free function is present.
	require.Contains(t, output, "func BillProductQueryCollectFields(_q *billproduct.BillProductQuery, ctx context.Context, satisfies ...string) (*billproduct.BillProductQuery, error)")

	// Verify PaginateArgs struct is generated.
	require.Contains(t, output, "billproductPaginateArgs")
	require.Contains(t, output, "newBillProductPaginateArgs")

	// HasWhereInputTemplate=false: no where input filter assignment.
	require.NotContains(t, output, "BillProductWhereInput")
}

func TestEdgeEntityTemplateParsed(t *testing.T) {
	// Verify the EdgeEntityTemplate was parsed successfully during init().
	require.NotNil(t, EdgeEntityTemplate, "EdgeEntityTemplate should be parsed during init()")
	// Verify the template has the expected name.
	require.Equal(t, "gql_edge_entity", EdgeEntityTemplate.Name())
	// Verify it has the expected define block.
	tmpl := EdgeEntityTemplate.Lookup("gql_edge_entity")
	require.NotNil(t, tmpl, "template should contain 'gql_edge_entity' define block")
}

func TestEdgeEntityTemplateContent(t *testing.T) {
	// After lever B-3c, the entity template is a thin var-forwarder shim;
	// the body lives in EdgeSubpkgTemplate.
	tmpl := EdgeEntityTemplate.Lookup("gql_edge_entity")
	require.NotNil(t, tmpl)
	src := tmpl.Tree.Root.String()

	// The shim references gqledges and forwards Resolve* funcs.
	require.Contains(t, src, "gqledges")
	require.Contains(t, src, "Register")
	require.Contains(t, src, "ClientFromCtx")
	require.Contains(t, src, "Resolve")

	// No Scope references remain.
	require.NotContains(t, src, "$.Scope")

	// References $.Node for single entity.
	require.Contains(t, src, "$.Node")
}

func TestEdgeSubpkgTemplateContent(t *testing.T) {
	// Verify the subpkg template (body owner after B-3c) contains the per-edge logic.
	tmpl := EdgeSubpkgTemplate.Lookup("gql_edge_subpkg")
	require.NotNil(t, tmpl)
	src := tmpl.Tree.Root.String()

	require.Contains(t, src, "package gqledges")

	// Verify the paginate helper is inlined (no template call).
	require.NotContains(t, src, `template "gql_edge/helper/paginate"`)

	// Verify no Scope references remain.
	require.NotContains(t, src, "$.Scope")

	// Verify $.HasWhereInputTemplate is used.
	require.Contains(t, src, "HasWhereInputTemplate")

	// Verify it references $.Node.
	require.Contains(t, src, "$.Node")

	// Verify all three edge types are handled.
	require.Contains(t, src, "isRelayConn")
	require.Contains(t, src, "IsNotLoaded")
	require.Contains(t, src, "MaskNotFound")

	// Verify Relay connection inlined code has expected elements.
	require.Contains(t, src, "nodePaginationNames")
	require.Contains(t, src, "Paginate")
}

func TestEdgeEntityTemplateExecution(t *testing.T) {
	// After lever B-3c, the entity template is a thin shim. Verify it forwards
	// to the gqledges sibling subpackage.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	var todoNode *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "Todo" {
			todoNode = n
			break
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist in the schema")

	tmpl := EdgeEntityTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_edge_entity", struct {
		*gen.Graph
		Node                  *gen.Type
		HasWhereInputTemplate bool
	}{graph, todoNode, true})
	require.NoError(t, err)

	output := buf.String()

	require.Contains(t, output, "package ent")

	// The shim forwards Resolve* funcs to gqledges and registers a client accessor.
	require.Contains(t, output, "gqledges.ResolveTodo")
	require.Contains(t, output, "gqledges.RegisterTodoClientFromCtx")
}

func TestEdgeSubpkgTemplateExecution(t *testing.T) {
	// Execute the subpkg template against the real todo schema graph for Todo entity
	// and verify the generated output contains expected edge resolver code.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	var todoNode *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "Todo" {
			todoNode = n
			break
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist in the schema")

	tmpl := EdgeSubpkgTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_edge_subpkg", struct {
		*gen.Graph
		Node                  *gen.Type
		HasWhereInputTemplate bool
	}{graph, todoNode, true})
	require.NoError(t, err)

	output := buf.String()

	require.Contains(t, output, "package gqledges")

	// Verify edge resolver standalone functions are generated.
	require.Contains(t, output, "func ResolveTodo")

	// Verify per-entity client accessor wiring.
	require.Contains(t, output, "RegisterTodoClientFromCtx")
	require.Contains(t, output, "todoClientFromCtx")

	// Verify import statements.
	require.Contains(t, output, `"context"`)
	require.Contains(t, output, `"github.com/99designs/gqlgen/graphql"`)

	// Verify no template call to gql_edge/helper/paginate (it should be inlined).
	require.NotContains(t, output, "gql_edge/helper/paginate")

	// Verify no Scope references in output.
	require.NotContains(t, output, "Scope")
}

func TestEdgeEntityTemplateNoWhereInput(t *testing.T) {
	// After B-3c the entity template is a thin shim. The HasWhereInputTemplate flag
	// matters only for the subpkg template. Verify the shim still produces forwarders.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	var todoNode *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "Todo" {
			todoNode = n
			break
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist in the schema")

	tmpl := EdgeEntityTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_edge_entity", struct {
		*gen.Graph
		Node                  *gen.Type
		HasWhereInputTemplate bool
	}{graph, todoNode, false})
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "package ent")
	require.Contains(t, output, "gqledges.ResolveTodo")
}

func TestEdgeSubpkgTemplateNoWhereInput(t *testing.T) {
	// Execute the subpkg template with HasWhereInputTemplate=false and verify the where
	// input filter code is not generated.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	var todoNode *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "Todo" {
			todoNode = n
			break
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist in the schema")

	tmpl := EdgeSubpkgTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_edge_subpkg", struct {
		*gen.Graph
		Node                  *gen.Type
		HasWhereInputTemplate bool
	}{graph, todoNode, false})
	require.NoError(t, err)

	output := buf.String()

	require.Contains(t, output, "package gqledges")
	require.Contains(t, output, "func ResolveTodo")
	require.NotContains(t, output, "WhereInput",
		"where input should not be generated when HasWhereInputTemplate=false")
}

func TestEdgeEntityTemplateMultipleEntities(t *testing.T) {
	// Verify the template can be executed for multiple different entities
	// and produces distinct output for each.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	// Find both Todo and Category nodes.
	var todoNode, categoryNode *gen.Type
	for _, n := range graph.Nodes {
		switch n.Name {
		case "Todo":
			todoNode = n
		case "Category":
			categoryNode = n
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist")
	require.NotNil(t, categoryNode, "Category node should exist")

	tmpl := EdgeEntityTemplate

	// Generate for Todo.
	var todoBuf bytes.Buffer
	err = tmpl.ExecuteTemplate(&todoBuf, "gql_edge_entity", struct {
		*gen.Graph
		Node                  *gen.Type
		HasWhereInputTemplate bool
	}{graph, todoNode, true})
	require.NoError(t, err)
	todoOutput := todoBuf.String()

	// Generate for Category.
	var catBuf bytes.Buffer
	err = tmpl.ExecuteTemplate(&catBuf, "gql_edge_entity", struct {
		*gen.Graph
		Node                  *gen.Type
		HasWhereInputTemplate bool
	}{graph, categoryNode, true})
	require.NoError(t, err)
	catOutput := catBuf.String()

	// Verify Todo output has Todo-specific forwarders.
	require.Contains(t, todoOutput, "ResolveTodo")
	require.NotContains(t, todoOutput, "ResolveCategory")

	// Verify Category output has Category-specific forwarders.
	require.Contains(t, catOutput, "ResolveCategory")
	require.NotContains(t, catOutput, "ResolveTodo")
}

func TestNodeDescriptorSharedTemplateParsed(t *testing.T) {
	// Verify the NodeDescriptorSharedTemplate was parsed successfully during init().
	require.NotNil(t, NodeDescriptorSharedTemplate, "NodeDescriptorSharedTemplate should be parsed during init()")
	// Verify the template has the expected name.
	require.Equal(t, "gql_node_descriptor_shared", NodeDescriptorSharedTemplate.Name())
	// Verify it has the expected define block.
	tmpl := NodeDescriptorSharedTemplate.Lookup("gql_node_descriptor_shared")
	require.NotNil(t, tmpl, "template should contain 'gql_node_descriptor_shared' define block")
}

func TestNodeDescriptorSharedTemplateContent(t *testing.T) {
	// Verify the template source contains the expected shared code elements.
	tmpl := NodeDescriptorSharedTemplate.Lookup("gql_node_descriptor_shared")
	require.NotNil(t, tmpl)
	src := tmpl.Tree.Root.String()

	// Verify shared struct types are present.
	require.Contains(t, src, "Node struct")
	require.Contains(t, src, "Field struct")
	require.Contains(t, src, "Edge struct")

	// Verify Client.Node() method is present.
	require.Contains(t, src, "func (c *Client) Node(")
	require.Contains(t, src, "c.noder(ctx, table, id)")

	// Verify node descriptor registry is present.
	require.Contains(t, src, "nodeDescriptors")
	require.Contains(t, src, "registerNodeDescriptor")

	// Verify NO per-entity code is present.
	// Note: $.Nodes (plural) is expected in the shared template for filterNodes;
	// we check that $.Node (singular entity reference) is not used as a data field.
	require.NotContains(t, src, "$.Node }}")
	require.NotContains(t, src, "$.Node.Name")
	require.NotContains(t, src, "json.Marshal")
	require.NotContains(t, src, "QueryTodos")
	require.NotContains(t, src, "Noder interface")
}

func TestNodeDescriptorSharedTemplateExecution(t *testing.T) {
	// Execute the template against the real todo schema graph and verify
	// the generated output contains expected shared code.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	tmpl := NodeDescriptorSharedTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_node_descriptor_shared", struct {
		*gen.Graph
	}{graph})
	require.NoError(t, err)

	output := buf.String()

	// Verify the generated output contains the package declaration.
	require.Contains(t, output, "package ent")

	// Verify shared struct types are generated.
	require.Contains(t, output, "type Node struct")
	require.Contains(t, output, "type Field struct")
	require.Contains(t, output, "type Edge struct")

	// Verify Client.Node() method is generated.
	require.Contains(t, output, "func (c *Client) Node(ctx context.Context, id int) (*Node, error)")
	require.Contains(t, output, "c.noder(ctx, table, id)")

	// Verify node descriptor registry is generated.
	require.Contains(t, output, "var nodeDescriptors")
	require.Contains(t, output, "func registerNodeDescriptor(")

	// Verify NO per-entity code is present.
	require.False(t, strings.Contains(output, "BillProductNode("),
		"shared template should not generate per-entity BillProductNode() function")
	require.False(t, strings.Contains(output, "TodoNode("),
		"shared template should not generate per-entity TodoNode() function")
	require.False(t, strings.Contains(output, "json.Marshal"),
		"shared template should not contain json.Marshal (per-entity code)")
}

func TestNodeDescriptorEntityTemplateParsed(t *testing.T) {
	// Verify the NodeDescriptorEntityTemplate was parsed successfully during init().
	require.NotNil(t, NodeDescriptorEntityTemplate, "NodeDescriptorEntityTemplate should be parsed during init()")
	// Verify the template has the expected name.
	require.Equal(t, "gql_node_descriptor_entity", NodeDescriptorEntityTemplate.Name())
	// Verify it has the expected define block.
	tmpl := NodeDescriptorEntityTemplate.Lookup("gql_node_descriptor_entity")
	require.NotNil(t, tmpl, "template should contain 'gql_node_descriptor_entity' define block")
}

func TestNodeDescriptorEntityTemplateContent(t *testing.T) {
	// Verify the template source contains the expected per-entity code elements.
	tmpl := NodeDescriptorEntityTemplate.Lookup("gql_node_descriptor_entity")
	require.NotNil(t, tmpl)
	src := tmpl.Tree.Root.String()

	// Verify per-entity node descriptor function is present.
	require.Contains(t, src, "registerNodeDescriptor")
	require.Contains(t, src, "json.Marshal")
	require.Contains(t, src, "$.Node")

	// Verify NO shared code is present.
	require.NotContains(t, src, "Node struct")
	require.NotContains(t, src, "Field struct")
	require.NotContains(t, src, "Edge struct")
	require.NotContains(t, src, "func (c *Client) Node(")
}

func TestNodeDescriptorEntityTemplateExecution(t *testing.T) {
	// Execute the template against the real todo schema graph for one entity
	// and verify the generated output contains expected per-entity code.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	// Find the Todo node.
	var todoNode *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "Todo" {
			todoNode = n
			break
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist in the schema")

	tmpl := NodeDescriptorEntityTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_node_descriptor_entity", struct {
		*gen.Graph
		Node *gen.Type
	}{graph, todoNode})
	require.NoError(t, err)

	output := buf.String()

	// Verify the generated output contains the package declaration.
	require.Contains(t, output, "package ent")

	// Verify per-entity standalone node descriptor function is generated for Todo.
	require.Contains(t, output, "func TodoNode(_m *Todo, ctx context.Context) (node *Node, err error)")

	// Verify node descriptor registration.
	require.Contains(t, output, "registerNodeDescriptor(todo.Table")

	// Verify field serialization is present.
	require.Contains(t, output, "json.Marshal(_m.CreatedAt)")
	require.Contains(t, output, "json.Marshal(_m.Status)")
	require.Contains(t, output, "json.Marshal(_m.Priority)")
	require.Contains(t, output, "json.Marshal(_m.Text)")

	// Verify edge queries use the root-facade Query<Node><Edge> free function
	// (chained from FromContext(ctx).<Node>) so they resolve in a package where
	// <Node>Client is an alias and the sub-package Client has no Query<Edge> method.
	require.Contains(t, output, "QueryTodoParent(FromContext(ctx).Todo, _m)")
	require.Contains(t, output, "QueryTodoChildren(FromContext(ctx).Todo, _m)")
	require.Contains(t, output, "QueryTodoCategory(FromContext(ctx).Todo, _m)")

	// Verify edge type imports.
	require.Contains(t, output, `"entgo.io/contrib/entgql/internal/todo/ent/todo"`)
	require.Contains(t, output, `"entgo.io/contrib/entgql/internal/todo/ent/category"`)

	// Verify NO shared code is present.
	require.NotContains(t, output, "type Node struct")
	require.NotContains(t, output, "type Field struct")
	require.NotContains(t, output, "type Edge struct")
	require.NotContains(t, output, "func (c *Client) Node(")
}

func TestNodeDescriptorEntityTemplateMultipleEntities(t *testing.T) {
	// Verify the template can be executed for multiple different entities
	// and produces distinct output for each.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	// Find both Todo and BillProduct nodes.
	var todoNode, billProductNode *gen.Type
	for _, n := range graph.Nodes {
		switch n.Name {
		case "Todo":
			todoNode = n
		case "BillProduct":
			billProductNode = n
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist")
	require.NotNil(t, billProductNode, "BillProduct node should exist")

	tmpl := NodeDescriptorEntityTemplate

	// Generate for Todo.
	var todoBuf bytes.Buffer
	err = tmpl.ExecuteTemplate(&todoBuf, "gql_node_descriptor_entity", struct {
		*gen.Graph
		Node *gen.Type
	}{graph, todoNode})
	require.NoError(t, err)
	todoOutput := todoBuf.String()

	// Generate for BillProduct.
	var bpBuf bytes.Buffer
	err = tmpl.ExecuteTemplate(&bpBuf, "gql_node_descriptor_entity", struct {
		*gen.Graph
		Node *gen.Type
	}{graph, billProductNode})
	require.NoError(t, err)
	bpOutput := bpBuf.String()

	// Verify Todo output has Todo-specific standalone function.
	require.Contains(t, todoOutput, "func TodoNode(_m *Todo, ctx context.Context)")
	require.NotContains(t, todoOutput, "BillProductNode(")

	// Verify BillProduct output has BillProduct-specific standalone function.
	require.Contains(t, bpOutput, "func BillProductNode(_m *BillProduct, ctx context.Context)")
	require.NotContains(t, bpOutput, "TodoNode(")

	// BillProduct has no edges, so no edge queries should appear.
	require.NotContains(t, bpOutput, "QueryTodos")
	require.NotContains(t, bpOutput, "QueryParent")

	// BillProduct has fields: name, sku, quantity.
	require.Contains(t, bpOutput, "json.Marshal(_m.Name)")
	require.Contains(t, bpOutput, "json.Marshal(_m.Sku)")
	require.Contains(t, bpOutput, "json.Marshal(_m.Quantity)")
}

func TestNodeSharedTemplateParsed(t *testing.T) {
	// Verify the NodeSharedTemplate was parsed successfully during init().
	require.NotNil(t, NodeSharedTemplate, "NodeSharedTemplate should be parsed during init()")
	// Verify the template has the expected name.
	require.Equal(t, "gql_node_shared", NodeSharedTemplate.Name())
	// Verify it has the expected define block.
	tmpl := NodeSharedTemplate.Lookup("gql_node_shared")
	require.NotNil(t, tmpl, "template should contain 'gql_node_shared' define block")
}

func TestNodeSharedTemplateContent(t *testing.T) {
	// Verify the template source contains the expected shared code elements.
	tmpl := NodeSharedTemplate.Lookup("gql_node_shared")
	require.NotNil(t, tmpl)
	src := tmpl.Tree.Root.String()

	// Verify shared types and infrastructure are present.
	require.Contains(t, src, "Noder interface")
	require.Contains(t, src, "errNodeInvalidID")
	require.Contains(t, src, "NodeOption")
	require.Contains(t, src, "WithNodeType")
	require.Contains(t, src, "WithFixedNodeType")
	require.Contains(t, src, "nodeOptions struct")

	// Verify new map-based dispatch infrastructure.
	require.Contains(t, src, "nodeResolver struct")
	require.Contains(t, src, "nodeResolvers")
	require.Contains(t, src, "registerNodeResolver")

	// Verify Client methods are present.
	require.Contains(t, src, "func (c *Client) Noder(")
	require.Contains(t, src, "func (c *Client) noder(")
	require.Contains(t, src, "func (c *Client) Noders(")
	require.Contains(t, src, "func (c *Client) noders(")
	require.Contains(t, src, "func (c *Client) newNodeOpts(")

	// Verify map-based dispatch instead of switch.
	require.Contains(t, src, "nodeResolvers[table]")
	require.Contains(t, src, "resolver.byID(")
	require.Contains(t, src, "resolver.byIDs(")

	// Verify NO per-entity code is present.
	require.NotContains(t, src, "range $n")
	require.NotContains(t, src, "$.Node.Name")
	require.NotContains(t, src, "nodeImplementorsVar")

	// Verify no hasTemplate references.
	require.NotContains(t, src, "hasTemplate",
		"shared template should not use hasTemplate")
}

func TestNodeSharedTemplateExecution(t *testing.T) {
	// Execute the template against the real todo schema graph and verify
	// the generated output contains expected shared code.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	tmpl := NodeSharedTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_node_shared", struct {
		*gen.Graph
		HasNodeDescriptorTemplate bool
	}{graph, true})
	require.NoError(t, err)

	output := buf.String()

	// Verify the generated output contains the package declaration.
	require.Contains(t, output, "package ent")

	// Verify Noder interface is generated with IsNode() method.
	require.Contains(t, output, "type Noder interface")
	require.Contains(t, output, "IsNode()")
	require.NotContains(t, output, "Node(context.Context) (*Node, error)")

	// Verify shared infrastructure types are generated.
	require.Contains(t, output, "var errNodeInvalidID")
	require.Contains(t, output, "type NodeOption func(*nodeOptions)")
	require.Contains(t, output, "func WithNodeType(")
	require.Contains(t, output, "func WithFixedNodeType(")

	// Verify new map-based dispatch types.
	require.Contains(t, output, "type nodeResolver struct")
	require.Contains(t, output, "var nodeResolvers = map[string]nodeResolver{}")
	require.Contains(t, output, "func registerNodeResolver(")

	// Verify Client methods.
	require.Contains(t, output, "func (c *Client) Noder(ctx context.Context, id int")
	require.Contains(t, output, "func (c *Client) Noders(ctx context.Context, ids []int")

	// Verify NO per-entity code is present (no entity package imports, no switch cases per entity).
	require.NotContains(t, output, `"/todo"`)
	require.NotContains(t, output, `"/category"`)
	require.NotContains(t, output, "case todo.Table")
	require.NotContains(t, output, "case category.Table")
}

func TestNodeSharedTemplateExecution_WithoutNodeDescriptor(t *testing.T) {
	// Execute with HasNodeDescriptorTemplate=false and verify Node() method is not in Noder interface.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	tmpl := NodeSharedTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_node_shared", struct {
		*gen.Graph
		HasNodeDescriptorTemplate bool
	}{graph, false})
	require.NoError(t, err)

	output := buf.String()

	// Noder interface should still exist but without Node() method.
	require.Contains(t, output, "type Noder interface")
	require.Contains(t, output, "IsNode()")
	require.NotContains(t, output, "Node(context.Context) (*Node, error)")
}

func TestNodeEntityTemplateParsed(t *testing.T) {
	// Verify the NodeEntityTemplate was parsed successfully during init().
	require.NotNil(t, NodeEntityTemplate, "NodeEntityTemplate should be parsed during init()")
	// Verify the template has the expected name.
	require.Equal(t, "gql_node_entity", NodeEntityTemplate.Name())
	// Verify it has the expected define block.
	tmpl := NodeEntityTemplate.Lookup("gql_node_entity")
	require.NotNil(t, tmpl, "template should contain 'gql_node_entity' define block")
}

func TestNodeEntityTemplateContent(t *testing.T) {
	// Verify the template source contains the expected per-entity code elements.
	tmpl := NodeEntityTemplate.Lookup("gql_node_entity")
	require.NotNil(t, tmpl)
	src := tmpl.Tree.Root.String()

	// Verify per-entity code is present.
	require.Contains(t, src, "nodeImplementorsVar")
	require.Contains(t, src, "nodeImplementors")
	require.Contains(t, src, "$.Node")
	require.Contains(t, src, "registerNodeResolver")
	require.Contains(t, src, "func init()")

	// Verify HasCollectionTemplate is used instead of hasTemplate.
	require.NotContains(t, src, "hasTemplate",
		"entity template should use $.HasCollectionTemplate instead of hasTemplate")
	require.Contains(t, src, "HasCollectionTemplate",
		"entity template should reference HasCollectionTemplate")

	// Verify NO shared code is present.
	require.NotContains(t, src, "Noder interface")
	require.NotContains(t, src, "errNodeInvalidID")
	require.NotContains(t, src, "NodeOption")
	require.NotContains(t, src, "func (c *Client) Noder(")
	require.NotContains(t, src, "func (c *Client) Noders(")
}

func TestNodeEntityTemplateExecution(t *testing.T) {
	// Execute the template against the real todo schema graph for one entity
	// and verify the generated output contains expected per-entity code.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	// Find the Todo node.
	var todoNode *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "Todo" {
			todoNode = n
			break
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist in the schema")

	tmpl := NodeEntityTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_node_entity", struct {
		*gen.Graph
		Node                  *gen.Type
		HasCollectionTemplate bool
	}{graph, todoNode, true})
	require.NoError(t, err)

	output := buf.String()

	// Verify the generated output contains the package declaration.
	require.Contains(t, output, "package ent")

	// Verify implementors var.
	require.Contains(t, output, "todoImplementors")

	// Verify init() registration.
	require.Contains(t, output, "func init()")
	require.Contains(t, output, "registerNodeResolver(todo.Table")
	require.Contains(t, output, "todoNoder")
	require.Contains(t, output, "todoNoders")

	// Verify noder functions.
	require.Contains(t, output, "func todoNoder(ctx context.Context, c *Client, id int) (Noder, error)")
	require.Contains(t, output, "func todoNoders(ctx context.Context, c *Client, ids []int")
	require.Contains(t, output, "c.Todo.Query()")
	require.Contains(t, output, "todo.ID(id)")
	require.Contains(t, output, "todo.IDIn(ids...)")

	// Verify collectField is present since HasCollectionTemplate=true.
	require.Contains(t, output, "collectField")

	// Verify entity package import.
	require.Contains(t, output, `"entgo.io/contrib/entgql/internal/todo/ent/todo"`)

	// Verify NO shared code is present.
	require.NotContains(t, output, "type Noder interface")
	require.NotContains(t, output, "var errNodeInvalidID")
	require.NotContains(t, output, "func (c *Client) Noder(")
	require.NotContains(t, output, "func (c *Client) Noders(")
}

func TestNodeEntityTemplateExecution_WithoutCollection(t *testing.T) {
	// Execute with HasCollectionTemplate=false and verify collectField is absent.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	var todoNode *gen.Type
	for _, n := range graph.Nodes {
		if n.Name == "Todo" {
			todoNode = n
			break
		}
	}
	require.NotNil(t, todoNode)

	tmpl := NodeEntityTemplate
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "gql_node_entity", struct {
		*gen.Graph
		Node                  *gen.Type
		HasCollectionTemplate bool
	}{graph, todoNode, false})
	require.NoError(t, err)

	output := buf.String()

	// Verify collectField is NOT present since HasCollectionTemplate=false.
	require.NotContains(t, output, "collectField")
	require.NotContains(t, output, "CollectFields")

	// But the core noder functions should still be present.
	require.Contains(t, output, "func todoNoder(")
	require.Contains(t, output, "func todoNoders(")
	require.Contains(t, output, "func init()")
}

func TestNodeEntityTemplateMultipleEntities(t *testing.T) {
	// Verify the template can be executed for multiple different entities
	// and produces distinct output for each.
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)

	graph, err := entc.LoadGraph("./internal/todo/ent/schema", &gen.Config{
		Storage: s,
	})
	require.NoError(t, err)

	// Find both Todo and Category nodes.
	var todoNode, categoryNode *gen.Type
	for _, n := range graph.Nodes {
		switch n.Name {
		case "Todo":
			todoNode = n
		case "Category":
			categoryNode = n
		}
	}
	require.NotNil(t, todoNode, "Todo node should exist")
	require.NotNil(t, categoryNode, "Category node should exist")

	tmpl := NodeEntityTemplate

	// Generate for Todo.
	var todoBuf bytes.Buffer
	err = tmpl.ExecuteTemplate(&todoBuf, "gql_node_entity", struct {
		*gen.Graph
		Node                  *gen.Type
		HasCollectionTemplate bool
	}{graph, todoNode, true})
	require.NoError(t, err)
	todoOutput := todoBuf.String()

	// Generate for Category.
	var catBuf bytes.Buffer
	err = tmpl.ExecuteTemplate(&catBuf, "gql_node_entity", struct {
		*gen.Graph
		Node                  *gen.Type
		HasCollectionTemplate bool
	}{graph, categoryNode, true})
	require.NoError(t, err)
	catOutput := catBuf.String()

	// Verify Todo output has Todo-specific code.
	require.Contains(t, todoOutput, "todoImplementors")
	require.Contains(t, todoOutput, "todoNoder")
	require.Contains(t, todoOutput, "todoNoders")
	require.Contains(t, todoOutput, "registerNodeResolver(todo.Table")
	require.NotContains(t, todoOutput, "categoryNoder")

	// Verify Category output has Category-specific code.
	require.Contains(t, catOutput, "categoryImplementors")
	require.Contains(t, catOutput, "categoryNoder")
	require.Contains(t, catOutput, "categoryNoders")
	require.Contains(t, catOutput, "registerNodeResolver(category.Table")
	require.NotContains(t, catOutput, "todoNoder")
}
