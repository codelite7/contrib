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
	"entgo.io/contrib/entgqlgo/internal/todo/ent/predicate"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/todo"

	"entgo.io/ent/dialect/sql"
	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// init registers a custom "textLengthGT" predicate field on TodoWhereInput via
// the public where-input extension seam: WhereInputExtraFields declares the
// GraphQL input field, and WhereInputParseHooks gives it runtime behaviour by
// adding a LENGTH(text) > N predicate to the typed where-input. This models the
// real consumer pattern (gemini extends generated WhereInputs with custom
// predicates), and is why the introspection golden lists textLengthGT on
// TodoWhereInput.
//
// Registration must happen before any test builds/resolves the schema, because
// InputObjectConfigFieldMapThunk caches its result on first resolution and the
// generated *Type vars are package-level — hence init().
func init() {
	gqlgo.WhereInputExtraFields["TodoWhereInput"] = graphql.InputObjectConfigFieldMap{
		"textLengthGT": &graphql.InputObjectFieldConfig{
			Type:        graphql.Int,
			Description: "Filter to todos whose text length is greater than the given value.",
		},
	}
	gqlgo.WhereInputParseHooks["TodoWhereInput"] = append(
		gqlgo.WhereInputParseHooks["TodoWhereInput"],
		func(_ context.Context, raw map[string]interface{}, whereInput interface{}) error {
			v, ok := raw["textLengthGT"]
			if !ok || v == nil {
				return nil
			}
			n, ok := v.(int)
			if !ok {
				return nil
			}
			wi, ok := whereInput.(*gqlgo.TodoWhereInput)
			if !ok {
				return nil
			}
			wi.AddPredicates(predicate.Todo(func(s *sql.Selector) {
				s.Where(sql.ExprP("LENGTH("+s.C(todo.FieldText)+") > ?", n))
			}))
			return nil
		},
	)
}

// seedTodosForLength creates todos with text of varying lengths and returns the
// client. Lengths used: "ab" (2), "abcd" (4), "abcdef" (6), "abcdefgh" (8).
func seedTodosForLength(t *testing.T, ctx context.Context) *ent.Client {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	for _, text := range []string{"ab", "abcd", "abcdef", "abcdefgh"} {
		client.Todo.Create().SetText(text).SetStatus(todo.StatusPending).SaveX(ctx)
	}
	return client
}

func todoTextsFromConnection(t *testing.T, data interface{}) []string {
	t.Helper()
	conn := data.(map[string]interface{})["todos"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	texts := make([]string, 0, len(edges))
	for _, e := range edges {
		node := e.(map[string]interface{})["node"].(map[string]interface{})
		texts = append(texts, node["text"].(string))
	}
	return texts
}

// TestWhereInputExtraField_TopLevel verifies a consumer-registered extra
// where-input field (textLengthGT) is part of the schema and that its parse hook
// runs at the top level, adding a predicate that actually filters the query.
func TestWhereInputExtraField_TopLevel(t *testing.T) {
	ctx := context.Background()
	client := seedTodosForLength(t, ctx)
	defer client.Close()

	schema, err := newTestSchema(client)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { todos(where: {textLengthGT: 5}) { edges { node { text } } } }`,
		Context:       ctx,
	})
	require.Empty(t, result.Errors, "query errors: %v", result.Errors)

	texts := todoTextsFromConnection(t, result.Data)
	require.ElementsMatch(t, []string{"abcdef", "abcdefgh"}, texts,
		"only todos with text length > 5 should be returned")
}

// TestWhereInputExtraField_Nested verifies the parse hook also runs for a
// TodoWhereInput nested inside and: [...], because Parse<Entity>WhereInput
// recurses and runs hooks for each nested input.
func TestWhereInputExtraField_Nested(t *testing.T) {
	ctx := context.Background()
	client := seedTodosForLength(t, ctx)
	defer client.Close()

	schema, err := newTestSchema(client)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { todos(where: {and: [{textLengthGT: 5}]}) { edges { node { text } } } }`,
		Context:       ctx,
	})
	require.Empty(t, result.Errors, "query errors: %v", result.Errors)

	texts := todoTextsFromConnection(t, result.Data)
	require.ElementsMatch(t, []string{"abcdef", "abcdefgh"}, texts,
		"hook must run for the where-input nested inside and:")
}

// TestWhereInputUnregisteredField_ValidationError verifies that querying an
// extra where-input field that was never registered produces a GraphQL
// validation error (graphql-go rejects unknown input fields), rather than being
// silently ignored.
func TestWhereInputUnregisteredField_ValidationError(t *testing.T) {
	ctx := context.Background()
	client := seedTodosForLength(t, ctx)
	defer client.Close()

	schema, err := newTestSchema(client)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { todos(where: {notARegisteredField: 5}) { edges { node { text } } } }`,
		Context:       ctx,
	})
	require.NotEmpty(t, result.Errors,
		"querying an unregistered where-input field must be a validation error")
}
