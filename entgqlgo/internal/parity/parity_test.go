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

package parity

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"entgo.io/contrib/entgqlgo/internal/parity/ent/enttest"
	"entgo.io/contrib/entgqlgo/internal/parity/ent/gqlgo"

	"github.com/graphql-go/graphql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// allowlistedDifferences are schema elements that intentionally differ between
// entgql+gqlgen and entgqlgo. Each entry documents why.
//
// Keys use the form "field:<fieldName>" to match a root field name prefix.
var allowlistedDifferences = map[string]string{
	// entgqlgo generates delete mutations as an entgqlgo superset feature.
	// entgql has no equivalent — gqlgen users hand-write their own delete
	// mutations (or omit them). There is no entgql-convention reference to
	// compare against, so delete mutations are explicitly allowlisted.
	"field:deleteCategory": "entgqlgo superset: delete mutations have no entgql reference convention",
	"field:deleteTodo":     "entgqlgo superset: delete mutations have no entgql reference convention",
}

// entgqlMutationReference is the set of mutation signatures entgql users
// conventionally hand-write for their gqlgen apps (see entgql/internal/todo/todo.graphql).
// entgql does not generate the Mutation root; entgqlgo does. The generated
// mutations must match the shapes entgql users would write, adapted to the
// parity schema's two entities (Category and Todo).
//
// Signature format mirrors fieldSignature() + typeRefToSDL() output:
//   name(argA:TypeA!, argB:TypeB!): ReturnType!
// Args are sorted alphabetically (id before input).
var entgqlMutationReference = map[string]bool{
	"createCategory(input:CreateCategoryInput!): Category!":          true,
	"updateCategory(id:ID!, input:UpdateCategoryInput!): Category!":  true,
	"createTodo(input:CreateTodoInput!): Todo!":                      true,
	"updateTodo(id:ID!, input:UpdateTodoInput!): Todo!":              true,
}

// fieldSignature renders "name(arg:Type, ...): ReturnType" for comparison.
func fieldSignature(name string, args []string, ret string) string {
	sort.Strings(args)
	return fmt.Sprintf("%s(%s): %s", name, strings.Join(args, ", "), ret)
}

// sdlQueryFields parses ent.graphql and returns the signature set of root Query fields.
func sdlQueryFields(t *testing.T) map[string]bool {
	t.Helper()
	return sdlRootFields(t, "Query")
}

func sdlRootFields(t *testing.T, rootType string) map[string]bool {
	t.Helper()
	sdl, err := os.ReadFile("ent.graphql")
	require.NoError(t, err)

	doc, gqlErr := gqlparser.LoadSchema(&ast.Source{Name: "ent.graphql", Input: string(sdl)})
	require.Nil(t, gqlErr)

	var def *ast.Definition
	switch rootType {
	case "Query":
		def = doc.Query
	case "Mutation":
		def = doc.Mutation
	}
	fields := map[string]bool{}
	if def == nil {
		return fields
	}
	for _, f := range def.Fields {
		if strings.HasPrefix(f.Name, "__") {
			continue
		}
		args := make([]string, 0, len(f.Arguments))
		for _, a := range f.Arguments {
			args = append(args, fmt.Sprintf("%s:%s", a.Name, a.Type.String()))
		}
		fields[fieldSignature(f.Name, args, f.Type.String())] = true
	}
	return fields
}

// introspectionRootFields introspects the gqlgo schema and returns the
// signature set of root fields for the given root type, in SDL type notation.
func introspectionRootFields(t *testing.T, rootType string) map[string]bool {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := gqlgo.NewSchema(client)
	require.NoError(t, err)

	rootField := "queryType"
	if rootType == "Mutation" {
		rootField = "mutationType"
	}

	result := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: fmt.Sprintf(`query {
			__schema {
				%s {
					fields {
						name
						args { name type { ...T } }
						type { ...T }
					}
				}
			}
		}
		fragment T on __Type {
			kind name
			ofType { kind name ofType { kind name ofType { kind name } } }
		}`, rootField),
		Context: context.Background(),
	})
	require.Empty(t, result.Errors)

	fields := map[string]bool{}
	schemaObj, ok := result.Data.(map[string]interface{})["__schema"].(map[string]interface{})
	if !ok {
		return fields
	}
	rootObj, ok := schemaObj[rootField].(map[string]interface{})
	if !ok {
		return fields
	}
	for _, f := range rootObj["fields"].([]interface{}) {
		fm := f.(map[string]interface{})
		name := fm["name"].(string)
		args := []string{}
		for _, a := range fm["args"].([]interface{}) {
			am := a.(map[string]interface{})
			args = append(args, fmt.Sprintf("%s:%s", am["name"].(string), typeRefToSDL(am["type"].(map[string]interface{}))))
		}
		fields[fieldSignature(name, args, typeRefToSDL(fm["type"].(map[string]interface{})))] = true
	}
	return fields
}

// typeRefToSDL converts an introspection type ref to SDL notation ([Todo!]!, Cursor, etc).
func typeRefToSDL(ref map[string]interface{}) string {
	kind, _ := ref["kind"].(string)
	switch kind {
	case "NON_NULL":
		return typeRefToSDL(ref["ofType"].(map[string]interface{})) + "!"
	case "LIST":
		return "[" + typeRefToSDL(ref["ofType"].(map[string]interface{})) + "]"
	default:
		name, _ := ref["name"].(string)
		return name
	}
}

// TestRootQueryParity compares root Query fields between entgql's SDL and
// entgqlgo's introspected schema.
func TestRootQueryParity(t *testing.T) {
	compareFields(t, sdlQueryFields(t), introspectionRootFields(t, "Query"))
}

// TestRootMutationParity compares root Mutation fields between an inline
// entgql-convention reference and entgqlgo's introspected schema. entgql does
// not generate the Mutation root type (gqlgen users hand-write it), so the
// reference is derived from the entgql hand-written example at
// entgql/internal/todo/todo.graphql, adapted to the parity schema.
// Only delete mutations are allowlisted — they are an entgqlgo superset with
// no entgql reference convention.
func TestRootMutationParity(t *testing.T) {
	compareFields(t, entgqlMutationReference, introspectionRootFields(t, "Mutation"))
}

func compareFields(t *testing.T, sdlFields, gqlgoFields map[string]bool) {
	t.Helper()
	var missing, extra []string
	for sig := range sdlFields {
		if !gqlgoFields[sig] {
			missing = append(missing, sig)
		}
	}
	for sig := range gqlgoFields {
		if !sdlFields[sig] {
			extra = append(extra, sig)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	missing = filterAllowlisted(missing)
	extra = filterAllowlisted(extra)

	if len(missing) > 0 || len(extra) > 0 {
		t.Errorf("Root field parity mismatch.\nIn entgql SDL but not in entgqlgo:\n  %s\nIn entgqlgo but not in entgql SDL:\n  %s",
			strings.Join(missing, "\n  "), strings.Join(extra, "\n  "))
	}
}

func filterAllowlisted(sigs []string) []string {
	out := make([]string, 0, len(sigs))
	for _, s := range sigs {
		allowed := false
		for key := range allowlistedDifferences {
			name := strings.TrimPrefix(strings.TrimPrefix(key, "field:"), "directive:")
			if strings.HasPrefix(s, name+"(") || strings.HasPrefix(s, name+":") {
				allowed = true
				break
			}
		}
		if !allowed {
			out = append(out, s)
		}
	}
	return out
}
