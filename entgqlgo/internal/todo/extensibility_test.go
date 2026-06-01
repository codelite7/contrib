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
	"time"

	"entgo.io/contrib/entgqlgo/internal/todo/ent/enttest"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/gqlgo"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/todo"

	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// TestCustomQueryField verifies users can add custom root query fields
// alongside the generated ones (code-first equivalent of extra .graphql files).
func TestCustomQueryField(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	cfg := gqlgo.SchemaConfig(client)
	cfg.Query.AddFieldConfig("ping", &graphql.Field{
		Type: graphql.NewNonNull(graphql.String),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return "pong", nil
		},
	})
	schema, err := graphql.NewSchema(cfg)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { ping }`,
		Context:       context.Background(),
	})
	require.Empty(t, result.Errors)
	data := result.Data.(map[string]interface{})
	require.Equal(t, "pong", data["ping"])

	// Generated fields still work on the same schema.
	result = graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `query { todos { totalCount } }`,
		Context:       context.Background(),
	})
	require.Empty(t, result.Errors)
}

// TestCustomMutationField verifies users can add custom mutations that use
// generated types and the ent client.
func TestCustomMutationField(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	client.Todo.Create().SetText("a").SetStatus(todo.StatusInProgress).SaveX(ctx)
	client.Todo.Create().SetText("b").SetStatus(todo.StatusInProgress).SaveX(ctx)

	cfg := gqlgo.SchemaConfig(client)
	cfg.Mutation.AddFieldConfig("clearTodos", &graphql.Field{
		Type:        graphql.NewNonNull(graphql.Int),
		Description: "Delete all todos and return the number deleted.",
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return client.Todo.Delete().Exec(p.Context)
		},
	})
	schema, err := graphql.NewSchema(cfg)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { clearTodos }`,
		Context:       ctx,
	})
	require.Empty(t, result.Errors)
	data := result.Data.(map[string]interface{})
	require.Equal(t, 2, data["clearTodos"])
	require.Zero(t, client.Todo.Query().CountX(ctx))
}

// TestCustomSubscription verifies users can attach a Subscription root type and
// stream events via graphql.Subscribe.
func TestCustomSubscription(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	events := make(chan interface{}, 1)
	cfg := gqlgo.SchemaConfig(client)
	cfg.Subscription = graphql.NewObject(graphql.ObjectConfig{
		Name: "Subscription",
		Fields: graphql.Fields{
			"todoEvents": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return p.Source, nil
				},
				Subscribe: func(p graphql.ResolveParams) (interface{}, error) {
					return events, nil
				},
			},
		},
	})
	schema, err := graphql.NewSchema(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := graphql.Subscribe(graphql.Params{
		Schema:        schema,
		RequestString: `subscription { todoEvents }`,
		Context:       ctx,
	})

	events <- "todo-created"

	select {
	case res := <-results:
		require.Empty(t, res.Errors)
		data := res.Data.(map[string]interface{})
		require.Equal(t, "todo-created", data["todoEvents"])
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for subscription event")
	}
}
