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
	"testing"

	"entgo.io/contrib/entgqlgo/internal/todosplit/ent"
	"entgo.io/contrib/entgqlgo/internal/todosplit/ent/edges"
	"entgo.io/contrib/entgqlgo/internal/todosplit/ent/enttest"
	"entgo.io/contrib/entgqlgo/internal/todosplit/ent/gqlgo"
	"entgo.io/contrib/entgqlgo/internal/todosplit/ent/todo"

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
		RequestString: `mutation ($todoID: ID!, $categoryID: ID!) {
			createFriendship(input: {todoID: $todoID, categoryID: $categoryID}) {
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
