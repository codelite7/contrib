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

	"entgo.io/contrib/entgqlgo/internal/todo/ent/enttest"

	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// TestImplementsInterface verifies that Category (annotated with
// entgqlgo.Implements("NamedNode")) exposes the interface in the schema and
// supports inline fragments on it.
func TestImplementsInterface(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	client.Category.Create().SetText("Work").SaveX(ctx)

	schema, err := newTestSchema(client)
	require.NoError(t, err)

	// Introspection: Category lists NamedNode among its interfaces.
	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			__type(name: "Category") {
				interfaces { name }
			}
		}`,
		Context: ctx,
	})
	require.Empty(t, result.Errors)

	data := result.Data.(map[string]interface{})
	typeInfo := data["__type"].(map[string]interface{})
	interfaces := typeInfo["interfaces"].([]interface{})

	names := make([]string, 0, len(interfaces))
	for _, i := range interfaces {
		names = append(names, i.(map[string]interface{})["name"].(string))
	}
	require.Contains(t, names, "Node")
	require.Contains(t, names, "NamedNode")

	// Inline fragments on the custom interface work.
	result = graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `query {
			categories {
				edges {
					node {
						... on NamedNode {
							text
						}
					}
				}
			}
		}`,
		Context: ctx,
	})
	require.Empty(t, result.Errors)
	conn := result.Data.(map[string]interface{})["categories"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	require.Len(t, edges, 1)
	node := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	require.Equal(t, "Work", node["text"])
}
