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

// ========== Relay-style cursor pagination tests ==========

// TestPageForward tests forward pagination using first/after.
func TestPageForward(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := ent.NewSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create 10 todos
	for i := 1; i <= 10; i++ {
		_, err := client.Todo.Create().
			SetText(fmt.Sprintf("Todo %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create todo: %v", err)
		}
	}

	// Query first 3 todos
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todosConnection(first: 3) {
				edges {
					node {
						id
						text
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPreviousPage
					startCursor
					endCursor
				}
				totalCount
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todosConnection"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	pageInfo := conn["pageInfo"].(map[string]interface{})

	// Verify first page
	if len(edges) != 3 {
		t.Errorf("expected 3 edges, got %d", len(edges))
	}
	if !pageInfo["hasNextPage"].(bool) {
		t.Error("expected hasNextPage to be true")
	}
	if pageInfo["hasPreviousPage"].(bool) {
		t.Error("expected hasPreviousPage to be false")
	}

	// Get the end cursor
	endCursor := pageInfo["endCursor"].(string)
	if endCursor == "" {
		t.Fatal("expected endCursor to be non-empty")
	}

	// Query next page using after cursor
	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			todosConnection(first: 3, after: "%s") {
				edges {
					node {
						id
						text
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPreviousPage
				}
				totalCount
			}
		}`, endCursor),
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query with after cursor had errors: %v", result.Errors)
	}

	data = result.Data.(map[string]interface{})
	conn = data["todosConnection"].(map[string]interface{})
	edges = conn["edges"].([]interface{})
	pageInfo = conn["pageInfo"].(map[string]interface{})

	// Verify second page
	if len(edges) != 3 {
		t.Errorf("expected 3 edges on second page, got %d", len(edges))
	}
	if !pageInfo["hasNextPage"].(bool) {
		t.Error("expected hasNextPage to be true on second page")
	}
	// After using a cursor, hasPreviousPage should be true
	if !pageInfo["hasPreviousPage"].(bool) {
		t.Error("expected hasPreviousPage to be true after using after cursor")
	}

	// Verify we got different items
	firstEdge := edges[0].(map[string]interface{})
	firstNode := firstEdge["node"].(map[string]interface{})
	if firstNode["text"].(string) != "Todo 4" {
		t.Errorf("expected first item on second page to be 'Todo 4', got '%s'", firstNode["text"])
	}
}

// TestPageBackward tests backward pagination using last/before.
func TestPageBackward(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := ent.NewSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create 10 todos
	for i := 1; i <= 10; i++ {
		_, err := client.Todo.Create().
			SetText(fmt.Sprintf("Todo %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create todo: %v", err)
		}
	}

	// Query last 3 todos
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todosConnection(last: 3) {
				edges {
					node {
						id
						text
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPreviousPage
					startCursor
					endCursor
				}
				totalCount
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todosConnection"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	pageInfo := conn["pageInfo"].(map[string]interface{})

	// Verify last page
	if len(edges) != 3 {
		t.Errorf("expected 3 edges, got %d", len(edges))
	}
	if pageInfo["hasNextPage"].(bool) {
		t.Error("expected hasNextPage to be false when using last")
	}
	if !pageInfo["hasPreviousPage"].(bool) {
		t.Error("expected hasPreviousPage to be true")
	}

	// Verify we got the last items (Todo 8, 9, 10)
	firstEdge := edges[0].(map[string]interface{})
	firstNode := firstEdge["node"].(map[string]interface{})
	if firstNode["text"].(string) != "Todo 8" {
		t.Errorf("expected first item to be 'Todo 8', got '%s'", firstNode["text"])
	}

	lastEdge := edges[2].(map[string]interface{})
	lastNode := lastEdge["node"].(map[string]interface{})
	if lastNode["text"].(string) != "Todo 10" {
		t.Errorf("expected last item to be 'Todo 10', got '%s'", lastNode["text"])
	}

	// Get the start cursor for the next query
	startCursor := pageInfo["startCursor"].(string)

	// Query previous page using before cursor
	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			todosConnection(last: 3, before: "%s") {
				edges {
					node {
						id
						text
					}
				}
				pageInfo {
					hasNextPage
					hasPreviousPage
				}
			}
		}`, startCursor),
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query with before cursor had errors: %v", result.Errors)
	}

	data = result.Data.(map[string]interface{})
	conn = data["todosConnection"].(map[string]interface{})
	edges = conn["edges"].([]interface{})

	// Verify we got the previous 3 items (Todo 5, 6, 7)
	if len(edges) != 3 {
		t.Errorf("expected 3 edges, got %d", len(edges))
	}

	firstEdge = edges[0].(map[string]interface{})
	firstNode = firstEdge["node"].(map[string]interface{})
	if firstNode["text"].(string) != "Todo 5" {
		t.Errorf("expected first item to be 'Todo 5', got '%s'", firstNode["text"])
	}
}

// TestTotalCount tests that totalCount is accurate.
func TestTotalCount(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := ent.NewSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create 7 todos
	for i := 1; i <= 7; i++ {
		_, err := client.Todo.Create().
			SetText(fmt.Sprintf("Todo %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create todo: %v", err)
		}
	}

	// Query with pagination but verify totalCount reflects all items
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todosConnection(first: 2) {
				edges {
					node {
						id
					}
				}
				totalCount
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todosConnection"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	totalCount := conn["totalCount"].(int)

	// Verify we only got 2 items but totalCount is 7
	if len(edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(edges))
	}
	if totalCount != 7 {
		t.Errorf("expected totalCount to be 7, got %d", totalCount)
	}
}

// TestEmptyPage tests querying past the end returns empty edges.
func TestEmptyPage(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := ent.NewSchema(client)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create 3 todos
	var lastTodo *ent.Todo
	for i := 1; i <= 3; i++ {
		lastTodo, err = client.Todo.Create().
			SetText(fmt.Sprintf("Todo %d", i)).
			SetStatus(todo.StatusPending).
			SetPriority(i).
			Save(ctx)
		if err != nil {
			t.Fatalf("failed to create todo: %v", err)
		}
	}

	// First get all todos to get the last cursor
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todosConnection(first: 10) {
				edges {
					cursor
				}
				pageInfo {
					endCursor
				}
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query had errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["todosConnection"].(map[string]interface{})
	pageInfo := conn["pageInfo"].(map[string]interface{})
	endCursor := pageInfo["endCursor"].(string)

	// Query after the last item - should get empty edges
	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			todosConnection(first: 5, after: "%s") {
				edges {
					node {
						id
					}
				}
				pageInfo {
					hasNextPage
					hasPreviousPage
				}
				totalCount
			}
		}`, endCursor),
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query after last had errors: %v", result.Errors)
	}

	data = result.Data.(map[string]interface{})
	conn = data["todosConnection"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	pageInfo = conn["pageInfo"].(map[string]interface{})
	totalCount := conn["totalCount"].(int)

	// Verify empty edges but correct metadata
	if len(edges) != 0 {
		t.Errorf("expected 0 edges when querying past end, got %d", len(edges))
	}
	if pageInfo["hasNextPage"].(bool) {
		t.Error("expected hasNextPage to be false when at end")
	}
	if !pageInfo["hasPreviousPage"].(bool) {
		t.Error("expected hasPreviousPage to be true when past beginning")
	}
	if totalCount != 3 {
		t.Errorf("expected totalCount to still be 3, got %d", totalCount)
	}

	// Also verify empty result when no items exist
	// Delete all todos
	client.Todo.DeleteOneID(lastTodo.ID).Exec(ctx)
	client.Todo.Delete().Exec(ctx)

	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			todosConnection(first: 5) {
				edges {
					node {
						id
					}
				}
				totalCount
			}
		}`,
		Context: ctx,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL query on empty table had errors: %v", result.Errors)
	}

	data = result.Data.(map[string]interface{})
	conn = data["todosConnection"].(map[string]interface{})
	edges = conn["edges"].([]interface{})
	totalCount = conn["totalCount"].(int)

	if len(edges) != 0 {
		t.Errorf("expected 0 edges on empty table, got %d", len(edges))
	}
	if totalCount != 0 {
		t.Errorf("expected totalCount to be 0 on empty table, got %d", totalCount)
	}
}
