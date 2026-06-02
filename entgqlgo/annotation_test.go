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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotationName(t *testing.T) {
	a := Annotation{}
	assert.Equal(t, "EntGQL", a.Name())
}

func TestOrderField(t *testing.T) {
	a := OrderField("CREATED_AT", "UPDATED_AT")
	assert.Equal(t, []string{"CREATED_AT", "UPDATED_AT"}, a.OrderField)
}

func TestMultiOrder(t *testing.T) {
	a := MultiOrder()
	assert.True(t, a.MultiOrder)
}

func TestUnbind(t *testing.T) {
	a := Unbind()
	assert.True(t, a.Unbind)
}

func TestMapsTo(t *testing.T) {
	a := MapsTo("field1", "field2")
	assert.Equal(t, []string{"field1", "field2"}, a.Mapping)
	assert.True(t, a.Unbind, "MapsTo should set Unbind to true")
}

func TestType(t *testing.T) {
	a := Type("CustomType")
	assert.Equal(t, "CustomType", a.Type)
}

func TestSkip(t *testing.T) {
	tests := []struct {
		name     string
		flags    []SkipMode
		expected SkipMode
	}{
		{
			name:     "no flags defaults to SkipAll",
			flags:    nil,
			expected: SkipAll,
		},
		{
			name:     "single flag",
			flags:    []SkipMode{SkipType},
			expected: SkipType,
		},
		{
			name:     "multiple flags",
			flags:    []SkipMode{SkipType, SkipEnumField},
			expected: SkipType | SkipEnumField,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Skip(tt.flags...)
			assert.Equal(t, tt.expected, a.Skip)
		})
	}
}

func TestSkipModeIs(t *testing.T) {
	tests := []struct {
		name     string
		mode     SkipMode
		check    SkipMode
		expected bool
	}{
		{"SkipType is SkipType", SkipType, SkipType, true},
		{"SkipAll has SkipType", SkipAll, SkipType, true},
		{"SkipType is not SkipEnumField", SkipType, SkipEnumField, false},
		{"combined has SkipType", SkipType | SkipEnumField, SkipType, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.mode.Is(tt.check))
		})
	}
}

func TestSkipModeAny(t *testing.T) {
	assert.False(t, SkipMode(0).Any())
	assert.True(t, SkipType.Any())
	assert.True(t, SkipAll.Any())
}

func TestRelayConnection(t *testing.T) {
	a := RelayConnection()
	assert.True(t, a.RelayConnection)
}

func TestQueryField(t *testing.T) {
	t.Run("without name", func(t *testing.T) {
		a := QueryField()
		require.NotNil(t, a.QueryField)
		assert.Equal(t, "", a.QueryField.Name)
	})

	t.Run("with name", func(t *testing.T) {
		a := QueryField("customName")
		require.NotNil(t, a.QueryField)
		assert.Equal(t, "customName", a.QueryField.Name)
	})

	t.Run("with description", func(t *testing.T) {
		a := QueryField("name").Description("A description")
		require.NotNil(t, a.QueryField)
		assert.Equal(t, "A description", a.QueryField.Description)
	})
}

func TestMutations(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		a := Mutations()
		require.Len(t, a.MutationInputs, 2)
		assert.True(t, a.MutationInputs[0].IsCreate)
		assert.False(t, a.MutationInputs[1].IsCreate)
	})

	t.Run("create only", func(t *testing.T) {
		a := Mutations(MutationCreate())
		require.Len(t, a.MutationInputs, 1)
		assert.True(t, a.MutationInputs[0].IsCreate)
	})

	t.Run("update only", func(t *testing.T) {
		a := Mutations(MutationUpdate())
		require.Len(t, a.MutationInputs, 1)
		assert.False(t, a.MutationInputs[0].IsCreate)
	})

	t.Run("with description", func(t *testing.T) {
		a := Mutations(MutationCreate().Description("Create description"))
		require.Len(t, a.MutationInputs, 1)
		assert.Equal(t, "Create description", a.MutationInputs[0].Description)
	})
}

func TestUseEnumNames(t *testing.T) {
	a := UseEnumNames()
	assert.True(t, a.UseEnumNames)
}

func TestAnnotationMerge(t *testing.T) {
	base := Annotation{
		OrderField: []string{"FIELD_A"},
		Type:       "BaseType",
	}

	other := Annotation{
		OrderField:      []string{"FIELD_B"},
		MultiOrder:      true,
		RelayConnection: true,
	}

	merged := base.Merge(other).(Annotation)

	assert.Equal(t, []string{"FIELD_B"}, merged.OrderField, "OrderField should be overwritten")
	assert.Equal(t, "BaseType", merged.Type, "Type should remain from base")
	assert.True(t, merged.MultiOrder, "MultiOrder should be set")
	assert.True(t, merged.RelayConnection, "RelayConnection should be set")
}

func TestAnnotationDecode(t *testing.T) {
	input := map[string]interface{}{
		"OrderField":      []interface{}{"CREATED_AT"},
		"MultiOrder":      true,
		"RelayConnection": true,
		"Type":            "CustomType",
	}

	var a Annotation
	err := a.Decode(input)
	require.NoError(t, err)

	assert.Equal(t, []string{"CREATED_AT"}, a.OrderField)
	assert.True(t, a.MultiOrder)
	assert.True(t, a.RelayConnection)
	assert.Equal(t, "CustomType", a.Type)
}

func TestImplementsAnnotation(t *testing.T) {
	t.Parallel()

	a := Implements("NamedNode", "Entity")
	require.Equal(t, []string{"NamedNode", "Entity"}, a.Implements)

	// Merge accumulates.
	merged := Implements("NamedNode").Merge(Implements("Entity")).(Annotation)
	require.Equal(t, []string{"NamedNode", "Entity"}, merged.Implements)

	// JSON round-trip uses the same key as entgql for annotation compatibility.
	decoded := Annotation{}
	require.NoError(t, decoded.Decode(map[string]interface{}{
		"Implements": []interface{}{"NamedNode"},
	}))
	require.Equal(t, []string{"NamedNode"}, decoded.Implements)
}
