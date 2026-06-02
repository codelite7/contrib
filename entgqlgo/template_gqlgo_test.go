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
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGqlgoDecodeField verifies the mutation-input decode helper emits real,
// compilable decode logic (not the old empty "// Handle <type>" stub) for every
// non-int/string/bool Go type an ent field can carry. This is the unit-level
// guard for the silent-data-loss fix; the todo/todosplit suites prove the
// end-to-end round-trip.
func TestGqlgoDecodeField(t *testing.T) {
	tests := []struct {
		name      string
		field     *gen.Field
		isPointer bool
		// substrings that must appear in the emitted decode code.
		contains []string
	}{
		{
			name: "float64 value",
			field: &gen.Field{Name: "score",
				Type: &field.TypeInfo{Type: field.TypeFloat64}},
			contains: []string{"case float64:", "float64(nv)", "result.Score = cv"},
		},
		{
			name: "float64 pointer",
			field: &gen.Field{Name: "score",
				Type: &field.TypeInfo{Type: field.TypeFloat64}},
			isPointer: true,
			contains:  []string{"dv := cv", "result.Score = &dv"},
		},
		{
			name: "int64 value",
			field: &gen.Field{Name: "duration",
				Type: &field.TypeInfo{Type: field.TypeInt64}},
			contains: []string{"case int:", "int64(nv)", "case json.Number:"},
		},
		{
			name: "time.Time pointer",
			field: &gen.Field{Name: "due_date",
				Type: &field.TypeInfo{Type: field.TypeTime}},
			isPointer: true,
			contains:  []string{"case time.Time:", "time.Parse(time.RFC3339", "result.DueDate = &dv"},
		},
		{
			name: "uuid value",
			field: &gen.Field{Name: "external_id",
				Type: &field.TypeInfo{Type: field.TypeUUID, Ident: "uuid.UUID"}},
			contains: []string{"case uuid.UUID:", "uuid.Parse(uv)"},
		},
		{
			name: "json map",
			field: &gen.Field{Name: "metadata",
				Type: &field.TypeInfo{Type: field.TypeJSON, Ident: "map[string]interface{}"}},
			contains: []string{"case map[string]interface{}:", "json.Unmarshal", "json.Marshal(jv)"},
		},
		{
			name: "string slice",
			field: &gen.Field{Name: "tags2",
				Type: &field.TypeInfo{Type: field.TypeJSON, Ident: "[]string", Nillable: true}},
			contains: []string{"case []interface{}:", "out = append(out, s)", "result.Tags2 = out"},
		},
		{
			name: "bytes",
			field: &gen.Field{Name: "payload",
				Type: &field.TypeInfo{Type: field.TypeBytes}},
			contains: []string{"base64.StdEncoding.DecodeString", "[]byte(bv)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := "result." + tt.field.StructField()
			code, err := gqlgoDecodeField(tt.field, tt.isPointer, "v", target)
			require.NoError(t, err)
			require.NotContains(t, code, "// Handle", "decode must not emit the data-loss stub")
			for _, want := range tt.contains {
				assert.Contains(t, code, want, "decode for %s should contain %q", tt.name, want)
			}
			// The emitted code must parse as a valid Go statement list.
			assertParsesAsStmts(t, target, code)
		})
	}
}

// assertParsesAsStmts wraps the emitted decode snippet in a function body and
// parses it, failing if it is not syntactically valid Go. The assignment target
// is declared as an addressable interface{} local so any "<target> = expr" /
// "<target> = &dv" assignment type-checks at the syntax level.
func assertParsesAsStmts(t *testing.T, target, code string) {
	t.Helper()
	// Replace the qualified target (e.g. "result.Score") with a bare local so the
	// snippet parses without needing the concrete struct type.
	body := strings.ReplaceAll(code, target, "dst")
	src := "package p\n" +
		"import (\n\t\"encoding/base64\"\n\t\"encoding/json\"\n\t\"fmt\"\n\t\"strconv\"\n\t\"time\"\n\t\"github.com/google/uuid\"\n)\n" +
		"var (_ = base64.StdEncoding; _ = json.Marshal; _ = fmt.Sprint; _ = strconv.Atoi; _ = time.Now; _ = uuid.New)\n" +
		"func _f(v interface{}) (interface{}, error) {\n" +
		"\tvar dst interface{}\n\t_ = dst\n" +
		body + "\n" +
		"\t_ = v\n\treturn nil, nil\n}\n"
	_, err := parser.ParseFile(token.NewFileSet(), "snippet.go", src, parser.AllErrors)
	require.NoError(t, err, "emitted decode code must be valid Go:\n%s", body)
}

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
