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

package entgql

import (
	"encoding/json"
	"reflect"
	"testing"
	"unsafe"

	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/entc/gen"
	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/require"
)

func TestIsUnboundedTextField_PostgresText(t *testing.T) {
	t.Parallel()
	f := makeTextSchemaTypeField("description", "text")
	require.True(t, isUnboundedTextField(f))
}

func TestIsUnboundedTextField_VarcharIsBounded(t *testing.T) {
	t.Parallel()
	f := makeTextSchemaTypeField("name", "varchar(256)")
	require.False(t, isUnboundedTextField(f))
}

func TestIsUnboundedTextField_NonStringField(t *testing.T) {
	t.Parallel()
	f := &gen.Field{Name: "amount", Type: &field.TypeInfo{Type: field.TypeFloat64}}
	require.False(t, isUnboundedTextField(f))
}

func TestIsEntSQLSkipped_EntityLevel(t *testing.T) {
	t.Parallel()
	skip, err := isEntSQLSkipped(annotationsWith(entsql.Skip()))
	require.NoError(t, err)
	require.True(t, skip)
}

func TestIsEntSQLSkipped_NotSet(t *testing.T) {
	t.Parallel()
	skip, err := isEntSQLSkipped(gen.Annotations{})
	require.NoError(t, err)
	require.False(t, skip)
}

func TestIsEntSQLSkipped_Nil(t *testing.T) {
	t.Parallel()
	skip, err := isEntSQLSkipped(nil)
	require.NoError(t, err)
	require.False(t, skip)
}

// --- test helpers ---

// annotationsWith marshals a schema.Annotation into the gen.Annotations
// shape that ent uses post-load.
func annotationsWith(ant interface {
	Name() string
}) gen.Annotations {
	buf, _ := json.Marshal(ant)
	var raw any
	_ = json.Unmarshal(buf, &raw)
	return gen.Annotations{ant.Name(): raw}
}

// makeTextSchemaTypeField creates a gen.Field with field.TypeString and a
// Postgres SchemaType set via unsafe (load.Field is unexported on gen.Field).
// Mirrors the helper in service-api-go/api-graphql/src/extensions/entorderindex/
// order_indexes_test.go (referenced in the spec).
func makeTextSchemaTypeField(name, postgresSchemaType string) *gen.Field {
	f := &gen.Field{Name: name, Type: &field.TypeInfo{Type: field.TypeString}}
	lf := &load.Field{
		Name: name,
		Info: &field.TypeInfo{Type: field.TypeString},
		SchemaType: map[string]string{
			"postgres": postgresSchemaType,
		},
	}
	rv := reflect.ValueOf(f).Elem()

	defField := rv.FieldByName("def")
	defPtr := unsafe.Pointer(defField.UnsafeAddr())
	*(**load.Field)(defPtr) = lf

	dummyType := &gen.Type{Name: "Dummy"}
	typField := rv.FieldByName("typ")
	typPtr := unsafe.Pointer(typField.UnsafeAddr())
	*(**gen.Type)(typPtr) = dummyType

	return f
}
