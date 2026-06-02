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

// introspectionQuery pulls every type's full field/input/enum shape so the
// introspected schema can be re-rendered as SDL and diffed against entgql's.
const introspectionQuery = `query {
  __schema {
    types {
      kind
      name
      fields(includeDeprecated: true) {
        name
        type { ...T }
      }
      inputFields {
        name
        type { ...T }
      }
      enumValues(includeDeprecated: true) {
        name
      }
    }
  }
}
fragment T on __Type {
  kind name
  ofType { kind name ofType { kind name ofType { kind name ofType { kind name } } } }
}`

// introspectionToSDL re-renders an introspection result into a minimal SDL
// document (type/input/enum bodies only, no directives/descriptions) suitable
// for re-parsing and field-shape comparison against entgql's ent.graphql.
func introspectionToSDL(t *testing.T, data map[string]interface{}) string {
	t.Helper()
	schemaObj := data["__schema"].(map[string]interface{})
	var b strings.Builder
	for _, raw := range schemaObj["types"].([]interface{}) {
		tp := raw.(map[string]interface{})
		name, _ := tp["name"].(string)
		if name == "" || strings.HasPrefix(name, "__") {
			continue
		}
		kind, _ := tp["kind"].(string)
		switch kind {
		case "OBJECT", "INTERFACE":
			keyword := "type"
			if kind == "INTERFACE" {
				keyword = "interface"
			}
			fmt.Fprintf(&b, "%s %s {\n", keyword, name)
			for _, fr := range tp["fields"].([]interface{}) {
				fm := fr.(map[string]interface{})
				fmt.Fprintf(&b, "  %s: %s\n", fm["name"].(string), typeRefToSDL(fm["type"].(map[string]interface{})))
			}
			b.WriteString("}\n")
		case "INPUT_OBJECT":
			fmt.Fprintf(&b, "input %s {\n", name)
			for _, fr := range tp["inputFields"].([]interface{}) {
				fm := fr.(map[string]interface{})
				fmt.Fprintf(&b, "  %s: %s\n", fm["name"].(string), typeRefToSDL(fm["type"].(map[string]interface{})))
			}
			b.WriteString("}\n")
		case "ENUM":
			fmt.Fprintf(&b, "enum %s {\n", name)
			for _, vr := range tp["enumValues"].([]interface{}) {
				vm := vr.(map[string]interface{})
				fmt.Fprintf(&b, "  %s\n", vm["name"].(string))
			}
			b.WriteString("}\n")
		case "SCALAR":
			if name != "String" && name != "Int" && name != "Float" && name != "Boolean" && name != "ID" {
				fmt.Fprintf(&b, "scalar %s\n", name)
			}
		}
	}
	return b.String()
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

// loadEntgqlSDL parses entgql's reference ent.graphql once.
func loadEntgqlSDL(t *testing.T) *ast.Schema {
	t.Helper()
	sdl, err := os.ReadFile("ent.graphql")
	require.NoError(t, err)
	doc, gqlErr := gqlparser.LoadSchema(&ast.Source{Name: "ent.graphql", Input: string(sdl)})
	require.Nil(t, gqlErr)
	return doc
}

// registerCustomScalars registers the custom scalars entgqlgo routes through
// its CustomTypes registry (Map for map[string]interface{} JSON fields, UUID,
// Upload). entgql declares these scalars in its SDL; entgqlgo only resolves
// them to concrete graphql.Type values when the consumer registers them, so the
// parity comparison must register them too. They are defined minimally — the
// comparison only inspects type names, not serialization behaviour.
func registerCustomScalars() {
	register := func(name string) {
		if _, ok := gqlgo.CustomTypes[name]; ok {
			return
		}
		gqlgo.CustomTypes[name] = graphql.NewScalar(graphql.ScalarConfig{
			Name:      name,
			Serialize: func(v interface{}) interface{} { return v },
		})
	}
	register("Map")
	register("UUID")
	register("Upload")
}

// introspectGqlgo introspects entgqlgo's generated schema and returns it as an
// AST schema so it can be compared field-by-field against entgql's SDL.
func introspectGqlgo(t *testing.T) *ast.Schema {
	t.Helper()
	registerCustomScalars()
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := gqlgo.NewSchema(client)
	require.NoError(t, err)

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: introspectionQuery,
		Context:       context.Background(),
	})
	require.Empty(t, result.Errors)

	sdl := introspectionToSDL(t, result.Data.(map[string]interface{}))
	doc, gqlErr := gqlparser.LoadSchema(&ast.Source{Name: "introspected.graphql", Input: sdl})
	require.Nil(t, gqlErr, "parsing introspected SDL: %v\n%s", gqlErr, sdl)
	return doc
}

// objectTypeFieldSig renders "name: TypeSDL" for one field, ignoring directives,
// descriptions, and argument details (covered separately by the root parity tests).
func objectTypeFieldSig(f *ast.FieldDefinition) string {
	return fmt.Sprintf("%s: %s", f.Name, f.Type.String())
}

func inputFieldSig(f *ast.FieldDefinition) string {
	return fmt.Sprintf("%s: %s", f.Name, f.Type.String())
}

// typeFieldSet returns the set of "name: Type" signatures for the named type in
// the schema, or nil if the type is absent.
func typeFieldSet(s *ast.Schema, typeName string) map[string]bool {
	def, ok := s.Types[typeName]
	if !ok {
		return nil
	}
	out := map[string]bool{}
	for _, f := range def.Fields {
		if strings.HasPrefix(f.Name, "__") {
			continue
		}
		out[objectTypeFieldSig(f)] = true
	}
	return out
}

func enumValueSet(s *ast.Schema, typeName string) map[string]bool {
	def, ok := s.Types[typeName]
	if !ok {
		return nil
	}
	out := map[string]bool{}
	for _, v := range def.EnumValues {
		out[v.Name] = true
	}
	return out
}

// TestTypeShapeParity compares the field shape (name + type, including
// nullability and list wrappers) of every object, input, and enum type entgql
// declares against entgqlgo's introspected schema. This catches nested
// mismatches the root-field parity tests cannot see: object-field enum types
// (A2), required-field / connection-edge nullability (A3), Order.direction
// nullability (A1), required-edge create-input ID nullability (A5a), and
// edge/field naming (A5b).
func TestTypeShapeParity(t *testing.T) {
	sdl := loadEntgqlSDL(t)
	got := introspectGqlgo(t)

	for name, def := range sdl.Types {
		if strings.HasPrefix(name, "__") {
			continue
		}
		// Built-in scalars and the Node interface model carry no field shape to
		// compare; skip scalars and the introspection plumbing.
		switch def.Kind {
		case ast.Scalar:
			continue
		case ast.Interface:
			// Node is the only interface; entgqlgo models it identically.
			continue
		}
		// Skip entgqlgo-superset / entgql-only roots handled elsewhere.
		if name == "Query" || name == "Mutation" {
			continue
		}
		if def.Kind == ast.Enum {
			want := enumValueSet(sdl, name)
			have := enumValueSet(got, name)
			if have == nil {
				t.Errorf("enum %s present in entgql SDL but missing from entgqlgo", name)
				continue
			}
			compareSets(t, "enum "+name+" values", want, have)
			continue
		}
		want := typeFieldSet(sdl, name)
		have := typeFieldSet(got, name)
		if have == nil {
			t.Errorf("type %s present in entgql SDL but missing from entgqlgo", name)
			continue
		}
		compareSets(t, string(def.Kind)+" "+name+" fields", want, have)
	}
}

// compareSets reports symmetric-difference between want and have for a labelled
// element set, after applying the field-level allowlist.
func compareSets(t *testing.T, label string, want, have map[string]bool) {
	t.Helper()
	var missing, extra []string
	for s := range want {
		if !have[s] {
			missing = append(missing, s)
		}
	}
	for s := range have {
		if !want[s] {
			extra = append(extra, s)
		}
	}
	missing = filterAllowlisted(missing)
	extra = filterAllowlisted(extra)
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Errorf("%s parity mismatch.\nIn entgql SDL but not in entgqlgo:\n  %s\nIn entgqlgo but not in entgql SDL:\n  %s",
			label, strings.Join(missing, "\n  "), strings.Join(extra, "\n  "))
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
