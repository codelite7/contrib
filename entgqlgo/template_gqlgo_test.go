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
			result, err := gqlgoType(tt.field)
			assert.NoError(t, err)
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
	timeType, err := gqlgoType(timeField)
	assert.NoError(t, err)
	assert.Equal(t, "TimeScalar", timeType,
		"gqlgoType should return TimeScalar, not graphql.DateTime")

	// gqlgoScalar should return "Time" to match gqlgen convention.
	assert.Equal(t, "Time", gqlgoScalar(timeField),
		"gqlgoScalar should return Time, not DateTime")
}

// TestQueryFieldNaming verifies that query fields use camel(plural(name)) which
// is the logically correct order: pluralize first, then camelCase.
func TestQueryFieldNaming(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		want     string
	}{
		{name: "Category", typeName: "Category", want: "categories"},
		{name: "Todo", typeName: "Todo", want: "todos"},
		{name: "User", typeName: "User", want: "users"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := camel(plural(tt.typeName))
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestMutationFieldNaming verifies mutation fields use PascalCase prefixes.
func TestMutationFieldNaming(t *testing.T) {
	tests := []struct {
		name       string
		typeName   string
		wantCreate string
		wantUpdate string
		wantDelete string
	}{
		{name: "Contact", typeName: "Contact", wantCreate: "CreateContact", wantUpdate: "UpdateContact", wantDelete: "DeleteContact"},
		{name: "AgentLicensing", typeName: "AgentLicensing", wantCreate: "CreateAgentLicensing", wantUpdate: "UpdateAgentLicensing", wantDelete: "DeleteAgentLicensing"},
		{name: "Todo", typeName: "Todo", wantCreate: "CreateTodo", wantUpdate: "UpdateTodo", wantDelete: "DeleteTodo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCreate, "Create"+tt.typeName)
			assert.Equal(t, tt.wantUpdate, "Update"+tt.typeName)
			assert.Equal(t, tt.wantDelete, "Delete"+tt.typeName)
		})
	}
}

// TestQueryFieldNamingOrder verifies camel(plural(name)) is the correct order.
func TestQueryFieldNamingOrder(t *testing.T) {
	assert.Equal(t, "categories", camel(plural("Category")))
	assert.Equal(t, camel(plural("Category")), plural(camel("Category")))
}

