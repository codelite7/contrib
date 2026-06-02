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

// newField returns a *gen.Field whose StructField() resolves to the pascal-cased
// struct field name derived from its (snake_case) schema name.
func newField(dbName string) *gen.Field {
	return &gen.Field{
		Name: dbName,
		Type: &field.TypeInfo{Type: field.TypeString},
	}
}

func newEdge(name string, unique bool) *gen.Edge {
	return &gen.Edge{
		Name:   name,
		Unique: unique,
		Type:   &gen.Type{Name: pascal(name)},
	}
}

func TestMutationSetField(t *testing.T) {
	f := &InputFieldDescriptor{Field: newField("status")}
	assert.Equal(t, `m.SetStatus(*v)`, mutationSetField(false, f, "*v"))
	assert.Equal(t, `_ = m.SetField("status", *v)`, mutationSetField(true, f, "*v"))

	f2 := &InputFieldDescriptor{Field: newField("created_at")}
	assert.Equal(t, `m.SetCreatedAt(i.CreatedAt)`, mutationSetField(false, f2, "i.CreatedAt"))
	assert.Equal(t, `_ = m.SetField("created_at", i.CreatedAt)`, mutationSetField(true, f2, "i.CreatedAt"))
}

func TestMutationClearField(t *testing.T) {
	f := &InputFieldDescriptor{Field: newField("priority")}
	assert.Equal(t, `m.ClearPriority()`, mutationClearField(false, f))
	assert.Equal(t, `_ = m.ClearField("priority")`, mutationClearField(true, f))
}

func TestMutationAppendField(t *testing.T) {
	f := &InputFieldDescriptor{Field: newField("tags")}
	assert.Equal(t, `m.AppendTags(i.Tags)`, mutationAppendField(false, f, "i.Tags"))
	assert.Equal(t, `_ = m.AppendField("tags", i.Tags)`, mutationAppendField(true, f, "i.Tags"))
}

func TestMutationSetEdgeID(t *testing.T) {
	e := newEdge("category", true)
	assert.Equal(t, `m.SetCategoryID(*v)`, mutationSetEdgeID(false, e, "*v"))
	assert.Equal(t, `_ = m.SetEdgeID("category", *v)`, mutationSetEdgeID(true, e, "*v"))
}

func TestMutationAddEdgeIDs(t *testing.T) {
	e := newEdge("children", false)
	assert.Equal(t, `m.AddChildIDs(v...)`, mutationAddEdgeIDs(false, e, "v"))
	assert.Equal(t, `_ = m.AddEdgeIDs("children", entbuilder.ToAny(v)...)`, mutationAddEdgeIDs(true, e, "v"))
}

func TestMutationRemoveEdgeIDs(t *testing.T) {
	e := newEdge("children", false)
	assert.Equal(t, `m.RemoveChildIDs(v...)`, mutationRemoveEdgeIDs(false, e, "v"))
	assert.Equal(t, `_ = m.RemoveEdgeIDs("children", entbuilder.ToAny(v)...)`, mutationRemoveEdgeIDs(true, e, "v"))
}

func TestMutationClearEdge(t *testing.T) {
	e := newEdge("category", true)
	assert.Equal(t, `m.ClearCategory()`, mutationClearEdge(false, e))
	assert.Equal(t, `_ = m.ClearEdge("category")`, mutationClearEdge(true, e))
}

func TestGqlgoDeref(t *testing.T) {
	assert.Equal(t, "*", gqlgoDeref(true))
	assert.Equal(t, "", gqlgoDeref(false))
}

func TestGqlgoSplitRuntimeReadsGraphAnnotation(t *testing.T) {
	name := ExtensionAnnotation{}.Name()

	graphWith := func(ant any) *gen.Graph {
		g := &gen.Graph{Config: &gen.Config{}}
		if ant != nil {
			g.Annotations = gen.Annotations{name: ant}
		}
		return g
	}

	// Nil graph / nil-config / nil annotations default to classic (false).
	assert.False(t, gqlgoSplitRuntime(nil))
	assert.False(t, gqlgoSplitRuntime(&gen.Graph{}))
	assert.False(t, gqlgoSplitRuntime(graphWith(nil)))

	// Struct-valued annotation (in-process hook injection).
	assert.False(t, gqlgoSplitRuntime(graphWith(ExtensionAnnotation{SplitRuntime: false})))
	assert.True(t, gqlgoSplitRuntime(graphWith(ExtensionAnnotation{SplitRuntime: true})))

	// Map-valued annotation (JSON round-tripped shape) is handled too.
	assert.True(t, gqlgoSplitRuntime(graphWith(map[string]any{"SplitRuntime": true})))
	assert.False(t, gqlgoSplitRuntime(graphWith(map[string]any{"SplitRuntime": false})))
}

func TestGqlgoPascalMutationsReadsGraphAnnotation(t *testing.T) {
	name := ExtensionAnnotation{}.Name()

	graphWith := func(ant any) *gen.Graph {
		g := &gen.Graph{Config: &gen.Config{}}
		if ant != nil {
			g.Annotations = gen.Annotations{name: ant}
		}
		return g
	}

	// Nil graph / nil-config / nil annotations default to camelCase (false).
	assert.False(t, gqlgoPascalMutations(nil))
	assert.False(t, gqlgoPascalMutations(&gen.Graph{}))
	assert.False(t, gqlgoPascalMutations(graphWith(nil)))

	// Struct-valued annotation (in-process hook injection).
	assert.False(t, gqlgoPascalMutations(graphWith(ExtensionAnnotation{PascalMutationNames: false})))
	assert.True(t, gqlgoPascalMutations(graphWith(ExtensionAnnotation{PascalMutationNames: true})))

	// Map-valued annotation (JSON round-tripped shape) is handled too.
	assert.True(t, gqlgoPascalMutations(graphWith(map[string]any{"PascalMutationNames": true})))
	assert.False(t, gqlgoPascalMutations(graphWith(map[string]any{"PascalMutationNames": false})))
}
