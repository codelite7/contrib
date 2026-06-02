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
	"entgo.io/contrib/entgqlgo/internal/todo/ent"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/gqlgo"

	"github.com/graphql-go/graphql"
)

// namedNodeInterface is the example custom interface implemented by Category
// via the entgqlgo.Implements("NamedNode") annotation.
var namedNodeInterface = graphql.NewInterface(graphql.InterfaceConfig{
	Name:        "NamedNode",
	Description: "An object with a text name.",
	Fields: graphql.Fields{
		"text": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
	},
	ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
		switch p.Value.(type) {
		case *ent.Category:
			return gqlgo.CategoryType
		default:
			return nil
		}
	},
})

func init() {
	gqlgo.CustomInterfaces["NamedNode"] = namedNodeInterface
}

// newTestSchema builds the generated schema with the example's custom
// interfaces registered. All tests in this package use this instead of
// calling gqlgo.NewSchema directly, so schema shape is consistent.
func newTestSchema(client *ent.Client, opts ...gqlgo.SchemaOption) (graphql.Schema, error) {
	return gqlgo.NewSchema(client, opts...)
}
