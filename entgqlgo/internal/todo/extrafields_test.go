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
	"entgo.io/contrib/entgqlgo/internal/todo/ent/gqlgo"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/todo"

	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// init registers a computed "textLength" field on the generated Todo object
// type via the public ExtraFields seam. graphql.FieldsThunk caches its result on
// first resolution per *graphql.Object, and the generated *Type vars are
// package-level, so the registration must happen before any test builds/resolves
// the schema — hence init(). This models the real consumer pattern (register
// extra fields once at startup) and is why the introspection golden file lists
// textLength on Todo.
func init() {
	gqlgo.ExtraFields["Todo"] = graphql.Fields{
		"textLength": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.Int),
			Description: "Number of characters in the todo text (computed, not stored).",
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				t, ok := p.Source.(*ent.Todo)
				if !ok {
					return nil, nil
				}
				return len(t.Text), nil
			},
		},
	}
}

// TestExtraFields verifies a consumer-registered extra field on a generated
// object type (Todo.textLength) is part of the schema and resolves through
// graphql.Do — the supported replacement for (*Object).AddFieldConfig, which is
// a silent no-op on thunk-built objects.
func TestExtraFields(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	client.Todo.Create().SetText("hello world").SetStatus(todo.StatusPending).SaveX(ctx)

	schema, err := newTestSchema(client)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { todos { edges { node { text textLength } } } }`,
		Context:       ctx,
	})
	require.Empty(t, result.Errors, "extra-field query errors: %v", result.Errors)

	conn := result.Data.(map[string]interface{})["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	require.Len(t, edges, 1)
	node := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	require.Equal(t, "hello world", node["text"])
	require.Equal(t, len("hello world"), node["textLength"])
}
