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
	"errors"
	"testing"

	"entgo.io/contrib/entgqlgo/internal/todo/ent/enttest"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/gqlgo"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/todo"

	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// TestWithTxCommit verifies WithTx commits the transaction when the wrapped
// resolver succeeds.
func TestWithTxCommit(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	cfg := gqlgo.SchemaConfig(client)
	cfg.Mutation.AddFieldConfig("seedTodo", &graphql.Field{
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: gqlgo.WithTx(client, func(p graphql.ResolveParams) (interface{}, error) {
			// The transactional client is in the context; generated resolvers and
			// custom code both retrieve it the same way.
			tc := gqlgo.ClientFromContext(p.Context, client)
			_, err := tc.Todo.Create().
				SetText("inside tx").
				SetStatus(todo.StatusInProgress).
				Save(p.Context)
			return err == nil, err
		}),
	})
	schema, err := graphql.NewSchema(cfg)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { seedTodo }`,
		Context:       ctx,
	})
	require.Empty(t, result.Errors)
	require.Equal(t, 1, client.Todo.Query().CountX(ctx), "committed row must be visible")
}

// TestWithTxRollback verifies WithTx rolls back the transaction when the
// wrapped resolver returns an error.
func TestWithTxRollback(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	cfg := gqlgo.SchemaConfig(client)
	cfg.Mutation.AddFieldConfig("failingMutation", &graphql.Field{
		Type: graphql.Boolean,
		Resolve: gqlgo.WithTx(client, func(p graphql.ResolveParams) (interface{}, error) {
			tc := gqlgo.ClientFromContext(p.Context, client)
			// Write succeeds inside the tx...
			_, err := tc.Todo.Create().
				SetText("will be rolled back").
				SetStatus(todo.StatusInProgress).
				Save(p.Context)
			require.NoError(t, err)
			// ...then the resolver fails.
			return nil, errors.New("boom")
		}),
	})
	schema, err := graphql.NewSchema(cfg)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { failingMutation }`,
		Context:       ctx,
	})
	require.NotEmpty(t, result.Errors)
	require.Zero(t, client.Todo.Query().CountX(ctx), "rolled-back row must not be visible")
}

// TestGeneratedMutationsInTx verifies WithTransactions() wraps generated
// mutation resolvers so they run inside a transaction.
func TestGeneratedMutationsInTx(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	schema, err := gqlgo.NewSchema(client, gqlgo.WithTransactions())
	require.NoError(t, err)

	// A successful generated mutation commits.
	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { createTodo(input: {text: "tx todo", status: IN_PROGRESS, priority: 1}) { id text } }`,
		Context:       ctx,
	})
	require.Empty(t, result.Errors)
	require.Equal(t, 1, client.Todo.Query().CountX(ctx))

	// A failing generated mutation (NotEmpty validator: empty text) leaves no row.
	result = graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { createTodo(input: {text: "", status: IN_PROGRESS, priority: 1}) { id } }`,
		Context:       ctx,
	})
	require.NotEmpty(t, result.Errors)
	require.Equal(t, 1, client.Todo.Query().CountX(ctx), "failed mutation must not add rows")
}
