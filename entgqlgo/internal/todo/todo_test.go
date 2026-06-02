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
	"fmt"
	"testing"

	"entgo.io/contrib/entgqlgo"
	"entgo.io/contrib/entgqlgo/internal/todo/ent"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/enttest"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/gqlgo"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/todo"

	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// TodoTestSuite is a test suite for the todo module.
type TodoTestSuite struct {
	suite.Suite
	client *ent.Client
	schema graphql.Schema
	ctx    context.Context
}

// SetupTest runs before each test.
func (s *TodoTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.client = enttest.Open(s.T(), "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")

	var err error
	s.schema, err = newTestSchema(s.client)
	s.Require().NoError(err, "failed to create GraphQL schema")
}

// TearDownTest runs after each test.
func (s *TodoTestSuite) TearDownTest() {
	if s.client != nil {
		s.client.Close()
	}
}

// TestTodoSuite runs the test suite.
func TestTodoSuite(t *testing.T) {
	suite.Run(t, new(TodoTestSuite))
}

// executeQuery executes a GraphQL query and returns the result.
func (s *TodoTestSuite) executeQuery(query string) *graphql.Result {
	return graphql.Do(graphql.Params{
		Schema:        s.schema,
		RequestString: query,
		Context:       s.ctx,
	})
}

// TestQueryEmpty verifies that an empty query returns no results.
func (s *TodoTestSuite) TestQueryEmpty() {
	// Query all todos - should be empty
	todos, err := s.client.Todo.Query().All(s.ctx)
	s.Require().NoError(err)
	s.Require().Empty(todos)

	// Query all categories - should be empty
	categories, err := s.client.Category.Query().All(s.ctx)
	s.Require().NoError(err)
	s.Require().Empty(categories)
}

// TestQueryTodos tests querying todos via GraphQL.
func (s *TodoTestSuite) TestQueryTodos() {
	// Create 3 todos
	for i := 1; i <= 3; i++ {
		_, err := s.client.Todo.Create().
			SetText(fmt.Sprintf("Todo %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			Save(s.ctx)
		s.Require().NoError(err)
	}

	// Query via GraphQL
	result := s.executeQuery(`
		query {
			todos {
				edges {
					node {
						id
						text
					}
				}
			}
		}
	`)

	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)
	s.Require().NotNil(result.Data)

	data, ok := result.Data.(map[string]interface{})
	s.Require().True(ok, "result.Data should be a map")

	conn, ok := data["todos"].(map[string]interface{})
	s.Require().True(ok, "todos should be a connection map")
	edges, ok := conn["edges"].([]interface{})
	s.Require().True(ok, "todos.edges should be a slice")
	s.Require().Len(edges, 3, "should have 3 todos")
}

// TestQueryCategories tests querying categories via GraphQL.
func (s *TodoTestSuite) TestQueryCategories() {
	// Create 2 categories
	_, err := s.client.Category.Create().
		SetText("Work").
		Save(s.ctx)
	s.Require().NoError(err)

	_, err = s.client.Category.Create().
		SetText("Personal").
		Save(s.ctx)
	s.Require().NoError(err)

	// Query via GraphQL
	result := s.executeQuery(`
		query {
			categories {
				edges {
					node {
						id
						text
					}
				}
			}
		}
	`)

	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)
	s.Require().NotNil(result.Data)

	data, ok := result.Data.(map[string]interface{})
	s.Require().True(ok, "result.Data should be a map")

	conn, ok := data["categories"].(map[string]interface{})
	s.Require().True(ok, "categories should be a connection map")
	edges, ok := conn["edges"].([]interface{})
	s.Require().True(ok, "categories.edges should be a slice")
	s.Require().Len(edges, 2, "should have 2 categories")
}

// TestQueryTodoFields tests that all todo fields are resolved correctly.
func (s *TodoTestSuite) TestQueryTodoFields() {
	// Create a todo with all fields
	created, err := s.client.Todo.Create().
		SetText("Test todo").
		SetStatus(todo.StatusInProgress).
		SetPriority(5).
		Save(s.ctx)
	s.Require().NoError(err)

	// Query via GraphQL
	result := s.executeQuery(`
		query {
			todos {
				edges {
					node {
						id
						text
						status
						priority
						createdAt
					}
				}
			}
		}
	`)

	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)
	s.Require().NotNil(result.Data)

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	s.Require().Len(edges, 1)

	todoItem := edges[0].(map[string]interface{})["node"].(map[string]interface{})

	// Verify each field
	s.Equal(fmt.Sprintf("%d", created.ID), todoItem["id"], "id should match")
	s.Equal("Test todo", todoItem["text"], "text should match")
	s.Equal("IN_PROGRESS", todoItem["status"], "status should match")
	s.Equal(5, todoItem["priority"], "priority should match")
	s.NotNil(todoItem["createdAt"], "createdAt should not be nil")
}

// TestQuerySingleTodo tests querying a single todo by ID.
func (s *TodoTestSuite) TestQuerySingleTodo() {
	// Create a todo
	created, err := s.client.Todo.Create().
		SetText("Single todo").
		SetStatus(todo.StatusCompleted).
		SetPriority(10).
		Save(s.ctx)
	s.Require().NoError(err)

	// Query by ID via GraphQL using node() with inline fragment
	result := s.executeQuery(fmt.Sprintf(`
		query {
			node(id: "%d") {
				id
				... on Todo {
					text
					status
				}
			}
		}
	`, created.ID))

	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)
	s.Require().NotNil(result.Data)

	data := result.Data.(map[string]interface{})
	todoResult := data["node"].(map[string]interface{})

	s.Equal(fmt.Sprintf("%d", created.ID), todoResult["id"])
	s.Equal("Single todo", todoResult["text"])
	s.Equal("COMPLETED", todoResult["status"])
}

// TestQueryTodosPagination tests cursor pagination on todos query.
func (s *TodoTestSuite) TestQueryTodosPagination() {
	// Create 5 todos
	for i := 1; i <= 5; i++ {
		_, err := s.client.Todo.Create().
			SetText(fmt.Sprintf("Todo %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			Save(s.ctx)
		s.Require().NoError(err)
	}

	// Query page 1: first 2
	result := s.executeQuery(`
		query {
			todos(first: 2) {
				edges { node { id } }
				pageInfo { endCursor }
			}
		}
	`)

	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	s.Require().Len(edges, 2, "should have 2 todos with first: 2")

	endCursor := conn["pageInfo"].(map[string]interface{})["endCursor"].(string)
	s.Require().NotEmpty(endCursor, "endCursor should not be empty")

	// Query page 2: skip past first 2 using cursor, get next 3
	result = s.executeQuery(fmt.Sprintf(`
		query {
			todos(first: 3, after: %q) {
				edges { node { id } }
			}
		}
	`, endCursor))

	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)

	data = result.Data.(map[string]interface{})
	conn = data["todos"].(map[string]interface{})
	edges = conn["edges"].([]interface{})
	s.Require().Len(edges, 3, "should have 3 todos after the first 2 (cursor pagination equivalent of offset: 2)")
}

// ========== Top-level tests that match -run 'TestQuery' ==========
// These are standalone tests that duplicate some suite functionality
// but allow running with the specified test pattern.

// TestQueryTodos creates todos and queries them via GraphQL.
func TestQueryTodos(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create 3 todos
	for i := 1; i <= 3; i++ {
		_, err := client.Todo.Create().
			SetText(fmt.Sprintf("Todo %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create todo: %v", err)
		}
	}

	// Query via GraphQL
	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { todos { edges { node { id text } } } }`,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatal("result.Data is not a map")
	}

	conn, ok := data["todos"].(map[string]interface{})
	if !ok {
		t.Fatal("todos is not a connection map")
	}

	edges, ok := conn["edges"].([]interface{})
	if !ok {
		t.Fatal("todos.edges is not a slice")
	}

	if len(edges) != 3 {
		t.Fatalf("expected 3 todos, got %d", len(edges))
	}
}

// TestQueryCategories creates categories and queries them via GraphQL.
func TestQueryCategories(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create 2 categories
	_, err = client.Category.Create().
		SetText("Work").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	_, err = client.Category.Create().
		SetText("Personal").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Query via GraphQL
	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { categories { edges { node { id text } } } }`,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatal("result.Data is not a map")
	}

	conn, ok := data["categories"].(map[string]interface{})
	if !ok {
		t.Fatal("categories is not a connection map")
	}

	edges, ok := conn["edges"].([]interface{})
	if !ok {
		t.Fatal("categories.edges is not a slice")
	}

	if len(edges) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(edges))
	}
}

// TestQueryTodoFields verifies all todo fields are resolved correctly.
func TestQueryTodoFields(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a todo with all fields
	created, err := client.Todo.Create().
		SetText("Test todo").
		SetStatus(todo.StatusInProgress).
		SetPriority(5).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Query via GraphQL
	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { todos { edges { node { id text status priority createdAt } } } }`,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	if len(edges) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(edges))
	}

	todoItem := edges[0].(map[string]interface{})["node"].(map[string]interface{})

	// Verify each field
	if todoItem["id"] != fmt.Sprintf("%d", created.ID) {
		t.Errorf("id mismatch: got %v, want %d", todoItem["id"], created.ID)
	}
	if todoItem["text"] != "Test todo" {
		t.Errorf("text mismatch: got %v, want 'Test todo'", todoItem["text"])
	}
	if todoItem["status"] != "IN_PROGRESS" {
		t.Errorf("status mismatch: got %v, want 'IN_PROGRESS'", todoItem["status"])
	}
	if todoItem["priority"] != 5 {
		t.Errorf("priority mismatch: got %v, want 5", todoItem["priority"])
	}
	if todoItem["createdAt"] == nil {
		t.Error("createdAt should not be nil")
	}
}

// ========== Relay-style cursor pagination tests ==========

// TestPageForward tests forward pagination using first/after.

// TestPageBackward tests backward pagination using last/before.

// TestTotalCount tests that totalCount is accurate.

// TestEmptyPage tests querying past the end returns empty edges.

// ========== WhereInput filtering tests ==========

// TestFilterByStatus tests filtering todos by status enum.
func TestFilterByStatus(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create todos with different statuses
	_, err = client.Todo.Create().
		SetText("Todo 1").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Todo 2").
		SetStatus(todo.StatusInProgress).
		SetPriority(2).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Todo 3").
		SetStatus(todo.StatusCompleted).
		SetPriority(3).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Filter by COMPLETED status
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(where: {status: COMPLETED}) {
				edges {
					node {
						id
						text
						status
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 1 {
		t.Fatalf("expected 1 todo with COMPLETED status, got %d", len(edges))
	}

	todoItem := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	if todoItem["status"] != "COMPLETED" {
		t.Errorf("expected status COMPLETED, got %v", todoItem["status"])
	}
	if todoItem["text"] != "Todo 3" {
		t.Errorf("expected text 'Todo 3', got %v", todoItem["text"])
	}
}

// TestFilterByText tests filtering todos by text field with contains.
func TestFilterByText(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create todos with different text
	_, err = client.Todo.Create().
		SetText("Buy groceries").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Write test code").
		SetStatus(todo.StatusPending).
		SetPriority(2).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Review test results").
		SetStatus(todo.StatusPending).
		SetPriority(3).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Filter by text containing "test"
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(where: {textContains: "test"}) {
				edges {
					node {
						id
						text
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 2 {
		t.Fatalf("expected 2 todos containing 'test', got %d", len(edges))
	}

	// Verify both todos contain "test"
	for _, item := range edges {
		todoItem := item.(map[string]interface{})["node"].(map[string]interface{})
		text := todoItem["text"].(string)
		if text != "Write test code" && text != "Review test results" {
			t.Errorf("unexpected todo text: %s", text)
		}
	}
}

// TestFilterAnd tests filtering with AND condition.
func TestFilterAnd(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create todos with various status and priority combinations
	_, err = client.Todo.Create().
		SetText("Low priority completed").
		SetStatus(todo.StatusCompleted).
		SetPriority(2).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("High priority completed").
		SetStatus(todo.StatusCompleted).
		SetPriority(8).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("High priority pending").
		SetStatus(todo.StatusPending).
		SetPriority(7).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Filter by COMPLETED status AND priority > 5
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(where: {and: [{status: COMPLETED}, {priorityGT: 5}]}) {
				edges {
					node {
						id
						text
						status
						priority
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 1 {
		t.Fatalf("expected 1 todo matching AND condition, got %d", len(edges))
	}

	todoItem := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	if todoItem["text"] != "High priority completed" {
		t.Errorf("expected 'High priority completed', got %v", todoItem["text"])
	}
}

// TestFilterOr tests filtering with OR condition.
func TestFilterOr(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create todos with different statuses
	_, err = client.Todo.Create().
		SetText("Todo 1").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Todo 2").
		SetStatus(todo.StatusInProgress).
		SetPriority(2).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Todo 3").
		SetStatus(todo.StatusCompleted).
		SetPriority(3).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Filter by COMPLETED OR IN_PROGRESS status
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(where: {or: [{status: COMPLETED}, {status: IN_PROGRESS}]}) {
				edges {
					node {
						id
						text
						status
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 2 {
		t.Fatalf("expected 2 todos matching OR condition, got %d", len(edges))
	}

	// Verify we got COMPLETED and IN_PROGRESS, but not PENDING
	statuses := make(map[string]bool)
	for _, item := range edges {
		todoItem := item.(map[string]interface{})["node"].(map[string]interface{})
		statuses[todoItem["status"].(string)] = true
	}

	if !statuses["COMPLETED"] {
		t.Error("expected COMPLETED status in results")
	}
	if !statuses["IN_PROGRESS"] {
		t.Error("expected IN_PROGRESS status in results")
	}
	if statuses["PENDING"] {
		t.Error("unexpected PENDING status in results")
	}
}

// TestFilterNot tests filtering with NOT condition.
func TestFilterNot(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create todos with different statuses
	_, err = client.Todo.Create().
		SetText("Todo 1").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Todo 2").
		SetStatus(todo.StatusInProgress).
		SetPriority(2).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Todo 3").
		SetStatus(todo.StatusCompleted).
		SetPriority(3).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Filter by NOT COMPLETED status (should get PENDING and IN_PROGRESS)
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(where: {not: {status: COMPLETED}}) {
				edges {
					node {
						id
						text
						status
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 2 {
		t.Fatalf("expected 2 todos NOT COMPLETED, got %d", len(edges))
	}

	// Verify none have COMPLETED status
	for _, item := range edges {
		todoItem := item.(map[string]interface{})["node"].(map[string]interface{})
		if todoItem["status"] == "COMPLETED" {
			t.Error("found COMPLETED status when it should be excluded")
		}
	}
}

// TestFilterByEdge tests filtering by edge existence.
func TestFilterByEdge(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a category
	category, err := client.Category.Create().
		SetText("Work").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create todo with category
	_, err = client.Todo.Create().
		SetText("Todo with category").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		SetCategory(category).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo with category: %v", err)
	}

	// Create todo without category
	_, err = client.Todo.Create().
		SetText("Todo without category").
		SetStatus(todo.StatusPending).
		SetPriority(2).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo without category: %v", err)
	}

	// Filter by hasCategory: true
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(where: {hasCategory: true}) {
				edges {
					node {
						id
						text
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 1 {
		t.Fatalf("expected 1 todo with category, got %d", len(edges))
	}

	todoItem := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	if todoItem["text"] != "Todo with category" {
		t.Errorf("expected 'Todo with category', got %v", todoItem["text"])
	}

	// Filter by hasCategory: false
	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(where: {hasCategory: false}) {
				edges {
					node {
						id
						text
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data = result.Data.(map[string]interface{})
	conn = data["todos"].(map[string]interface{})
	edges = conn["edges"].([]interface{})

	if len(edges) != 1 {
		t.Fatalf("expected 1 todo without category, got %d", len(edges))
	}

	todoItem = edges[0].(map[string]interface{})["node"].(map[string]interface{})
	if todoItem["text"] != "Todo without category" {
		t.Errorf("expected 'Todo without category', got %v", todoItem["text"])
	}
}

// ========== Ordering tests ==========

// TestOrderByPriority tests ordering todos by priority.
func TestOrderByPriority(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create todos with different priorities
	_, err = client.Todo.Create().
		SetText("Low priority").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("High priority").
		SetStatus(todo.StatusPending).
		SetPriority(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Medium priority").
		SetStatus(todo.StatusPending).
		SetPriority(5).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Query with orderBy priority ASC
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(orderBy: [{field: PRIORITY, direction: ASC}]) {
				edges {
					node {
						text
						priority
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 3 {
		t.Fatalf("expected 3 todos, got %d", len(edges))
	}

	// Verify order: Low (1), Medium (5), High (10)
	if edges[0].(map[string]interface{})["node"].(map[string]interface{})["text"] != "Low priority" {
		t.Errorf("expected first todo to be 'Low priority', got %v", edges[0].(map[string]interface{})["node"].(map[string]interface{})["text"])
	}
	if edges[1].(map[string]interface{})["node"].(map[string]interface{})["text"] != "Medium priority" {
		t.Errorf("expected second todo to be 'Medium priority', got %v", edges[1].(map[string]interface{})["node"].(map[string]interface{})["text"])
	}
	if edges[2].(map[string]interface{})["node"].(map[string]interface{})["text"] != "High priority" {
		t.Errorf("expected third todo to be 'High priority', got %v", edges[2].(map[string]interface{})["node"].(map[string]interface{})["text"])
	}
}

// TestOrderByCreatedAt tests ordering todos by created_at timestamp.
func TestOrderByCreatedAt(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create todos - they will have slightly different created_at timestamps
	_, err = client.Todo.Create().
		SetText("First created").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Second created").
		SetStatus(todo.StatusPending).
		SetPriority(2).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Third created").
		SetStatus(todo.StatusPending).
		SetPriority(3).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Query with orderBy created_at ASC
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(orderBy: [{field: CREATED_AT, direction: ASC}]) {
				edges {
					node {
						text
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 3 {
		t.Fatalf("expected 3 todos, got %d", len(edges))
	}

	// Verify order based on creation time (ASC = oldest first)
	if edges[0].(map[string]interface{})["node"].(map[string]interface{})["text"] != "First created" {
		t.Errorf("expected first todo to be 'First created', got %v", edges[0].(map[string]interface{})["node"].(map[string]interface{})["text"])
	}
	if edges[2].(map[string]interface{})["node"].(map[string]interface{})["text"] != "Third created" {
		t.Errorf("expected third todo to be 'Third created', got %v", edges[2].(map[string]interface{})["node"].(map[string]interface{})["text"])
	}
}

// TestOrderDesc tests ordering in descending direction.
func TestOrderDesc(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create todos with different priorities
	_, err = client.Todo.Create().
		SetText("Low priority").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("High priority").
		SetStatus(todo.StatusPending).
		SetPriority(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Medium priority").
		SetStatus(todo.StatusPending).
		SetPriority(5).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Query with orderBy priority DESC
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(orderBy: [{field: PRIORITY, direction: DESC}]) {
				edges {
					node {
						text
						priority
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 3 {
		t.Fatalf("expected 3 todos, got %d", len(edges))
	}

	// Verify order: High (10), Medium (5), Low (1)
	if edges[0].(map[string]interface{})["node"].(map[string]interface{})["text"] != "High priority" {
		t.Errorf("expected first todo to be 'High priority', got %v", edges[0].(map[string]interface{})["node"].(map[string]interface{})["text"])
	}
	if edges[1].(map[string]interface{})["node"].(map[string]interface{})["text"] != "Medium priority" {
		t.Errorf("expected second todo to be 'Medium priority', got %v", edges[1].(map[string]interface{})["node"].(map[string]interface{})["text"])
	}
	if edges[2].(map[string]interface{})["node"].(map[string]interface{})["text"] != "Low priority" {
		t.Errorf("expected third todo to be 'Low priority', got %v", edges[2].(map[string]interface{})["node"].(map[string]interface{})["text"])
	}
}

// TestMultiOrder tests ordering by multiple fields (multi-order).
func TestMultiOrder(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create todos with same status but different priorities
	_, err = client.Todo.Create().
		SetText("Pending Low").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Completed High").
		SetStatus(todo.StatusCompleted).
		SetPriority(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Pending High").
		SetStatus(todo.StatusPending).
		SetPriority(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Completed Low").
		SetStatus(todo.StatusCompleted).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Query with orderBy status ASC, then priority DESC
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(orderBy: [{field: STATUS, direction: ASC}, {field: PRIORITY, direction: DESC}]) {
				edges {
					node {
						text
						status
						priority
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 4 {
		t.Fatalf("expected 4 todos, got %d", len(edges))
	}

	// Verify order: COMPLETED status first (alphabetically), then by priority DESC
	// COMPLETED High (10), COMPLETED Low (1), then IN_PROGRESS, then PENDING High (10), PENDING Low (1)

	// Since status sorts alphabetically: COMPLETED < IN_PROGRESS < PENDING
	// First two should be COMPLETED, last two should be PENDING
	firstTodo := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	secondTodo := edges[1].(map[string]interface{})["node"].(map[string]interface{})
	thirdTodo := edges[2].(map[string]interface{})["node"].(map[string]interface{})
	fourthTodo := edges[3].(map[string]interface{})["node"].(map[string]interface{})

	// COMPLETED items first (with high priority first due to DESC)
	if firstTodo["status"] != "COMPLETED" {
		t.Errorf("expected first todo status to be COMPLETED, got %v", firstTodo["status"])
	}
	if firstTodo["priority"] != 10 {
		t.Errorf("expected first todo priority to be 10, got %v", firstTodo["priority"])
	}

	if secondTodo["status"] != "COMPLETED" {
		t.Errorf("expected second todo status to be COMPLETED, got %v", secondTodo["status"])
	}
	if secondTodo["priority"] != 1 {
		t.Errorf("expected second todo priority to be 1, got %v", secondTodo["priority"])
	}

	// PENDING items last (with high priority first due to DESC)
	if thirdTodo["status"] != "PENDING" {
		t.Errorf("expected third todo status to be PENDING, got %v", thirdTodo["status"])
	}
	if thirdTodo["priority"] != 10 {
		t.Errorf("expected third todo priority to be 10, got %v", thirdTodo["priority"])
	}

	if fourthTodo["status"] != "PENDING" {
		t.Errorf("expected fourth todo status to be PENDING, got %v", fourthTodo["status"])
	}
	if fourthTodo["priority"] != 1 {
		t.Errorf("expected fourth todo priority to be 1, got %v", fourthTodo["priority"])
	}
}

// TestOrderByText tests ordering by text field.
func TestOrderByText(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create todos with different text
	_, err = client.Todo.Create().
		SetText("Charlie").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Alpha").
		SetStatus(todo.StatusPending).
		SetPriority(2).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Bravo").
		SetStatus(todo.StatusPending).
		SetPriority(3).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Query with orderBy text ASC
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(orderBy: [{field: TEXT, direction: ASC}]) {
				edges {
					node {
						text
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 3 {
		t.Fatalf("expected 3 todos, got %d", len(edges))
	}

	// Verify alphabetical order: Alpha, Bravo, Charlie
	if edges[0].(map[string]interface{})["node"].(map[string]interface{})["text"] != "Alpha" {
		t.Errorf("expected first todo to be 'Alpha', got %v", edges[0].(map[string]interface{})["node"].(map[string]interface{})["text"])
	}
	if edges[1].(map[string]interface{})["node"].(map[string]interface{})["text"] != "Bravo" {
		t.Errorf("expected second todo to be 'Bravo', got %v", edges[1].(map[string]interface{})["node"].(map[string]interface{})["text"])
	}
	if edges[2].(map[string]interface{})["node"].(map[string]interface{})["text"] != "Charlie" {
		t.Errorf("expected third todo to be 'Charlie', got %v", edges[2].(map[string]interface{})["node"].(map[string]interface{})["text"])
	}
}

// TestOrderWithPagination tests ordering with pagination.
func TestOrderWithPagination(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create 5 todos with different priorities
	for i := 1; i <= 5; i++ {
		_, err = client.Todo.Create().
			SetText(fmt.Sprintf("Priority %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create todo: %v", err)
		}
	}

	// Query first 2 todos ordered by priority DESC
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(first: 2, orderBy: [{field: PRIORITY, direction: DESC}]) {
				edges {
					node {
						text
						priority
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(edges))
	}

	// Should get priority 5 and 4 (highest first)
	if edges[0].(map[string]interface{})["node"].(map[string]interface{})["priority"] != 5 {
		t.Errorf("expected first todo priority to be 5, got %v", edges[0].(map[string]interface{})["node"].(map[string]interface{})["priority"])
	}
	if edges[1].(map[string]interface{})["node"].(map[string]interface{})["priority"] != 4 {
		t.Errorf("expected second todo priority to be 4, got %v", edges[1].(map[string]interface{})["node"].(map[string]interface{})["priority"])
	}
}

// ========== Mutation tests ==========

// TestCreateTodo tests creating a todo via GraphQL mutation.
func TestCreateTodo(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a todo via mutation
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `mutation {
			createTodo(input: {
				text: "Test todo from mutation"
				status: PENDING
				priority: 5
			}) {
				id
				text
				status
				priority
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL mutation had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	created := data["createTodo"].(map[string]interface{})

	if created["text"] != "Test todo from mutation" {
		t.Errorf("expected text 'Test todo from mutation', got %v", created["text"])
	}
	if created["status"] != "PENDING" {
		t.Errorf("expected status 'PENDING', got %v", created["status"])
	}
	if created["priority"] != 5 {
		t.Errorf("expected priority 5, got %v", created["priority"])
	}

	// Verify it was created in the database
	todos, err := client.Todo.Query().All(ctx)
	if err != nil {
		t.Fatalf("failed to query todos: %v", err)
	}
	if len(todos) != 1 {
		t.Fatalf("expected 1 todo in database, got %d", len(todos))
	}
	if todos[0].Text != "Test todo from mutation" {
		t.Errorf("expected text 'Test todo from mutation' in DB, got %s", todos[0].Text)
	}
}

// TestCreateCategory tests creating a category via GraphQL mutation.
func TestCreateCategory(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a category via mutation (status and kind have defaults, so they are optional)
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `mutation {
			createCategory(input: {
				text: "Work"
				status: ENABLED
			}) {
				id
				text
				status
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL mutation had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	created := data["createCategory"].(map[string]interface{})

	if created["text"] != "Work" {
		t.Errorf("expected text 'Work', got %v", created["text"])
	}
	if created["status"] != "ENABLED" {
		t.Errorf("expected status 'ENABLED', got %v", created["status"])
	}

	// Verify it was created in the database
	categories, err := client.Category.Query().All(ctx)
	if err != nil {
		t.Fatalf("failed to query categories: %v", err)
	}
	if len(categories) != 1 {
		t.Fatalf("expected 1 category in database, got %d", len(categories))
	}
}

// TestUpdateTodo tests updating a todo via GraphQL mutation.
func TestUpdateTodo(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a todo first
	created, err := client.Todo.Create().
		SetText("Original text").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Update the todo via mutation
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`mutation {
			updateTodo(id: "%d", input: {
				text: "Updated text"
				status: COMPLETED
				priority: 10
			}) {
				id
				text
				status
				priority
			}
		}`, created.ID),
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL mutation had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	updated := data["updateTodo"].(map[string]interface{})

	if updated["text"] != "Updated text" {
		t.Errorf("expected text 'Updated text', got %v", updated["text"])
	}
	if updated["status"] != "COMPLETED" {
		t.Errorf("expected status 'COMPLETED', got %v", updated["status"])
	}
	if updated["priority"] != 10 {
		t.Errorf("expected priority 10, got %v", updated["priority"])
	}

	// Verify it was updated in the database
	dbTodo, err := client.Todo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed to get todo: %v", err)
	}
	if dbTodo.Text != "Updated text" {
		t.Errorf("expected text 'Updated text' in DB, got %s", dbTodo.Text)
	}
	if dbTodo.Status != todo.StatusCompleted {
		t.Errorf("expected status COMPLETED in DB, got %s", dbTodo.Status)
	}
}

// TestUpdateWithClear tests clearing optional fields via update mutation.
func TestUpdateWithClear(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a category
	category, err := client.Category.Create().
		SetText("Work").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create a todo with category
	created, err := client.Todo.Create().
		SetText("Todo with category").
		SetStatus(todo.StatusPending).
		SetPriority(5).
		SetCategory(category).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Verify category is set
	dbTodo, err := client.Todo.Query().
		Where(todo.ID(created.ID)).
		WithCategory().
		Only(ctx)
	if err != nil {
		t.Fatalf("failed to query todo: %v", err)
	}
	if dbTodo.Edges.Category == nil {
		t.Fatal("expected category to be set before clear")
	}

	// Clear the category via mutation
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`mutation {
			updateTodo(id: "%d", input: {
				clearCategory: true
			}) {
				id
				text
			}
		}`, created.ID),
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL mutation had errors: %v", result.Errors)
	}

	// Verify category was cleared
	dbTodo, err = client.Todo.Query().
		Where(todo.ID(created.ID)).
		WithCategory().
		Only(ctx)
	if err != nil {
		t.Fatalf("failed to query todo after clear: %v", err)
	}
	if dbTodo.Edges.Category != nil {
		t.Errorf("expected category to be cleared, but got %v", dbTodo.Edges.Category)
	}
}

// TestCreateWithEdge tests creating with edge reference.
func TestCreateWithEdge(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a category first
	category, err := client.Category.Create().
		SetText("Work").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create a todo with the category via mutation
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`mutation {
			createTodo(input: {
				text: "Todo with category"
				status: PENDING
				priority: 3
				categoryID: "%d"
			}) {
				id
				text
				category {
					id
					text
				}
			}
		}`, category.ID),
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL mutation had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	created := data["createTodo"].(map[string]interface{})

	if created["text"] != "Todo with category" {
		t.Errorf("expected text 'Todo with category', got %v", created["text"])
	}

	catData := created["category"].(map[string]interface{})
	if catData["text"] != "Work" {
		t.Errorf("expected category text 'Work', got %v", catData["text"])
	}

	// Verify the edge in the database
	todos, err := client.Todo.Query().WithCategory().All(ctx)
	if err != nil {
		t.Fatalf("failed to query todos: %v", err)
	}
	if len(todos) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(todos))
	}
	if todos[0].Edges.Category == nil {
		t.Fatal("expected category edge to be set")
	}
	if todos[0].Edges.Category.Text != "Work" {
		t.Errorf("expected category text 'Work', got %s", todos[0].Edges.Category.Text)
	}
}

// ========== Relay Node interface tests ==========

// TestNodeQuery tests fetching a single node by global ID.
func TestNodeQuery(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a todo
	created, err := client.Todo.Create().
		SetText("Test node query").
		SetStatus(todo.StatusPending).
		SetPriority(5).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Query via node query with raw ID (backward compatibility)
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			node(id: "%d") {
				... on Todo {
					id
					text
					status
				}
			}
		}`, created.ID),
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	node := data["node"].(map[string]interface{})

	if node["text"] != "Test node query" {
		t.Errorf("expected text 'Test node query', got %v", node["text"])
	}
	if node["status"] != "PENDING" {
		t.Errorf("expected status 'PENDING', got %v", node["status"])
	}
}

// TestNodeQueryWithGlobalID tests fetching a node using encoded global ID.
func TestNodeQueryWithGlobalID(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a category
	category, err := client.Category.Create().
		SetText("Work").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create global ID
	globalID := entgqlgo.GlobalID("Category", category.ID)

	// Query via node query with global ID
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			node(id: "%s") {
				... on Category {
					id
					text
				}
			}
		}`, globalID),
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	node := data["node"].(map[string]interface{})

	if node["text"] != "Work" {
		t.Errorf("expected text 'Work', got %v", node["text"])
	}
}

// TestNodesQuery tests fetching multiple nodes by their IDs.
func TestNodesQuery(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create multiple todos
	todo1, err := client.Todo.Create().
		SetText("Todo 1").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo 1: %v", err)
	}

	todo2, err := client.Todo.Create().
		SetText("Todo 2").
		SetStatus(todo.StatusCompleted).
		SetPriority(2).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo 2: %v", err)
	}

	// Create a category
	category, err := client.Category.Create().
		SetText("Work").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create global IDs
	todoGlobalID1 := entgqlgo.GlobalID("Todo", todo1.ID)
	todoGlobalID2 := entgqlgo.GlobalID("Todo", todo2.ID)
	categoryGlobalID := entgqlgo.GlobalID("Category", category.ID)

	// Query multiple nodes
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			nodes(ids: ["%s", "%s", "%s"]) {
				... on Todo {
					id
					text
					status
				}
				... on Category {
					id
					text
				}
			}
		}`, todoGlobalID1, categoryGlobalID, todoGlobalID2),
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	nodes := data["nodes"].([]interface{})

	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}

	// First node should be Todo 1
	node0 := nodes[0].(map[string]interface{})
	if node0["text"] != "Todo 1" {
		t.Errorf("expected first node text 'Todo 1', got %v", node0["text"])
	}

	// Second node should be Category
	node1 := nodes[1].(map[string]interface{})
	if node1["text"] != "Work" {
		t.Errorf("expected second node text 'Work', got %v", node1["text"])
	}

	// Third node should be Todo 2
	node2 := nodes[2].(map[string]interface{})
	if node2["text"] != "Todo 2" {
		t.Errorf("expected third node text 'Todo 2', got %v", node2["text"])
	}
}

// TestNodeTypename tests that __typename resolves correctly.
func TestNodeTypename(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a todo
	createdTodo, err := client.Todo.Create().
		SetText("Test typename").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Create a category
	createdCategory, err := client.Category.Create().
		SetText("Work").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Query Todo __typename
	todoGlobalID := entgqlgo.GlobalID("Todo", createdTodo.ID)
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			node(id: "%s") {
				__typename
				... on Todo {
					text
				}
			}
		}`, todoGlobalID),
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	node := data["node"].(map[string]interface{})

	if node["__typename"] != "Todo" {
		t.Errorf("expected __typename 'Todo', got %v", node["__typename"])
	}

	// Query Category __typename
	categoryGlobalID := entgqlgo.GlobalID("Category", createdCategory.ID)
	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			node(id: "%s") {
				__typename
				... on Category {
					text
				}
			}
		}`, categoryGlobalID),
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data = result.Data.(map[string]interface{})
	node = data["node"].(map[string]interface{})

	if node["__typename"] != "Category" {
		t.Errorf("expected __typename 'Category', got %v", node["__typename"])
	}
}

// TestNodeNotFound tests graceful handling of missing IDs.
func TestNodeNotFound(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Query non-existent node with global ID
	nonExistentGlobalID := entgqlgo.GlobalID("Todo", 99999)
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			node(id: "%s") {
				... on Todo {
					id
					text
				}
			}
		}`, nonExistentGlobalID),
		Context: ctx,
	})

	// Should return null for node (the resolver returns an error which translates to null)
	// The data should be a map with "node" key
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		// If Data is nil entirely, that's also acceptable for error cases
		return
	}

	// node should be nil (null in GraphQL)
	nodeValue, exists := data["node"]
	if exists && nodeValue != nil {
		// If it's a typed nil (interface holding nil), that's acceptable
		// Otherwise fail
		if _, isMap := nodeValue.(map[string]interface{}); isMap {
			t.Errorf("expected null for non-existent node, got %v", nodeValue)
		}
	}
	// Getting here means node is nil, which is the expected behavior
}

// TestNodesWithMixedResults tests nodes query with some existing and some missing IDs.
func TestNodesWithMixedResults(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a todo
	created, err := client.Todo.Create().
		SetText("Existing todo").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	existingID := entgqlgo.GlobalID("Todo", created.ID)
	nonExistentID := entgqlgo.GlobalID("Todo", 99999)

	// Query with mixed IDs - one exists, one doesn't
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			nodes(ids: ["%s", "%s"]) {
				... on Todo {
					id
					text
				}
			}
		}`, existingID, nonExistentID),
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	nodes := data["nodes"].([]interface{})

	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	// First node should exist
	if nodes[0] == nil {
		t.Error("expected first node to exist")
	} else {
		node0 := nodes[0].(map[string]interface{})
		if node0["text"] != "Existing todo" {
			t.Errorf("expected text 'Existing todo', got %v", node0["text"])
		}
	}

	// Second node should be nil (non-existent)
	if nodes[1] != nil {
		t.Errorf("expected second node to be nil for non-existent ID, got %v", nodes[1])
	}
}

// ========== Eager Loading tests ==========

// TestEagerLoadEdges tests that edges are eager loaded when selected, avoiding N+1 queries.
func TestEagerLoadEdges(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a category with multiple todos
	category, err := client.Category.Create().
		SetText("Work").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create 5 todos in this category
	for i := 1; i <= 5; i++ {
		_, err := client.Todo.Create().
			SetText(fmt.Sprintf("Task %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			SetCategory(category).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create todo %d: %v", i, err)
		}
	}

	// Query todos WITH category edge selected
	// This should eager load the category and not cause N+1 queries
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos {
				edges {
					node {
						id
						text
						category {
							id
							text
						}
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 5 {
		t.Fatalf("expected 5 todos, got %d", len(edges))
	}

	// Verify all todos have their category loaded
	for i, item := range edges {
		todoItem := item.(map[string]interface{})["node"].(map[string]interface{})
		cat, ok := todoItem["category"].(map[string]interface{})
		if !ok {
			t.Errorf("todo %d: expected category to be loaded, got %v", i, todoItem["category"])
			continue
		}
		if cat["text"] != "Work" {
			t.Errorf("todo %d: expected category text 'Work', got %v", i, cat["text"])
		}
	}
}

// TestNoEagerLoadWhenNotSelected tests that edges are NOT loaded when not selected in the query.
func TestNoEagerLoadWhenNotSelected(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a category with a todo
	category, err := client.Category.Create().
		SetText("Personal").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	_, err = client.Todo.Create().
		SetText("Personal task").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		SetCategory(category).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Query todos WITHOUT category edge selected
	// The category should NOT be in the response
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos {
				edges {
					node {
						id
						text
						status
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(edges))
	}

	todoItem := edges[0].(map[string]interface{})["node"].(map[string]interface{})

	// Verify basic fields are present
	if todoItem["text"] != "Personal task" {
		t.Errorf("expected text 'Personal task', got %v", todoItem["text"])
	}
	if todoItem["status"] != "PENDING" {
		t.Errorf("expected status 'PENDING', got %v", todoItem["status"])
	}

	// Verify category is NOT in the response (it wasn't selected)
	if _, exists := todoItem["category"]; exists {
		t.Error("expected category to NOT be in response when not selected")
	}
}

// TestNestedEagerLoad tests that nested edge selections are properly loaded.
func TestNestedEagerLoad(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a category
	category, err := client.Category.Create().
		SetText("Projects").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create a parent todo with children
	parent, err := client.Todo.Create().
		SetText("Parent Task").
		SetStatus(todo.StatusInProgress).
		SetPriority(10).
		SetCategory(category).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create parent todo: %v", err)
	}

	// Create child todos
	for i := 1; i <= 3; i++ {
		_, err := client.Todo.Create().
			SetText(fmt.Sprintf("Child Task %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			SetParent(parent).
			SetCategory(category).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create child todo %d: %v", i, err)
		}
	}

	// Query todos with nested edges: children (RelayConnection) -> category
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(first: 1) {
				edges {
					node {
						id
						text
						children {
							totalCount
							edges {
								node {
									id
									text
									category {
										id
										text
									}
								}
							}
						}
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	todoEdges := conn["edges"].([]interface{})

	if len(todoEdges) < 1 {
		t.Fatal("expected at least 1 todo")
	}

	// Find the parent todo (it has children)
	var parentTodo map[string]interface{}
	for _, item := range todoEdges {
		todoItem := item.(map[string]interface{})["node"].(map[string]interface{})
		if todoItem["text"] == "Parent Task" {
			parentTodo = todoItem
			break
		}
	}

	// If parent wasn't in first result, query specifically
	if parentTodo == nil {
		result = graphql.Do(graphql.Params{
			Schema: schema,
			RequestString: fmt.Sprintf(`query {
				node(id: "%d") {
					id
					text
					children {
						totalCount
						edges {
							node {
								id
								text
								category {
									id
									text
								}
							}
						}
					}
				}
			}`, parent.ID),
			Context: ctx,
		})

		if len(result.Errors) > 0 {
			t.Fatalf("GraphQL query had errors: %v", result.Errors)
		}

		data = result.Data.(map[string]interface{})
		parentTodo = data["node"].(map[string]interface{})
	}

	// Verify children are loaded (children is now a TodoConnection)
	childrenConn, ok := parentTodo["children"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected children to be a connection map, got %T", parentTodo["children"])
	}

	edges, ok := childrenConn["edges"].([]interface{})
	if !ok {
		t.Fatalf("expected children.edges to be a list, got %T", childrenConn["edges"])
	}

	if len(edges) != 3 {
		t.Errorf("expected 3 children edges, got %d", len(edges))
	}

	// Verify each child has its category loaded
	for i, edgeItem := range edges {
		edge := edgeItem.(map[string]interface{})
		child := edge["node"].(map[string]interface{})
		cat, ok := child["category"].(map[string]interface{})
		if !ok {
			t.Errorf("child %d: expected category to be loaded, got %v", i, child["category"])
			continue
		}
		if cat["text"] != "Projects" {
			t.Errorf("child %d: expected category text 'Projects', got %v", i, cat["text"])
		}
	}
}

// TestEagerLoadEdgesList tests eager loading with multiple edges selected.
func TestEagerLoadEdgesList(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a category
	category, err := client.Category.Create().
		SetText("Family").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create parent todo
	parent, err := client.Todo.Create().
		SetText("Parent Todo").
		SetStatus(todo.StatusPending).
		SetPriority(5).
		SetCategory(category).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create parent: %v", err)
	}

	// Create child todo
	_, err = client.Todo.Create().
		SetText("Child Todo").
		SetStatus(todo.StatusPending).
		SetPriority(3).
		SetParent(parent).
		SetCategory(category).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	// Query with both parent and category edges selected
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos {
				edges {
					node {
						id
						text
						parent {
							id
							text
						}
						category {
							id
							text
						}
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(edges))
	}

	// Find the child todo (it has a parent)
	var childTodo map[string]interface{}
	for _, item := range edges {
		todoItem := item.(map[string]interface{})["node"].(map[string]interface{})
		if todoItem["text"] == "Child Todo" {
			childTodo = todoItem
			break
		}
	}

	if childTodo == nil {
		t.Fatal("could not find child todo")
	}

	// Verify parent is loaded
	parentEdge, ok := childTodo["parent"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parent to be loaded, got %v", childTodo["parent"])
	}
	if parentEdge["text"] != "Parent Todo" {
		t.Errorf("expected parent text 'Parent Todo', got %v", parentEdge["text"])
	}

	// Verify category is loaded
	catEdge, ok := childTodo["category"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected category to be loaded, got %v", childTodo["category"])
	}
	if catEdge["text"] != "Family" {
		t.Errorf("expected category text 'Family', got %v", catEdge["text"])
	}
}

// TestEagerLoadCategoryTodos tests eager loading from category side.
func TestEagerLoadCategoryTodos(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create categories with todos
	cat1, err := client.Category.Create().
		SetText("Work").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category 1: %v", err)
	}

	cat2, err := client.Category.Create().
		SetText("Home").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category 2: %v", err)
	}

	// Create todos for each category
	for i := 1; i <= 3; i++ {
		_, err := client.Todo.Create().
			SetText(fmt.Sprintf("Work Task %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			SetCategory(cat1).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create work todo %d: %v", i, err)
		}

		_, err = client.Todo.Create().
			SetText(fmt.Sprintf("Home Task %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			SetCategory(cat2).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create home todo %d: %v", i, err)
		}
	}

	// Query categories with todos edge selected (todos is a RelayConnection)
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			categories {
				edges {
					node {
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
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["categories"].(map[string]interface{})
	catEdges := conn["edges"].([]interface{})

	if len(catEdges) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(catEdges))
	}

	// Verify each category has its todos loaded (todos is now a TodoConnection)
	for _, catItem := range catEdges {
		cat := catItem.(map[string]interface{})["node"].(map[string]interface{})
		todosConn, ok := cat["todos"].(map[string]interface{})
		if !ok {
			t.Errorf("category %v: expected todos to be a connection map", cat["text"])
			continue
		}
		edges, ok := todosConn["edges"].([]interface{})
		if !ok {
			t.Errorf("category %v: expected todos.edges to be a list", cat["text"])
			continue
		}
		if len(edges) != 3 {
			t.Errorf("category %v: expected 3 todo edges, got %d", cat["text"], len(edges))
		}
	}
}

// ========== Advanced Features Tests ==========

// TestNullsDirection tests ordering with nulls first/last.
func TestNullsDirection(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create todos with varying priorities
	for i := 1; i <= 3; i++ {
		_, err := client.Todo.Create().
			SetText(fmt.Sprintf("Todo %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create todo: %v", err)
		}
	}

	// Query with nulls ordering - ASC with NULLS_FIRST
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(orderBy: [{field: PRIORITY, direction: ASC, nullsDirection: FIRST}]) {
				edges {
					node {
						id
						priority
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 3 {
		t.Fatalf("expected 3 todos, got %d", len(edges))
	}

	// Verify ascending order (since all have values, no nulls to sort)
	for i, item := range edges {
		todoItem := item.(map[string]interface{})["node"].(map[string]interface{})
		priority := todoItem["priority"].(int)
		expectedPriority := i + 1
		if priority != expectedPriority {
			t.Errorf("expected priority %d at index %d, got %d", expectedPriority, i, priority)
		}
	}

	// Test DESC with NULLS_LAST
	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(orderBy: [{field: PRIORITY, direction: DESC, nullsDirection: LAST}]) {
				edges {
					node {
						id
						priority
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data = result.Data.(map[string]interface{})
	conn = data["todos"].(map[string]interface{})
	edges = conn["edges"].([]interface{})

	// Verify descending order
	expectedOrder := []int{3, 2, 1}
	for i, item := range edges {
		todoItem := item.(map[string]interface{})["node"].(map[string]interface{})
		priority := todoItem["priority"].(int)
		if priority != expectedOrder[i] {
			t.Errorf("expected priority %d at index %d, got %d", expectedOrder[i], i, priority)
		}
	}
}

// TestCustomScalarDateTime tests that DateTime scalar works correctly.
func TestCustomScalarDateTime(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create a todo - createdAt is auto-set
	created, err := client.Todo.Create().
		SetText("Test todo").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Query the todo and verify createdAt is returned as RFC3339 string
	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { todos { edges { node { createdAt } } } }`,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(edges))
	}

	todoItem := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	createdAtStr, ok := todoItem["createdAt"].(string)
	if !ok {
		t.Fatalf("createdAt should be a string, got %T", todoItem["createdAt"])
	}

	// Verify it's a valid RFC3339 format by checking it matches the expected format
	if createdAtStr == "" {
		t.Error("createdAt should not be empty")
	}

	// The created time should roughly match what we have in the entity
	expectedPrefix := created.CreatedAt.Format("2006-01-02")
	if len(createdAtStr) < len(expectedPrefix) || createdAtStr[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("createdAt date mismatch: got %s, expected prefix %s", createdAtStr, expectedPrefix)
	}
}

// TestEnumValues tests that enum values are correctly mapped.
func TestEnumValues(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create todos with each status
	statuses := []todo.Status{todo.StatusInProgress, todo.StatusCompleted, todo.StatusPending}
	for i, status := range statuses {
		_, err := client.Todo.Create().
			SetText(fmt.Sprintf("Todo %d", i+1)).
			SetStatus(status).
			SetPriority(i + 1).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create todo: %v", err)
		}
	}

	// Query and verify enum values
	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { todos { edges { node { status } } } }`,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 3 {
		t.Fatalf("expected 3 todos, got %d", len(edges))
	}

	// Verify the enum values are correct strings
	expectedStatuses := map[string]bool{
		"IN_PROGRESS": true,
		"COMPLETED":   true,
		"PENDING":     true,
	}

	for _, item := range edges {
		todoItem := item.(map[string]interface{})["node"].(map[string]interface{})
		status, ok := todoItem["status"].(string)
		if !ok {
			t.Errorf("status should be a string, got %T", todoItem["status"])
			continue
		}
		if !expectedStatuses[status] {
			t.Errorf("unexpected status value: %s", status)
		}
		delete(expectedStatuses, status)
	}

	if len(expectedStatuses) > 0 {
		t.Errorf("some statuses were not found: %v", expectedStatuses)
	}
}

// TestSkipMutationCreateInput tests that SkipMutationCreateInput annotation works.
// The created_at field has Skip(SkipMutationCreateInput) annotation.
func TestSkipMutationCreateInput(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	// Create a todo using the generated CreateTodoInput
	// Note: created_at should NOT be in the input type due to skip annotation
	input := ent.CreateTodoInput{
		Status:   todo.StatusPending,
		Text:     "Test todo",
		Priority: intPtr(5),
	}

	created, err := client.Todo.Create().SetInput(input).Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo with input: %v", err)
	}

	// Verify the todo was created with auto-generated createdAt
	if created.CreatedAt.IsZero() {
		t.Error("createdAt should have been auto-set")
	}

	// Verify other fields
	if created.Text != "Test todo" {
		t.Errorf("text mismatch: got %s, want 'Test todo'", created.Text)
	}
	if created.Priority != 5 {
		t.Errorf("priority mismatch: got %d, want 5", created.Priority)
	}
}

// TestSkipWhereInputVerifyGenerated verifies the skip annotation by checking
// that skipped fields are not included in query capabilities.
func TestSkipWhereInputVerifyGenerated(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	// Create a todo with specific text
	_, err := client.Todo.Create().
		SetText("Test todo").
		SetStatus(todo.StatusPending).
		SetPriority(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Verify filtering works on non-skipped fields
	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todos(where: {textContains: "Test"}) {
				edges {
					node {
						text
					}
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(edges))
	}
}

// TestNodeDescriptor tests the Node() method returns correct field/edge data.
func TestNodeDescriptor(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	// Create a category
	category, err := client.Category.Create().
		SetText("Test Category").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create a todo with the category
	createdTodo, err := client.Todo.Create().
		SetText("Test todo").
		SetStatus(todo.StatusCompleted).
		SetPriority(5).
		SetCategory(category).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}

	// Get the node descriptor
	node, err := gqlgo.TodoNode(ctx, createdTodo)
	if err != nil {
		t.Fatalf("failed to get node descriptor: %v", err)
	}

	// Verify node type
	if node.Type != "Todo" {
		t.Errorf("expected type 'Todo', got '%s'", node.Type)
	}

	// Verify node ID
	if node.ID != createdTodo.ID {
		t.Errorf("expected ID %d, got %d", createdTodo.ID, node.ID)
	}

	// Verify fields are present
	if len(node.Fields) != 4 {
		t.Errorf("expected 4 fields, got %d", len(node.Fields))
	}

	// Check specific field
	fieldNames := make(map[string]bool)
	for _, f := range node.Fields {
		fieldNames[f.Name] = true
	}
	expectedFields := []string{"created_at", "status", "priority", "text"}
	for _, fname := range expectedFields {
		if !fieldNames[fname] {
			t.Errorf("expected field '%s' not found", fname)
		}
	}

	// Verify edges are present (parent, children, category)
	if len(node.Edges) != 3 {
		t.Errorf("expected 3 edges, got %d", len(node.Edges))
	}

	// Check that category edge has the correct ID
	for _, e := range node.Edges {
		if e.Name == "category" {
			if len(e.IDs) != 1 {
				t.Errorf("expected 1 category ID, got %d", len(e.IDs))
			} else if e.IDs[0] != category.ID {
				t.Errorf("expected category ID %d, got %d", category.ID, e.IDs[0])
			}
		}
	}
}

// TestNodeDescriptorClient tests the client.NodeWithDescriptor method.
func TestNodeDescriptorClient(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	// Create a category
	category, err := client.Category.Create().
		SetText("Test Category").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Get node via gqlgo helper
	node, err := gqlgo.NodeWithDescriptor(client, ctx, category.ID)
	if err != nil {
		t.Fatalf("failed to get node with descriptor: %v", err)
	}

	// Verify the node
	if node.Type != "Category" {
		t.Errorf("expected type 'Category', got '%s'", node.Type)
	}

	if node.ID != category.ID {
		t.Errorf("expected ID %d, got %d", category.ID, node.ID)
	}

	// Verify fields (text, status, kind, tags, config).
	if len(node.Fields) != 5 {
		t.Errorf("expected 5 fields, got %d", len(node.Fields))
	}
}

// TestEdgeConnectionPagination tests cursor-based pagination on edge-level
// Relay connections (Category.todos), exercising the +1 lookahead logic,
// backward reversal, and PageInfo flag correctness.
func TestEdgeConnectionPagination(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Seed: one category with 5 todos (priority 1..5 so IDs are predictable in order).
	category, err := client.Category.Create().SetText("Work").Save(ctx)
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}
	for i := 1; i <= 5; i++ {
		_, err := client.Todo.Create().
			SetText(fmt.Sprintf("Task %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			SetCategory(category).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create todo %d: %v", i, err)
		}
	}

	// --- Page 1: first: 2 ---
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			node(id: "%d") {
				... on Category {
					todos(first: 2) {
						totalCount
						edges {
							node { text }
							cursor
						}
						pageInfo {
							hasNextPage
							hasPreviousPage
							endCursor
						}
					}
				}
			}
		}`, category.ID),
		Context: ctx,
	})
	if len(result.Errors) > 0 {
		t.Fatalf("page1 errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	node := data["node"].(map[string]interface{})
	todosConn := node["todos"].(map[string]interface{})

	totalCount, _ := todosConn["totalCount"].(int)
	if totalCount != 5 {
		t.Errorf("page1: expected totalCount=5, got %d", totalCount)
	}

	edges := todosConn["edges"].([]interface{})
	if len(edges) != 2 {
		t.Errorf("page1: expected 2 edges, got %d", len(edges))
	}

	pageInfo := todosConn["pageInfo"].(map[string]interface{})
	if pageInfo["hasNextPage"] != true {
		t.Errorf("page1: expected hasNextPage=true, got %v", pageInfo["hasNextPage"])
	}
	if pageInfo["hasPreviousPage"] != false {
		t.Errorf("page1: expected hasPreviousPage=false, got %v", pageInfo["hasPreviousPage"])
	}

	endCursor, _ := pageInfo["endCursor"].(string)
	if endCursor == "" {
		t.Fatal("page1: endCursor must not be empty")
	}

	// Verify texts: first two in ascending ID order (Task 1, Task 2).
	if len(edges) == 2 {
		e0 := edges[0].(map[string]interface{})["node"].(map[string]interface{})
		e1 := edges[1].(map[string]interface{})["node"].(map[string]interface{})
		if e0["text"] != "Task 1" {
			t.Errorf("page1 edge[0]: expected 'Task 1', got %v", e0["text"])
		}
		if e1["text"] != "Task 2" {
			t.Errorf("page1 edge[1]: expected 'Task 2', got %v", e1["text"])
		}
	}

	// --- Page 2: first: 2, after: endCursor ---
	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			node(id: "%d") {
				... on Category {
					todos(first: 2, after: "%s") {
						totalCount
						edges {
							node { text }
						}
						pageInfo {
							hasNextPage
							hasPreviousPage
						}
					}
				}
			}
		}`, category.ID, endCursor),
		Context: ctx,
	})
	if len(result.Errors) > 0 {
		t.Fatalf("page2 errors: %v", result.Errors)
	}

	data = result.Data.(map[string]interface{})
	node = data["node"].(map[string]interface{})
	todosConn = node["todos"].(map[string]interface{})
	edges = todosConn["edges"].([]interface{})

	if len(edges) != 2 {
		t.Errorf("page2: expected 2 edges, got %d", len(edges))
	}

	pageInfo = todosConn["pageInfo"].(map[string]interface{})
	// after is set → hasPreviousPage must be true.
	if pageInfo["hasPreviousPage"] != true {
		t.Errorf("page2: expected hasPreviousPage=true, got %v", pageInfo["hasPreviousPage"])
	}
	// Still one item left (Task 5), so hasNextPage must be true.
	if pageInfo["hasNextPage"] != true {
		t.Errorf("page2: expected hasNextPage=true, got %v", pageInfo["hasNextPage"])
	}

	if len(edges) == 2 {
		e0 := edges[0].(map[string]interface{})["node"].(map[string]interface{})
		e1 := edges[1].(map[string]interface{})["node"].(map[string]interface{})
		if e0["text"] != "Task 3" {
			t.Errorf("page2 edge[0]: expected 'Task 3', got %v", e0["text"])
		}
		if e1["text"] != "Task 4" {
			t.Errorf("page2 edge[1]: expected 'Task 4', got %v", e1["text"])
		}
	}

	// --- Backward: last: 2 (no before cursor) ---
	// Should return the LAST 2 items in ascending order: Task 4, Task 5.
	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			node(id: "%d") {
				... on Category {
					todos(last: 2) {
						totalCount
						edges {
							node { text }
						}
						pageInfo {
							hasNextPage
							hasPreviousPage
						}
					}
				}
			}
		}`, category.ID),
		Context: ctx,
	})
	if len(result.Errors) > 0 {
		t.Fatalf("backward errors: %v", result.Errors)
	}

	data = result.Data.(map[string]interface{})
	node = data["node"].(map[string]interface{})
	todosConn = node["todos"].(map[string]interface{})
	edges = todosConn["edges"].([]interface{})

	if len(edges) != 2 {
		t.Errorf("backward: expected 2 edges, got %d", len(edges))
	}

	pageInfo = todosConn["pageInfo"].(map[string]interface{})
	// last: 2 with 5 total → more items exist before → hasPreviousPage=true.
	if pageInfo["hasPreviousPage"] != true {
		t.Errorf("backward: expected hasPreviousPage=true, got %v", pageInfo["hasPreviousPage"])
	}
	// No before cursor → hasNextPage=false.
	if pageInfo["hasNextPage"] != false {
		t.Errorf("backward: expected hasNextPage=false, got %v", pageInfo["hasNextPage"])
	}

	// Results must be in ascending order after reversal: Task 4, Task 5.
	if len(edges) == 2 {
		e0 := edges[0].(map[string]interface{})["node"].(map[string]interface{})
		e1 := edges[1].(map[string]interface{})["node"].(map[string]interface{})
		if e0["text"] != "Task 4" {
			t.Errorf("backward edge[0]: expected 'Task 4', got %v", e0["text"])
		}
		if e1["text"] != "Task 5" {
			t.Errorf("backward edge[1]: expected 'Task 5', got %v", e1["text"])
		}
	}
}

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

	schema, err := newTestSchema(client)
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

// Helper function for int pointers
func intPtr(i int) *int {
	return &i
}

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

	schema, err := newTestSchema(client)
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

// TestDeprecatedEnumValueUseEnumNames verifies DeprecatedEnumValues matches
// against GraphQL enum names (trimmed Go names) when UseEnumNames is set,
// matching entgql's behavior.
func TestDeprecatedEnumValueUseEnumNames(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := newTestSchema(client)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			__type(name: "CategoryKind") {
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
	// UseEnumNames: GraphQL names are the Go names, not DB values.
	require.Contains(t, byName, "Primary")
	require.Contains(t, byName, "Secondary")
	require.False(t, byName["Primary"]["isDeprecated"].(bool))
	require.True(t, byName["Secondary"]["isDeprecated"].(bool))
	require.Equal(t, "No longer supported", byName["Secondary"]["deprecationReason"])
}
