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
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/assert"
)

// jsonFieldWithType builds a JSON-typed field carrying an entgqlgo.Type
// annotation with the given SDL type expression.
func jsonFieldWithType(sdl string) *gen.Field {
	return &gen.Field{
		Name: "custom",
		Type: &field.TypeInfo{Type: field.TypeJSON},
		Annotations: gen.Annotations{
			"EntGQL": map[string]interface{}{"Type": sdl},
		},
	}
}

// TestSDLTypeToGo verifies that GraphQL SDL type expressions from entgqlgo.Type
// annotations are translated into valid graphql-go Go expressions rather than
// being emitted verbatim (which produces invalid Go source).
func TestSDLTypeToGo(t *testing.T) {
	tests := []struct {
		name     string
		sdl      string
		expected string
	}{
		// Built-in scalars.
		{name: "String", sdl: "String", expected: "graphql.String"},
		{name: "Int", sdl: "Int", expected: "graphql.Int"},
		{name: "Float", sdl: "Float", expected: "graphql.Float"},
		{name: "Boolean", sdl: "Boolean", expected: "graphql.Boolean"},
		{name: "ID", sdl: "ID", expected: "graphql.ID"},
		{name: "Time maps to TimeScalar", sdl: "Time", expected: "TimeScalar"},

		// Non-null wrapper.
		{name: "String!", sdl: "String!", expected: "graphql.NewNonNull(graphql.String)"},
		{name: "ID!", sdl: "ID!", expected: "graphql.NewNonNull(graphql.ID)"},

		// List wrappers.
		{name: "[String]", sdl: "[String]", expected: "graphql.NewList(graphql.String)"},
		{name: "[String!]", sdl: "[String!]", expected: "graphql.NewList(graphql.NewNonNull(graphql.String))"},
		{name: "[String!]!", sdl: "[String!]!", expected: "graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(graphql.String)))"},
		{name: "[Int!]", sdl: "[Int!]", expected: "graphql.NewList(graphql.NewNonNull(graphql.Int))"},

		// Unknown named types fall back via the CustomTypes registry.
		{name: "Upload", sdl: "Upload", expected: `customTypeOr("Upload", graphql.String)`},
		{name: "[AppAuthMethod!]", sdl: "[AppAuthMethod!]", expected: `graphql.NewList(graphql.NewNonNull(customTypeOr("AppAuthMethod", graphql.String)))`},
		{name: "[ContactWestRegion!]!", sdl: "[ContactWestRegion!]!", expected: `graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(customTypeOr("ContactWestRegion", graphql.String))))`},
		{name: "Custom!", sdl: "Custom!", expected: `graphql.NewNonNull(customTypeOr("Custom", graphql.String))`},

		// Whitespace tolerance.
		{name: "spaced [String!]", sdl: " [String!] ", expected: "graphql.NewList(graphql.NewNonNull(graphql.String))"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sdlTypeToGo(tt.sdl)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestSDLTypeToGoErrors verifies that malformed SDL type expressions are
// reported as errors rather than producing garbage Go.
func TestSDLTypeToGoErrors(t *testing.T) {
	for _, sdl := range []string{"", "[String", "String]", "[]", "!", "[!]"} {
		_, err := sdlTypeToGo(sdl)
		assert.Error(t, err, "expected error for %q", sdl)
	}
}

// TestGqlgoTypeUsesSDLTranslator verifies the integration: a JSON/Other field
// carrying a Type annotation renders a valid Go expression, not raw SDL.
func TestGqlgoTypeUsesSDLTranslator(t *testing.T) {
	f := jsonFieldWithType("[String!]")
	assert.Equal(t, "graphql.NewList(graphql.NewNonNull(graphql.String))", gqlgoType(f))

	f = jsonFieldWithType("Upload")
	assert.Equal(t, `customTypeOr("Upload", graphql.String)`, gqlgoType(f))

	f = jsonFieldWithType("[AppAuthMethod!]")
	assert.Equal(t, `graphql.NewList(graphql.NewNonNull(customTypeOr("AppAuthMethod", graphql.String)))`, gqlgoType(f))
}
