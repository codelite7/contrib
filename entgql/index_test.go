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

// --- buildIndexConfig walker tests ---

func TestBuildIndexConfig_BasicOrderField(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension(WithIndexTableNameStrip("_view$"))
	require.NoError(t, err)

	g := &gen.Graph{
		Nodes: []*gen.Type{
			makeIndexNode("Escrow", "escrows_view", true,
				makeOrderFieldGen("created_at", "CREATED_AT"),
			),
		},
	}

	cfg, err := buildIndexConfig(g, ex)
	require.NoError(t, err)
	require.Len(t, cfg.Tables, 1)
	require.Equal(t, "escrows", cfg.Tables[0].Name)
	require.Len(t, cfg.Tables[0].Indexes, 1)
	require.Equal(t, "idx_order_escrows_created_at_id", cfg.Tables[0].Indexes[0].Name)
	require.Equal(t, "created_at", cfg.Tables[0].Indexes[0].Field)
	require.Equal(t, "deleted_at IS NULL", cfg.Tables[0].Indexes[0].Where)
	require.Empty(t, cfg.Tables[0].Indexes[0].Expression)
}

func TestBuildIndexConfig_NoSoftDelete_NoPartial(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension()
	require.NoError(t, err)

	g := &gen.Graph{
		Nodes: []*gen.Type{
			makeIndexNode("SyncPosition", "sync_positions", false,
				makeOrderFieldGen("name", "NAME"),
			),
		},
	}

	cfg, err := buildIndexConfig(g, ex)
	require.NoError(t, err)
	require.Len(t, cfg.Tables, 1)
	require.Empty(t, cfg.Tables[0].Indexes[0].Where)
}

func TestBuildIndexConfig_SkipsIDAndSoftDeleteField(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension()
	require.NoError(t, err)

	g := &gen.Graph{
		Nodes: []*gen.Type{
			makeIndexNode("Escrow", "escrows", true,
				makeOrderFieldGen("id", "ID"),
				makeOrderFieldGen("deleted_at", "DELETED_AT"),
				makeOrderFieldGen("created_at", "CREATED_AT"),
			),
		},
	}

	cfg, err := buildIndexConfig(g, ex)
	require.NoError(t, err)
	require.Len(t, cfg.Tables[0].Indexes, 1)
	require.Equal(t, "created_at", cfg.Tables[0].Indexes[0].Field)
}

func TestBuildIndexConfig_SkipsNonOrderField(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension()
	require.NoError(t, err)

	g := &gen.Graph{
		Nodes: []*gen.Type{
			makeIndexNode("Escrow", "escrows", false,
				&gen.Field{Name: "name"}, // no OrderField annotation
			),
		},
	}

	cfg, err := buildIndexConfig(g, ex)
	require.NoError(t, err)
	require.Empty(t, cfg.Tables)
}

func TestBuildIndexConfig_SkipIndexOrder_Excludes(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension()
	require.NoError(t, err)

	g := &gen.Graph{
		Nodes: []*gen.Type{
			makeIndexNode("Escrow", "escrows", false,
				makeOrderFieldGenWith("commission", "COMMISSION", SkipIndex(SkipIndexOrder)),
				makeOrderFieldGen("created_at", "CREATED_AT"),
			),
		},
	}

	cfg, err := buildIndexConfig(g, ex)
	require.NoError(t, err)
	require.Len(t, cfg.Tables[0].Indexes, 1)
	require.Equal(t, "created_at", cfg.Tables[0].Indexes[0].Field)
}

func TestBuildIndexConfig_EntityLevelEntSQLSkip_Excluded(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension()
	require.NoError(t, err)

	node := makeIndexNode("DeletedRow", "deleted_rows", false,
		makeOrderFieldGen("created_at", "CREATED_AT"),
	)
	for k, v := range annotationsWith(entsql.Skip()) {
		node.Annotations[k] = v
	}

	g := &gen.Graph{Nodes: []*gen.Type{node}}

	cfg, err := buildIndexConfig(g, ex)
	require.NoError(t, err)
	require.Empty(t, cfg.Tables)
}

func TestBuildIndexConfig_FieldLevelEntSQLSkip_Excluded(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension()
	require.NoError(t, err)

	skippedField := makeOrderFieldGen("commission", "COMMISSION")
	for k, v := range annotationsWith(entsql.Skip()) {
		skippedField.Annotations[k] = v
	}

	g := &gen.Graph{
		Nodes: []*gen.Type{
			makeIndexNode("CoBroker", "co_brokers", false,
				skippedField,
				makeOrderFieldGen("created_at", "CREATED_AT"),
			),
		},
	}

	cfg, err := buildIndexConfig(g, ex)
	require.NoError(t, err)
	require.Len(t, cfg.Tables[0].Indexes, 1)
	require.Equal(t, "created_at", cfg.Tables[0].Indexes[0].Field)
}

func TestBuildIndexConfig_DeterministicSort(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension()
	require.NoError(t, err)

	g := &gen.Graph{
		Nodes: []*gen.Type{
			makeIndexNode("Zebra", "zebras", false,
				makeOrderFieldGen("name", "NAME"),
			),
			makeIndexNode("Alpha", "alphas", false,
				makeOrderFieldGen("z_col", "Z_COL"),
				makeOrderFieldGen("a_col", "A_COL"),
			),
		},
	}

	cfg, err := buildIndexConfig(g, ex)
	require.NoError(t, err)
	require.Len(t, cfg.Tables, 2)
	require.Equal(t, "alphas", cfg.Tables[0].Name)
	require.Equal(t, "zebras", cfg.Tables[1].Name)
	require.Equal(t, "a_col", cfg.Tables[0].Indexes[0].Field)
	require.Equal(t, "z_col", cfg.Tables[0].Indexes[1].Field)
}

func TestBuildIndexConfig_CustomSoftDeleteColumn(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension(WithIndexSoftDeleteColumn("removed_at"))
	require.NoError(t, err)

	g := &gen.Graph{
		Nodes: []*gen.Type{
			makeIndexNodeWithColumn("Item", "items",
				&gen.Field{Name: "removed_at"},
				makeOrderFieldGen("created_at", "CREATED_AT"),
			),
		},
	}

	cfg, err := buildIndexConfig(g, ex)
	require.NoError(t, err)
	require.Equal(t, "removed_at IS NULL", cfg.Tables[0].Indexes[0].Where)
}

func TestBuildIndexConfig_DisabledSoftDelete_NoPartial(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension(WithIndexSoftDeleteColumn(""))
	require.NoError(t, err)

	g := &gen.Graph{
		Nodes: []*gen.Type{
			makeIndexNode("Escrow", "escrows", true, // has deleted_at, but option is off
				makeOrderFieldGen("created_at", "CREATED_AT"),
			),
		},
	}

	cfg, err := buildIndexConfig(g, ex)
	require.NoError(t, err)
	require.Empty(t, cfg.Tables[0].Indexes[0].Where)
}

// --- more test helpers ---

func makeIndexNode(name, table string, hasSoftDelete bool, fields ...*gen.Field) *gen.Type {
	node := &gen.Type{Name: name, Fields: fields}
	if hasSoftDelete {
		node.Fields = append(node.Fields, &gen.Field{Name: "deleted_at"})
	}
	node.Annotations = gen.Annotations{
		"EntSQL": map[string]any{"table": table},
	}
	return node
}

// makeIndexNodeWithColumn is like makeIndexNode but lets the caller supply an
// extra plain (non-OrderField) field — used when the soft-delete column is
// renamed and we need to verify presence-based detection.
func makeIndexNodeWithColumn(name, table string, extra *gen.Field, fields ...*gen.Field) *gen.Type {
	node := &gen.Type{Name: name, Fields: append(fields, extra)}
	node.Annotations = gen.Annotations{
		"EntSQL": map[string]any{"table": table},
	}
	return node
}

func makeOrderFieldGen(name, orderFieldName string) *gen.Field {
	gqlAnt := Annotation{OrderField: []string{orderFieldName}}
	buf, _ := json.Marshal(gqlAnt)
	var raw any
	_ = json.Unmarshal(buf, &raw)
	return &gen.Field{
		Name:        name,
		Annotations: gen.Annotations{gqlAnt.Name(): raw},
	}
}

func makeOrderFieldGenWith(name, orderFieldName string, extra Annotation) *gen.Field {
	gqlAnt := Annotation{
		OrderField: []string{orderFieldName},
		SkipIndex:  extra.SkipIndex,
	}
	buf, _ := json.Marshal(gqlAnt)
	var raw any
	_ = json.Unmarshal(buf, &raw)
	return &gen.Field{
		Name:        name,
		Annotations: gen.Annotations{gqlAnt.Name(): raw},
	}
}
