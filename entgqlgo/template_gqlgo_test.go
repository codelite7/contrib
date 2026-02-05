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

func TestGqlgoType_TypeMapping(t *testing.T) {
	tests := []struct {
		name     string
		field    *gen.Field
		expected string
	}{
		{
			name: "time field returns TimeScalar",
			field: &gen.Field{
				Name: "created_at",
				Type: &field.TypeInfo{Type: field.TypeTime},
			},
			expected: "TimeScalar",
		},
		{
			name: "string field returns graphql.String",
			field: &gen.Field{
				Name: "name",
				Type: &field.TypeInfo{Type: field.TypeString},
			},
			expected: "graphql.String",
		},
		{
			name: "bool field returns graphql.Boolean",
			field: &gen.Field{
				Name: "active",
				Type: &field.TypeInfo{Type: field.TypeBool},
			},
			expected: "graphql.Boolean",
		},
		{
			name: "int field returns graphql.Int",
			field: &gen.Field{
				Name: "age",
				Type: &field.TypeInfo{Type: field.TypeInt},
			},
			expected: "graphql.Int",
		},
		{
			name: "float field returns graphql.Float",
			field: &gen.Field{
				Name: "score",
				Type: &field.TypeInfo{Type: field.TypeFloat64},
			},
			expected: "graphql.Float",
		},
		{
			name: "id field returns graphql.ID",
			field: &gen.Field{
				Name: "id",
				Type: &field.TypeInfo{Type: field.TypeInt},
			},
			expected: "graphql.ID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gqlgoType(tt.field)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGqlgoScalar_TypeMapping(t *testing.T) {
	tests := []struct {
		name     string
		field    *gen.Field
		expected string
	}{
		{
			name: "time field returns Time not DateTime",
			field: &gen.Field{
				Name: "created_at",
				Type: &field.TypeInfo{Type: field.TypeTime},
			},
			expected: "Time",
		},
		{
			name: "string field returns String",
			field: &gen.Field{
				Name: "name",
				Type: &field.TypeInfo{Type: field.TypeString},
			},
			expected: "String",
		},
		{
			name: "bool field returns Boolean",
			field: &gen.Field{
				Name: "active",
				Type: &field.TypeInfo{Type: field.TypeBool},
			},
			expected: "Boolean",
		},
		{
			name: "int field returns Int",
			field: &gen.Field{
				Name: "age",
				Type: &field.TypeInfo{Type: field.TypeInt},
			},
			expected: "Int",
		},
		{
			name: "float field returns Float",
			field: &gen.Field{
				Name: "score",
				Type: &field.TypeInfo{Type: field.TypeFloat64},
			},
			expected: "Float",
		},
		{
			name: "id field returns ID",
			field: &gen.Field{
				Name: "id",
				Type: &field.TypeInfo{Type: field.TypeInt},
			},
			expected: "ID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gqlgoScalar(tt.field)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTimeScalarNameIsTime verifies the scalar is named "Time" (matching gqlgen)
// and not "DateTime". This is critical for introspection parity between gqlgen
// and graphql-go server implementations.
func TestTimeScalarNameIsTime(t *testing.T) {
	timeField := &gen.Field{
		Name: "created_at",
		Type: &field.TypeInfo{Type: field.TypeTime},
	}

	// gqlgoType should return "TimeScalar" (our custom scalar),
	// not "graphql.DateTime" (the graphql-go built-in).
	assert.Equal(t, "TimeScalar", gqlgoType(timeField),
		"gqlgoType should return TimeScalar, not graphql.DateTime")

	// gqlgoScalar should return "Time" to match gqlgen convention.
	assert.Equal(t, "Time", gqlgoScalar(timeField),
		"gqlgoScalar should return Time, not DateTime")
}
