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

	"entgo.io/contrib/entgqlgo/internal/todo/ent"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/enttest"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/todo"

	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
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
	s.schema, err = ent.NewSchema(s.client)
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
				id
				text
			}
		}
	`)

	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)
	s.Require().NotNil(result.Data)

	data, ok := result.Data.(map[string]interface{})
	s.Require().True(ok, "result.Data should be a map")

	todos, ok := data["todos"].([]interface{})
	s.Require().True(ok, "todos should be a slice")
	s.Require().Len(todos, 3, "should have 3 todos")
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
				id
				text
			}
		}
	`)

	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)
	s.Require().NotNil(result.Data)

	data, ok := result.Data.(map[string]interface{})
	s.Require().True(ok, "result.Data should be a map")

	categories, ok := data["categories"].([]interface{})
	s.Require().True(ok, "categories should be a slice")
	s.Require().Len(categories, 2, "should have 2 categories")
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
				id
				text
				status
				priority
				createdAt
			}
		}
	`)

	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)
	s.Require().NotNil(result.Data)

	data := result.Data.(map[string]interface{})
	todos := data["todos"].([]interface{})
	s.Require().Len(todos, 1)

	todo := todos[0].(map[string]interface{})

	// Verify each field
	s.Equal(fmt.Sprintf("%d", created.ID), todo["id"], "id should match")
	s.Equal("Test todo", todo["text"], "text should match")
	s.Equal("IN_PROGRESS", todo["status"], "status should match")
	s.Equal(5, todo["priority"], "priority should match")
	s.NotNil(todo["createdAt"], "createdAt should not be nil")
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

	// Query by ID via GraphQL
	result := s.executeQuery(fmt.Sprintf(`
		query {
			todo(id: "%d") {
				id
				text
				status
			}
		}
	`, created.ID))

	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)
	s.Require().NotNil(result.Data)

	data := result.Data.(map[string]interface{})
	todoResult := data["todo"].(map[string]interface{})

	s.Equal(fmt.Sprintf("%d", created.ID), todoResult["id"])
	s.Equal("Single todo", todoResult["text"])
	s.Equal("COMPLETED", todoResult["status"])
}

// TestQueryTodosPagination tests pagination on todos query.
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

	// Query with first: 2
	result := s.executeQuery(`
		query {
			todos(first: 2) {
				id
			}
		}
	`)

	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)

	data := result.Data.(map[string]interface{})
	todos := data["todos"].([]interface{})
	s.Require().Len(todos, 2, "should have 2 todos with first: 2")

	// Query with offset: 3
	result = s.executeQuery(`
		query {
			todos(offset: 3) {
				id
			}
		}
	`)

	s.Require().Empty(result.Errors, "GraphQL query should not have errors: %v", result.Errors)

	data = result.Data.(map[string]interface{})
	todos = data["todos"].([]interface{})
	s.Require().Len(todos, 2, "should have 2 todos with offset: 3 (5 total - 3 skipped)")
}

// ========== Top-level tests that match -run 'TestQuery' ==========
// These are standalone tests that duplicate some suite functionality
// but allow running with the specified test pattern.

// TestQueryTodos creates todos and queries them via GraphQL.
func TestQueryTodos(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := ent.NewSchema(client)
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
		RequestString: `query { todos { id text } }`,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatal("result.Data is not a map")
	}

	todos, ok := data["todos"].([]interface{})
	if !ok {
		t.Fatal("todos is not a slice")
	}

	if len(todos) != 3 {
		t.Fatalf("expected 3 todos, got %d", len(todos))
	}
}

// TestQueryCategories creates categories and queries them via GraphQL.
func TestQueryCategories(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := ent.NewSchema(client)
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
		RequestString: `query { categories { id text } }`,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatal("result.Data is not a map")
	}

	categories, ok := data["categories"].([]interface{})
	if !ok {
		t.Fatal("categories is not a slice")
	}

	if len(categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(categories))
	}
}

// TestQueryTodoFields verifies all todo fields are resolved correctly.
func TestQueryTodoFields(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := ent.NewSchema(client)
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
		RequestString: `query { todos { id text status priority createdAt } }`,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	todos := data["todos"].([]interface{})
	if len(todos) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(todos))
	}

	todoItem := todos[0].(map[string]interface{})

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
