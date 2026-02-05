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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMutationDescriptorInput(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		isCreate bool
		expected string
	}{
		{
			name:     "create input follows Create*Input convention",
			typeName: "Contact",
			isCreate: true,
			expected: "CreateContactInput",
		},
		{
			name:     "update input follows Update*Input convention",
			typeName: "Contact",
			isCreate: false,
			expected: "UpdateContactInput",
		},
		{
			name:     "create input for Todo",
			typeName: "Todo",
			isCreate: true,
			expected: "CreateTodoInput",
		},
		{
			name:     "update input for Todo",
			typeName: "Todo",
			isCreate: false,
			expected: "UpdateTodoInput",
		},
		{
			name:     "create input for Category",
			typeName: "Category",
			isCreate: true,
			expected: "CreateCategoryInput",
		},
		{
			name:     "update input for Category",
			typeName: "Category",
			isCreate: false,
			expected: "UpdateCategoryInput",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MutationDescriptor{
				Type:     &gen.Type{Name: tt.typeName},
				IsCreate: tt.isCreate,
			}
			got, err := m.Input()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestMutationDescriptorInputWithCustomType(t *testing.T) {
	m := &MutationDescriptor{
		Type: &gen.Type{
			Name: "Contact",
			Annotations: gen.Annotations{
				"EntGQL": map[string]interface{}{
					"Type": "Person",
				},
			},
		},
		IsCreate: true,
	}
	got, err := m.Input()
	require.NoError(t, err)
	assert.Equal(t, "CreatePersonInput", got, "should use the custom GQL type name")
}

func TestMutationInputNamingConvention(t *testing.T) {
	m := &MutationDescriptor{
		Type:     &gen.Type{Name: "Contact"},
		IsCreate: true,
	}
	got, err := m.Input()
	require.NoError(t, err)

	assert.NotContains(t, got, "InputGG", "should not contain InputGG suffix")
	assert.True(t, len(got) > 0 && got[:6] == "Create", "create input should start with Create")
	assert.True(t, len(got) > 5 && got[len(got)-5:] == "Input", "input should end with Input")
}
