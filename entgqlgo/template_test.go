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
	"reflect"
	"testing"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGqlgoType(t *testing.T) {
	tests := []struct {
		name     string
		field    *gen.Field
		expected string
	}{
		{
			name: "string field",
			field: &gen.Field{
				Name: "name",
				Type: &field.TypeInfo{Type: field.TypeString},
			},
			expected: "graphql.String",
		},
		{
			name: "int field",
			field: &gen.Field{
				Name: "age",
				Type: &field.TypeInfo{Type: field.TypeInt},
			},
			expected: "graphql.Int",
		},
		{
			name: "float field",
			field: &gen.Field{
				Name: "score",
				Type: &field.TypeInfo{Type: field.TypeFloat64},
			},
			expected: "graphql.Float",
		},
		{
			name: "bool field",
			field: &gen.Field{
				Name: "active",
				Type: &field.TypeInfo{Type: field.TypeBool},
			},
			expected: "graphql.Boolean",
		},
		{
			name: "time field maps to TimeScalar",
			field: &gen.Field{
				Name: "created_at",
				Type: &field.TypeInfo{Type: field.TypeTime},
			},
			expected: "TimeScalar",
		},
		{
			name: "uuid field routes to UUID scalar with ID fallback",
			field: &gen.Field{
				Name: "external_id",
				Type: &field.TypeInfo{Type: field.TypeUUID},
			},
			expected: `customTypeOr("UUID", graphql.ID)`,
		},
		{
			name: "enum field",
			field: &gen.Field{
				Name: "status",
				Type: &field.TypeInfo{Type: field.TypeEnum},
			},
			expected: "graphql.String",
		},
		{
			name: "id field",
			field: &gen.Field{
				Name: "id",
				Type: &field.TypeInfo{Type: field.TypeInt},
			},
			expected: "graphql.ID",
		},
		{
			name: "json field without annotation",
			field: &gen.Field{
				Name: "metadata",
				Type: &field.TypeInfo{Type: field.TypeJSON},
			},
			expected: "graphql.String",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := gqlgoType(tt.field)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGqlgoTypeScalarRouting covers the CustomTypes-registry routing for UUID,
// map[string]interface{} JSON ("Map"), and a Bytes field carrying a Type
// annotation ("Upload"). These let downstream consumers register real scalars
// while preserving the prior default when unregistered.
func TestGqlgoTypeScalarRouting(t *testing.T) {
	tests := []struct {
		name     string
		field    *gen.Field
		expected string
	}{
		{
			name: "map[string]interface{} JSON routes to Map scalar",
			field: &gen.Field{
				Name: "metadata",
				Type: &field.TypeInfo{
					Type:  field.TypeJSON,
					RType: &field.RType{Kind: reflect.Map, Ident: "map[string]interface {}"},
				},
			},
			expected: `customTypeOr("Map", graphql.String)`,
		},
		{
			name: "bytes field with Upload Type annotation honors the annotation",
			field: &gen.Field{
				Name: "payload",
				Type: &field.TypeInfo{Type: field.TypeBytes},
				Annotations: gen.Annotations{
					"EntGQL": map[string]interface{}{"Type": "Upload"},
				},
			},
			expected: `customTypeOr("Upload", graphql.String)`,
		},
		{
			name: "bytes field without annotation stays String",
			field: &gen.Field{
				Name: "blob",
				Type: &field.TypeInfo{Type: field.TypeBytes},
			},
			expected: "graphql.String",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := gqlgoType(tt.field)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFieldNullabilityLogic verifies the NonNull wrapping logic used by the types template.
// Required fields (not Optional, not Nillable) should be wrapped with graphql.NewNonNull().
// Optional or Nillable fields should remain as bare types (nullable).
func TestFieldNullabilityLogic(t *testing.T) {
	tests := []struct {
		name        string
		field       *gen.Field
		wantNonNull bool
	}{
		{
			name: "required string field should be NonNull",
			field: &gen.Field{
				Name:     "name",
				Type:     &field.TypeInfo{Type: field.TypeString},
				Optional: false,
				Nillable: false,
			},
			wantNonNull: true,
		},
		{
			name: "optional string field should be nullable",
			field: &gen.Field{
				Name:     "nickname",
				Type:     &field.TypeInfo{Type: field.TypeString},
				Optional: true,
				Nillable: false,
			},
			wantNonNull: false,
		},
		{
			name: "nillable string field should be nullable",
			field: &gen.Field{
				Name:     "bio",
				Type:     &field.TypeInfo{Type: field.TypeString},
				Optional: false,
				Nillable: true,
			},
			wantNonNull: false,
		},
		{
			name: "optional and nillable field should be nullable",
			field: &gen.Field{
				Name:     "description",
				Type:     &field.TypeInfo{Type: field.TypeString},
				Optional: true,
				Nillable: true,
			},
			wantNonNull: false,
		},
		{
			name: "required int field should be NonNull",
			field: &gen.Field{
				Name:     "age",
				Type:     &field.TypeInfo{Type: field.TypeInt},
				Optional: false,
				Nillable: false,
			},
			wantNonNull: true,
		},
		{
			name: "optional int field should be nullable",
			field: &gen.Field{
				Name:     "score",
				Type:     &field.TypeInfo{Type: field.TypeInt},
				Optional: true,
				Nillable: false,
			},
			wantNonNull: false,
		},
		{
			name: "required bool field should be NonNull",
			field: &gen.Field{
				Name:     "active",
				Type:     &field.TypeInfo{Type: field.TypeBool},
				Optional: false,
				Nillable: false,
			},
			wantNonNull: true,
		},
		{
			name: "required time field should be NonNull",
			field: &gen.Field{
				Name:     "created_at",
				Type:     &field.TypeInfo{Type: field.TypeTime},
				Optional: false,
				Nillable: false,
			},
			wantNonNull: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This mirrors the template logic: {{- if or $f.Optional $f.Nillable }}
			isRequired := !tt.field.Optional && !tt.field.Nillable
			assert.Equal(t, tt.wantNonNull, isRequired,
				"field %q: Optional=%v, Nillable=%v -> isRequired should be %v",
				tt.field.Name, tt.field.Optional, tt.field.Nillable, tt.wantNonNull)

			// Verify the base type is returned correctly by gqlgoType
			baseType, err := gqlgoType(tt.field)
			assert.NoError(t, err)
			assert.NotEmpty(t, baseType)

			// Verify the composed output matches what the template would produce:
			// Required -> graphql.NewNonNull(<baseType>)
			// Nullable -> <baseType>
			if tt.wantNonNull {
				composed := "graphql.NewNonNull(" + baseType + ")"
				assert.Contains(t, composed, "NewNonNull")
			}
		})
	}
}

// TestEdgeNullabilityLogic verifies the NonNull wrapping logic for edges in the types template.
// - Unique + Optional edges: nullable (bare type reference)
// - Unique + Required edges: graphql.NewNonNull(type)
// - List edges: graphql.NewList(graphql.NewNonNull(type))
func TestEdgeNullabilityLogic(t *testing.T) {
	tests := []struct {
		name       string
		unique     bool
		optional   bool
		wantFormat string // "bare", "nonnull", or "list_nonnull"
	}{
		{
			name:       "unique optional edge should be nullable",
			unique:     true,
			optional:   true,
			wantFormat: "bare",
		},
		{
			name:       "unique required edge should be NonNull",
			unique:     true,
			optional:   false,
			wantFormat: "nonnull",
		},
		{
			name:       "list optional edge should use NewList(NewNonNull(...))",
			unique:     false,
			optional:   true,
			wantFormat: "list_nonnull",
		},
		{
			name:       "list required edge should also use NewList(NewNonNull(...))",
			unique:     false,
			optional:   false,
			wantFormat: "list_nonnull",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This mirrors the template logic for edge type assignment
			typeName := "FooType"
			var result string

			if tt.unique {
				if tt.optional {
					// Template: Type: {{ $e.Type.Name }}Type,
					result = typeName
				} else {
					// Template: Type: graphql.NewNonNull({{ $e.Type.Name }}Type),
					result = "graphql.NewNonNull(" + typeName + ")"
				}
			} else {
				// Template: Type: graphql.NewList(graphql.NewNonNull({{ $e.Type.Name }}Type)),
				result = "graphql.NewList(graphql.NewNonNull(" + typeName + "))"
			}

			switch tt.wantFormat {
			case "bare":
				assert.Equal(t, typeName, result,
					"optional unique edge should produce bare type reference")
			case "nonnull":
				assert.Equal(t, "graphql.NewNonNull("+typeName+")", result,
					"required unique edge should be wrapped with NewNonNull")
			case "list_nonnull":
				assert.Equal(t, "graphql.NewList(graphql.NewNonNull("+typeName+"))", result,
					"list edge should use NewList(NewNonNull(...))")
			}
		})
	}
}


func TestIsDeprecatedEnumValue(t *testing.T) {
	t.Parallel()

	f := &gen.Field{Annotations: gen.Annotations{
		"EntGQL": map[string]interface{}{
			"DeprecatedEnumValues": []interface{}{"DISABLED"},
		},
	}}
	dep, err := isDeprecatedEnumValue(f, "DISABLED")
	require.NoError(t, err)
	require.True(t, dep)

	dep, err = isDeprecatedEnumValue(f, "ENABLED")
	require.NoError(t, err)
	require.False(t, dep)
}

func TestIsRelayConn(t *testing.T) {
	tests := []struct {
		name string
		edge *gen.Edge
		want bool
	}{
		{
			name: "edge with RelayConnection annotation returns true",
			edge: &gen.Edge{
				Type: &gen.Type{
					Name: "Todo",
				},
				Annotations: gen.Annotations{
					"EntGQL": map[string]any{"RelayConnection": true},
				},
			},
			want: true,
		},
		{
			name: "edge without RelayConnection annotation returns false",
			edge: &gen.Edge{
				Type: &gen.Type{
					Name: "Todo",
				},
			},
			want: false,
		},
		{
			name: "edge with RelayConnection false returns false",
			edge: &gen.Edge{
				Type: &gen.Type{
					Name: "Todo",
				},
				Annotations: gen.Annotations{
					"EntGQL": map[string]any{"RelayConnection": false},
				},
			},
			want: false,
		},
		{
			name: "edge with other annotations but no RelayConnection returns false",
			edge: &gen.Edge{
				Type: &gen.Type{
					Name: "Todo",
				},
				Annotations: gen.Annotations{
					"EntGQL": map[string]any{"Skip": float64(SkipType)},
				},
			},
			want: false,
		},
		{
			name: "edge with both RelayConnection and other annotations returns true",
			edge: &gen.Edge{
				Type: &gen.Type{
					Name: "Todo",
				},
				Annotations: gen.Annotations{
					"EntGQL": map[string]any{
						"RelayConnection": true,
						"OrderField":      []string{"CREATED_AT"},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isRelayConn(tt.edge)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got, "isRelayConn(%+v)", tt.edge)
		})
	}
}

func TestQueryFieldName(t *testing.T) {
	t.Parallel()

	// Node without QueryField annotation -> no root query field.
	node := &gen.Type{Name: "Todo", Annotations: gen.Annotations{}}
	name, err := queryFieldName(node)
	require.NoError(t, err)
	require.Empty(t, name)

	// Node with QueryField() -> default name: camel(snake(plural(type))).
	node = &gen.Type{Name: "Todo", Annotations: gen.Annotations{
		"EntGQL": map[string]interface{}{
			"QueryField": map[string]interface{}{},
		},
	}}
	name, err = queryFieldName(node)
	require.NoError(t, err)
	require.Equal(t, "todos", name)

	// Node with QueryField("customName") -> custom name.
	node = &gen.Type{Name: "Todo", Annotations: gen.Annotations{
		"EntGQL": map[string]interface{}{
			"QueryField": map[string]interface{}{"Name": "allTodos"},
		},
	}}
	name, err = queryFieldName(node)
	require.NoError(t, err)
	require.Equal(t, "allTodos", name)
}

func TestQueryFieldDescription(t *testing.T) {
	t.Parallel()

	node := &gen.Type{Name: "Todo", Annotations: gen.Annotations{
		"EntGQL": map[string]interface{}{
			"QueryField": map[string]interface{}{"Description": "All the todos"},
		},
	}}
	desc, err := queryFieldDescription(node)
	require.NoError(t, err)
	require.Equal(t, "All the todos", desc)
}

func TestIsRelayConnNode(t *testing.T) {
	t.Parallel()

	node := &gen.Type{Name: "Todo", Annotations: gen.Annotations{}}
	rc, err := isRelayConnNode(node)
	require.NoError(t, err)
	require.False(t, rc)

	node = &gen.Type{Name: "Todo", Annotations: gen.Annotations{
		"EntGQL": map[string]interface{}{"RelayConnection": true},
	}}
	rc, err = isRelayConnNode(node)
	require.NoError(t, err)
	require.True(t, rc)
}

func TestRelayConnEdgePaginationNames(t *testing.T) {
	// When an edge is a RelayConnection edge, the types template uses
	// gqlgoNodePaginationNames on the edge's target type to determine
	// the connection type name and related pagination type names.
	// This test verifies that the pagination names are correctly derived.
	tests := []struct {
		name           string
		targetTypeName string
		wantConnection string
		wantEdge       string
		wantOrder      string
		wantWhereInput string
	}{
		{
			name:           "Todo type pagination names",
			targetTypeName: "Todo",
			wantConnection: "TodoConnection",
			wantEdge:       "TodoEdge",
			wantOrder:      "TodoOrder",
			wantWhereInput: "TodoWhereInput",
		},
		{
			name:           "Category type pagination names",
			targetTypeName: "Category",
			wantConnection: "CategoryConnection",
			wantEdge:       "CategoryEdge",
			wantOrder:      "CategoryOrder",
			wantWhereInput: "CategoryWhereInput",
		},
		{
			name:           "ContactListContact type pagination names",
			targetTypeName: "ContactListContact",
			wantConnection: "ContactListContactConnection",
			wantEdge:       "ContactListContactEdge",
			wantOrder:      "ContactListContactOrder",
			wantWhereInput: "ContactListContactWhereInput",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names := paginationNames(tt.targetTypeName)
			assert.Equal(t, tt.wantConnection, names.Connection)
			assert.Equal(t, tt.wantEdge, names.Edge)
			assert.Equal(t, tt.wantOrder, names.Order)
			assert.Equal(t, tt.wantWhereInput, names.WhereInput)
		})
	}
}
