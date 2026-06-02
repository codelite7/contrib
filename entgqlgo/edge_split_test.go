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

package entgqlgo

import (
	"testing"

	"entgo.io/ent/entc/gen"
	"github.com/stretchr/testify/assert"
)

// node returns a *gen.Type whose Name is the pascal-cased source-node name used
// by the split-layout hoisted edge functions (Query<TypeName><EdgeStructField>).
func node(name string) *gen.Type {
	return &gen.Type{Name: name}
}

func TestEdgeQueryExpr(t *testing.T) {
	n := node("Todo")
	e := newEdge("parent", true)

	assert.Equal(t, `source.QueryParent()`,
		edgeQueryExpr(false, n, e, "ent", "source", "r.client.Todo"))
	assert.Equal(t, `ent.QueryTodoParent(r.client.Todo, source)`,
		edgeQueryExpr(true, n, e, "ent", "source", "r.client.Todo"))

	// Non-unique edge and a different entity/client expression (node_descriptor
	// derives the typed client from the entity's embedded Config).
	n2 := node("Category")
	e2 := newEdge("todos", false)
	assert.Equal(t, `n.QueryTodos()`,
		edgeQueryExpr(false, n2, e2, "ent", "n", "ent.NewCategoryClient(n.Config)"))
	assert.Equal(t, `ent.QueryCategoryTodos(ent.NewCategoryClient(n.Config), n)`,
		edgeQueryExpr(true, n2, e2, "ent", "n", "ent.NewCategoryClient(n.Config)"))
}

func TestEdgeWithExpr(t *testing.T) {
	n := node("Todo")
	e := newEdge("parent", true)

	// No options.
	assert.Equal(t, `query.WithParent()`,
		edgeWithExpr(false, n, e, "ent", "query"))
	assert.Equal(t, `ent.WithTodoParent(query)`,
		edgeWithExpr(true, n, e, "ent", "query"))

	// With a sub-query option closure (nested field collection).
	closure := "func(q *ent.TodoQuery) { /* ... */ }"
	assert.Equal(t, `query.WithParent(func(q *ent.TodoQuery) { /* ... */ })`,
		edgeWithExpr(false, n, e, "ent", "query", closure))
	assert.Equal(t, `ent.WithTodoParent(query, func(q *ent.TodoQuery) { /* ... */ })`,
		edgeWithExpr(true, n, e, "ent", "query", closure))

	// Non-unique edge.
	n2 := node("Category")
	e2 := newEdge("todos", false)
	assert.Equal(t, `query.WithTodos()`,
		edgeWithExpr(false, n2, e2, "ent", "query"))
	assert.Equal(t, `ent.WithCategoryTodos(query)`,
		edgeWithExpr(true, n2, e2, "ent", "query"))
}
