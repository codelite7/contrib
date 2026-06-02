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

package todosplit

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"entgo.io/contrib/entgqlgo/internal/todosplit/ent"
	"entgo.io/contrib/entgqlgo/internal/todosplit/ent/category"
	"entgo.io/contrib/entgqlgo/internal/todosplit/ent/edges"
	"entgo.io/contrib/entgqlgo/internal/todosplit/ent/enttest"
	"entgo.io/contrib/entgqlgo/internal/todosplit/ent/gqlgo"
	"entgo.io/contrib/entgqlgo/internal/todosplit/ent/todo"

	"github.com/google/uuid"
	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// TodoSplitTestSuite exercises the split-runtime mutation-input code path: the
// generated Create/Update input types apply their change-set to a mutation via
// the generic entbuilder API (SetField/SetEdgeID/AddEdgeIDs/...) rather than
// typed setters.
type TodoSplitTestSuite struct {
	suite.Suite
	client *ent.Client
	schema graphql.Schema
	ctx    context.Context
}

func (s *TodoSplitTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.client = enttest.Open(s.T(), "sqlite3", "file:entsplit?mode=memory&cache=shared&_fk=1")

	var err error
	s.schema, err = gqlgo.NewSchema(s.client)
	s.Require().NoError(err, "failed to create GraphQL schema")
}

func (s *TodoSplitTestSuite) TearDownTest() {
	if s.client != nil {
		s.client.Close()
	}
}

func TestTodoSplitSuite(t *testing.T) {
	suite.Run(t, new(TodoSplitTestSuite))
}

// TestOptionalNillableEnumField is the split-runtime counterpart to bug A2: an
// Optional+Nillable enum (Category.config_type) with UseEnumNames must render as
// the enum type on the object, serialize a typed enum value back through the
// schema, and expose Boolean IsNil/NotNil WhereInput predicates.
func (s *TodoSplitTestSuite) TestOptionalNillableEnumField() {
	introspect := func(typeName, fieldName string) map[string]interface{} {
		res := graphql.Do(graphql.Params{
			Schema: s.schema,
			RequestString: `query($t: String!) {
				__type(name: $t) {
					fields { name type { kind name } }
					inputFields { name type { kind name } }
				}
			}`,
			VariableValues: map[string]interface{}{"t": typeName},
			Context:        s.ctx,
		})
		s.Require().Empty(res.Errors)
		tp := res.Data.(map[string]interface{})["__type"].(map[string]interface{})
		for _, key := range []string{"fields", "inputFields"} {
			if tp[key] == nil {
				continue
			}
			for _, raw := range tp[key].([]interface{}) {
				fm := raw.(map[string]interface{})
				if fm["name"] == fieldName {
					return fm["type"].(map[string]interface{})
				}
			}
		}
		return nil
	}

	objType := introspect("Category", "configType")
	s.Require().NotNil(objType)
	require.Equal(s.T(), "ENUM", objType["kind"])
	require.Equal(s.T(), "CategoryConfigType", objType["name"])

	for _, p := range []string{"configTypeIsNil", "configTypeNotNil"} {
		wt := introspect("CategoryWhereInput", p)
		s.Require().NotNil(wt, "missing predicate %s", p)
		require.Equal(s.T(), "Boolean", wt["name"], "%s must be Boolean", p)
	}

	// Serialize a typed enum value back through the object field.
	cat, err := s.client.Category.Create().
		SetName("Cfg").
		SetConfigType(category.ConfigTypeExternal).
		Save(s.ctx)
	s.Require().NoError(err)

	res := graphql.Do(graphql.Params{
		Schema: s.schema,
		RequestString: `query($id: ID!) {
			node(id: $id) { ... on Category { configType } }
		}`,
		VariableValues: map[string]interface{}{"id": cat.ID},
		Context:        s.ctx,
	})
	s.Require().Empty(res.Errors)
	node := res.Data.(map[string]interface{})["node"].(map[string]interface{})
	require.Equal(s.T(), "External", node["configType"])
}

// TestCreateMutationViaInput verifies that CreateTodoInput.Mutate populates the
// mutation through the generic split-runtime API (SetField/SetEdgeID/AddEdgeIDs)
// and that the resulting entity persists with the expected field and edge state.
func (s *TodoSplitTestSuite) TestCreateMutationViaInput() {
	cat, err := s.client.Category.Create().SetName("Work").Save(s.ctx)
	s.Require().NoError(err)

	priority := 7
	input := &gqlgo.CreateTodoInput{
		Text:       "write split-runtime support",
		Status:     todo.StatusInProgress,
		Priority:   &priority,
		CategoryID: &cat.ID,
	}

	builder := s.client.Todo.Create()
	input.Mutate(builder.Mutation())
	created, err := builder.Save(s.ctx)
	s.Require().NoError(err)

	require.Equal(s.T(), "write split-runtime support", created.Text)
	require.Equal(s.T(), todo.StatusInProgress, created.Status)
	require.Equal(s.T(), 7, created.Priority)

	gotCat, err := edges.QueryTodoCategory(s.client.Todo, created).Only(s.ctx)
	s.Require().NoError(err)
	require.Equal(s.T(), cat.ID, gotCat.ID)
}

// TestUpdateMutationWithClear verifies that UpdateTodoInput.Mutate routes a
// scalar set, an optional-field clear, and an edge clear through ClearField /
// SetField / ClearEdge respectively.
func (s *TodoSplitTestSuite) TestUpdateMutationWithClear() {
	cat, err := s.client.Category.Create().SetName("Work").Save(s.ctx)
	s.Require().NoError(err)

	td, err := s.client.Todo.Create().
		SetText("initial").
		SetStatus(todo.StatusInProgress).
		SetPriority(3).
		SetCategoryID(cat.ID).
		Save(s.ctx)
	s.Require().NoError(err)

	newText := "updated"
	input := &gqlgo.UpdateTodoInput{
		Text:          &newText,
		ClearPriority: true,
		ClearCategory: true,
	}

	builder := s.client.Todo.UpdateOneID(td.ID)
	input.Mutate(builder.Mutation())
	updated, err := builder.Save(s.ctx)
	s.Require().NoError(err)

	require.Equal(s.T(), "updated", updated.Text)
	require.Zero(s.T(), updated.Priority, "priority should be cleared to its zero value")

	_, err = edges.QueryTodoCategory(s.client.Todo, updated).Only(s.ctx)
	require.Error(s.T(), err, "category edge should have been cleared")
}

// TestNonUniqueEdgeMutation verifies the AddEdgeIDs/RemoveEdgeIDs (entbuilder.ToAny)
// path on a non-unique edge via CreateCategoryInput / UpdateCategoryInput.
func (s *TodoSplitTestSuite) TestNonUniqueEdgeMutation() {
	t1, err := s.client.Todo.Create().SetText("a").SetStatus(todo.StatusInProgress).Save(s.ctx)
	s.Require().NoError(err)
	t2, err := s.client.Todo.Create().SetText("b").SetStatus(todo.StatusInProgress).Save(s.ctx)
	s.Require().NoError(err)

	createInput := &gqlgo.CreateCategoryInput{
		Name:    "Bucket",
		TodoIDs: []int{t1.ID, t2.ID},
	}
	cb := s.client.Category.Create()
	createInput.Mutate(cb.Mutation())
	cat, err := cb.Save(s.ctx)
	s.Require().NoError(err)

	todos, err := edges.QueryCategoryTodos(s.client.Category, cat).All(s.ctx)
	s.Require().NoError(err)
	require.Len(s.T(), todos, 2)

	updateInput := &gqlgo.UpdateCategoryInput{
		RemoveTodoIDs: []int{t1.ID},
	}
	ub := s.client.Category.UpdateOneID(cat.ID)
	updateInput.Mutate(ub.Mutation())
	cat, err = ub.Save(s.ctx)
	s.Require().NoError(err)

	todos, err = edges.QueryCategoryTodos(s.client.Category, cat).All(s.ctx)
	s.Require().NoError(err)
	require.Len(s.T(), todos, 1)
	require.Equal(s.T(), t2.ID, todos[0].ID)
}

// TestCreateFriendship exercises the join-entity create path in the split
// runtime: Friendship's only fields are edge-bound, GraphQL-skipped FKs, so the
// create input must expose the two edge ID fields (todoID/categoryID) and apply
// them via the generic SetEdgeID API. Regression for the empty-input bug that
// made graphql.NewSchema reject CreateFriendshipInput.
func (s *TodoSplitTestSuite) TestCreateFriendship() {
	td, err := s.client.Todo.Create().SetText("joined").SetStatus(todo.StatusInProgress).Save(s.ctx)
	s.Require().NoError(err)
	cat, err := s.client.Category.Create().SetName("Joined").Save(s.ctx)
	s.Require().NoError(err)

	result := graphql.Do(graphql.Params{
		Schema:  s.schema,
		Context: s.ctx,
		// PascalCase root mutation field name (CreateFriendship), enabled via
		// WithPascalMutationNames(true) in ent/entc.go. The default camelCase name
		// (createFriendship) would fail to resolve here.
		RequestString: `mutation ($todoID: ID!, $categoryID: ID!) {
			CreateFriendship(input: {todoID: $todoID, categoryID: $categoryID}) {
				id
				todo { id }
				category { id }
			}
		}`,
		VariableValues: map[string]interface{}{
			"todoID":     td.ID,
			"categoryID": cat.ID,
		},
	})
	s.Require().Empty(result.Errors, "GraphQL mutation should not have errors: %v", result.Errors)

	fs, err := s.client.Friendship.Query().All(s.ctx)
	s.Require().NoError(err)
	require.Len(s.T(), fs, 1)

	gotTodo, err := edges.QueryFriendshipTodo(s.client.Friendship, fs[0]).Only(s.ctx)
	s.Require().NoError(err)
	require.Equal(s.T(), td.ID, gotTodo.ID)

	gotCat, err := edges.QueryFriendshipCategory(s.client.Friendship, fs[0]).Only(s.ctx)
	s.Require().NoError(err)
	require.Equal(s.T(), cat.ID, gotCat.ID)
}

// TestRelayQuery exercises the generated Relay connection query end-to-end
// through the GraphQL schema, exercising the split-runtime edge/collection
// support (hoisted Query/With edge functions).
func (s *TodoSplitTestSuite) TestRelayQuery() {
	for _, text := range []string{"one", "two", "three"} {
		input := &gqlgo.CreateTodoInput{Text: text, Status: todo.StatusInProgress}
		b := s.client.Todo.Create()
		input.Mutate(b.Mutation())
		_, err := b.Save(s.ctx)
		s.Require().NoError(err)
	}

	result := graphql.Do(graphql.Params{
		Schema:  s.schema,
		Context: s.ctx,
		RequestString: `
			query {
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
		`,
	})
	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)

	data, ok := result.Data.(map[string]interface{})
	s.Require().True(ok)
	conn, ok := data["todos"].(map[string]interface{})
	s.Require().True(ok)
	connEdges, ok := conn["edges"].([]interface{})
	s.Require().True(ok)
	require.Len(s.T(), connEdges, 3)
}

// TestPascalMutationNames is the end-to-end proof for WithPascalMutationNames(true):
// the full Create/Update/Delete Todo lifecycle resolves through the root Mutation
// using PascalCase field names (CreateTodo/UpdateTodo/DeleteTodo). It also asserts
// the default camelCase names (createTodo) are absent, so the option is observably
// in effect rather than coincidentally matching.
func (s *TodoSplitTestSuite) TestPascalMutationNames() {
	do := func(req string, vars map[string]interface{}) *graphql.Result {
		return graphql.Do(graphql.Params{
			Schema:         s.schema,
			Context:        s.ctx,
			RequestString:  req,
			VariableValues: vars,
		})
	}

	// Create via PascalCase CreateTodo.
	created := do(`mutation {
		CreateTodo(input: {text: "pascal", status: IN_PROGRESS}) { id text status }
	}`, nil)
	s.Require().Empty(created.Errors, "CreateTodo should resolve: %v", created.Errors)
	todoData := created.Data.(map[string]interface{})["CreateTodo"].(map[string]interface{})
	require.Equal(s.T(), "pascal", todoData["text"])
	id := todoData["id"]

	// camelCase createTodo must NOT exist (proves the option flipped the name).
	rejected := do(`mutation {
		createTodo(input: {text: "x", status: IN_PROGRESS}) { id }
	}`, nil)
	s.Require().NotEmpty(rejected.Errors, "default camelCase createTodo must not resolve when PascalCase is enabled")

	// Update via PascalCase UpdateTodo.
	updated := do(`mutation ($id: ID!) {
		UpdateTodo(id: $id, input: {text: "pascal-updated"}) { id text }
	}`, map[string]interface{}{"id": id})
	s.Require().Empty(updated.Errors, "UpdateTodo should resolve: %v", updated.Errors)
	require.Equal(s.T(), "pascal-updated",
		updated.Data.(map[string]interface{})["UpdateTodo"].(map[string]interface{})["text"])

	// Delete via PascalCase DeleteTodo.
	deleted := do(`mutation ($id: ID!) { DeleteTodo(id: $id) }`,
		map[string]interface{}{"id": id})
	s.Require().Empty(deleted.Errors, "DeleteTodo should resolve: %v", deleted.Errors)
	require.Equal(s.T(), true, deleted.Data.(map[string]interface{})["DeleteTodo"])

	count, err := s.client.Todo.Query().Count(s.ctx)
	s.Require().NoError(err)
	require.Zero(s.T(), count, "todo should have been deleted")
}

// TestMutationDecodesAllFieldTypes is the split-runtime counterpart of the
// data-loss regression test: it drives a Float, Time, Strings list, JSON map,
// UUID and Int64 field through the PascalCase Create/Update Todo mutations via
// graphql.Do and asserts every value round-trips into the database. The
// split-runtime mutation applies these via the generic entbuilder SetField API,
// but the input parsers (where the bug lived) are shared with the classic path.
func (s *TodoSplitTestSuite) TestMutationDecodesAllFieldTypes() {
	extID := uuid.New()
	due := time.Date(1990, time.June, 15, 12, 0, 0, 0, time.UTC)

	create := graphql.Do(graphql.Params{
		Schema:  s.schema,
		Context: s.ctx,
		// PascalCase CreateTodo (WithPascalMutationNames). metadata/externalID use
		// the String/ID scalar fallbacks (no Map/UUID scalar registered).
		RequestString: `mutation ($due: Time!, $ext: ID!, $meta: String!) {
			CreateTodo(input: {
				text: "all-types"
				status: IN_PROGRESS
				score: 4.5
				dueDate: $due
				tags2: ["a", "b", "c"]
				metadata: $meta
				externalID: $ext
				duration: 90000
			}) { id }
		}`,
		VariableValues: map[string]interface{}{
			"due":  due.Format(time.RFC3339),
			"ext":  extID.String(),
			"meta": `{"k":"v","n":2}`,
		},
	})
	s.Require().Empty(create.Errors, "create errors: %v", create.Errors)

	got, err := s.client.Todo.Query().Where(todo.TextEQ("all-types")).Only(s.ctx)
	s.Require().NoError(err)
	require.Equal(s.T(), 4.5, got.Score)
	require.WithinDuration(s.T(), due, got.DueDate, 0)
	require.Equal(s.T(), []string{"a", "b", "c"}, got.Tags2)
	require.Equal(s.T(), map[string]interface{}{"k": "v", "n": float64(2)}, got.Metadata)
	require.Equal(s.T(), extID, got.ExternalID)
	require.Equal(s.T(), int64(90000), got.Duration)

	newExt := uuid.New()
	newDue := time.Date(2001, time.January, 2, 3, 4, 5, 0, time.UTC)
	update := graphql.Do(graphql.Params{
		Schema:  s.schema,
		Context: s.ctx,
		RequestString: fmt.Sprintf(`mutation ($due: Time!, $ext: ID!, $meta: String!) {
			UpdateTodo(id: "%d", input: {
				score: 8.25
				dueDate: $due
				tags2: ["x", "y"]
				metadata: $meta
				externalID: $ext
				duration: 120000
			}) { id }
		}`, got.ID),
		VariableValues: map[string]interface{}{
			"due":  newDue.Format(time.RFC3339),
			"ext":  newExt.String(),
			"meta": `{"updated":true}`,
		},
	})
	s.Require().Empty(update.Errors, "update errors: %v", update.Errors)

	got2, err := s.client.Todo.Get(s.ctx, got.ID)
	s.Require().NoError(err)
	require.Equal(s.T(), 8.25, got2.Score)
	require.WithinDuration(s.T(), newDue, got2.DueDate, 0)
	require.Equal(s.T(), []string{"x", "y"}, got2.Tags2)
	require.Equal(s.T(), map[string]interface{}{"updated": true}, got2.Metadata)
	require.Equal(s.T(), newExt, got2.ExternalID)
	require.Equal(s.T(), int64(120000), got2.Duration)
}

// TestFilterDecodesAllFieldTypes is the split-runtime counterpart of the
// where-input data-integrity regression: it asserts ParseTodoWhereInput decodes
// non-primitive predicates (UUID, time, float) into correctly-typed ent
// predicates instead of silently dropping them (which previously made
// `where: {externalID: $uuid}` return every row). The where-input parsers are
// shared with the classic path, but this proves the fix holds under the split
// runtime layout too.
func (s *TodoSplitTestSuite) TestFilterDecodesAllFieldTypes() {
	extA, extB, extC := uuid.New(), uuid.New(), uuid.New()
	dueA := time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC)
	dueB := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	dueC := time.Date(2010, time.January, 1, 0, 0, 0, 0, time.UTC)

	mk := func(text string, ext uuid.UUID, due time.Time, score float64) {
		s.client.Todo.Create().
			SetText(text).
			SetStatus(todo.StatusInProgress).
			SetExternalID(ext).
			SetDueDate(due).
			SetScore(score).
			SaveX(s.ctx)
	}
	mk("A", extA, dueA, 1.5)
	mk("B", extB, dueB, 5.5)
	mk("C", extC, dueC, 9.5)

	texts := func(query string, vars map[string]interface{}) []string {
		res := graphql.Do(graphql.Params{
			Schema:         s.schema,
			Context:        s.ctx,
			RequestString:  query,
			VariableValues: vars,
		})
		s.Require().Empty(res.Errors, "where query errors: %v", res.Errors)
		conn := res.Data.(map[string]interface{})["todos"].(map[string]interface{})
		var out []string
		for _, e := range conn["edges"].([]interface{}) {
			node := e.(map[string]interface{})["node"].(map[string]interface{})
			out = append(out, node["text"].(string))
		}
		sort.Strings(out)
		return out
	}

	// Filter by UUID id: exactly one match.
	require.Equal(s.T(), []string{"A"}, texts(
		`query ($ext: ID!) { todos(where: {externalID: $ext}) { edges { node { text } } } }`,
		map[string]interface{}{"ext": extA.String()}))
	// Filter by UUID In: two matches.
	require.Equal(s.T(), []string{"A", "C"}, texts(
		`query ($a: ID!, $c: ID!) { todos(where: {externalIDIn: [$a, $c]}) { edges { node { text } } } }`,
		map[string]interface{}{"a": extA.String(), "c": extC.String()}))
	// Filter by time range: 1995 < due < 2005 -> only B.
	require.Equal(s.T(), []string{"B"}, texts(
		`query ($lo: Time!, $hi: Time!) { todos(where: {dueDateGT: $lo, dueDateLT: $hi}) { edges { node { text } } } }`,
		map[string]interface{}{
			"lo": time.Date(1995, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"hi": time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		}))
	// Filter by float: score > 4.0 -> B and C.
	require.Equal(s.T(), []string{"B", "C"}, texts(
		`query { todos(where: {scoreGT: 4.0}) { edges { node { text } } } }`, nil))
}
