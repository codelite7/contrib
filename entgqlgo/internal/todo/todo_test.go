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

	"entgo.io/contrib/entgqlgo/internal/todo/ent"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/enttest"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/suite"
)

// TodoTestSuite is a test suite for the todo module.
type TodoTestSuite struct {
	suite.Suite
	client *ent.Client
	ctx    context.Context
}

// SetupTest runs before each test.
func (s *TodoTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.client = enttest.Open(s.T(), "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
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
