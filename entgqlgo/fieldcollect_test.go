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

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/stretchr/testify/assert"
)

func TestCollectFields(t *testing.T) {
	tests := []struct {
		name     string
		info     graphql.ResolveInfo
		expected []string
	}{
		{
			name: "empty selection set",
			info: graphql.ResolveInfo{
				FieldASTs: []*ast.Field{
					{
						Name:         &ast.Name{Value: "test"},
						SelectionSet: nil,
					},
				},
			},
			expected: nil,
		},
		{
			name: "single field",
			info: graphql.ResolveInfo{
				FieldASTs: []*ast.Field{
					{
						Name: &ast.Name{Value: "test"},
						SelectionSet: &ast.SelectionSet{
							Selections: []ast.Selection{
								&ast.Field{Name: &ast.Name{Value: "id"}},
							},
						},
					},
				},
			},
			expected: []string{"id"},
		},
		{
			name: "multiple fields",
			info: graphql.ResolveInfo{
				FieldASTs: []*ast.Field{
					{
						Name: &ast.Name{Value: "test"},
						SelectionSet: &ast.SelectionSet{
							Selections: []ast.Selection{
								&ast.Field{Name: &ast.Name{Value: "id"}},
								&ast.Field{Name: &ast.Name{Value: "name"}},
								&ast.Field{Name: &ast.Name{Value: "email"}},
							},
						},
					},
				},
			},
			expected: []string{"id", "name", "email"},
		},
		{
			name:     "no FieldASTs",
			info:     graphql.ResolveInfo{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CollectFields(tt.info)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasField(t *testing.T) {
	info := graphql.ResolveInfo{
		FieldASTs: []*ast.Field{
			{
				Name: &ast.Name{Value: "test"},
				SelectionSet: &ast.SelectionSet{
					Selections: []ast.Selection{
						&ast.Field{Name: &ast.Name{Value: "id"}},
						&ast.Field{Name: &ast.Name{Value: "name"}},
						&ast.Field{Name: &ast.Name{Value: "todos"}},
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{"existing field id", "id", true},
		{"existing field name", "name", true},
		{"existing field todos", "todos", true},
		{"non-existing field", "email", false},
		{"empty field", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasField(info, tt.field)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollectNestedFields(t *testing.T) {
	info := graphql.ResolveInfo{
		FieldASTs: []*ast.Field{
			{
				Name: &ast.Name{Value: "users"},
				SelectionSet: &ast.SelectionSet{
					Selections: []ast.Selection{
						&ast.Field{
							Name: &ast.Name{Value: "edges"},
							SelectionSet: &ast.SelectionSet{
								Selections: []ast.Selection{
									&ast.Field{
										Name: &ast.Name{Value: "node"},
										SelectionSet: &ast.SelectionSet{
											Selections: []ast.Selection{
												&ast.Field{Name: &ast.Name{Value: "id"}},
												&ast.Field{Name: &ast.Name{Value: "name"}},
												&ast.Field{Name: &ast.Name{Value: "email"}},
											},
										},
									},
									&ast.Field{Name: &ast.Name{Value: "cursor"}},
								},
							},
						},
						&ast.Field{Name: &ast.Name{Value: "pageInfo"}},
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		path     []string
		expected []string
	}{
		{
			name:     "root level",
			path:     nil,
			expected: []string{"edges", "pageInfo"},
		},
		{
			name:     "edges level",
			path:     []string{"edges"},
			expected: []string{"node", "cursor"},
		},
		{
			name:     "edges.node level",
			path:     []string{"edges", "node"},
			expected: []string{"id", "name", "email"},
		},
		{
			name:     "non-existing path",
			path:     []string{"nonexistent"},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CollectNestedFields(info, tt.path...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasNestedField(t *testing.T) {
	info := graphql.ResolveInfo{
		FieldASTs: []*ast.Field{
			{
				Name: &ast.Name{Value: "users"},
				SelectionSet: &ast.SelectionSet{
					Selections: []ast.Selection{
						&ast.Field{
							Name: &ast.Name{Value: "edges"},
							SelectionSet: &ast.SelectionSet{
								Selections: []ast.Selection{
									&ast.Field{
										Name: &ast.Name{Value: "node"},
										SelectionSet: &ast.SelectionSet{
											Selections: []ast.Selection{
												&ast.Field{Name: &ast.Name{Value: "id"}},
												&ast.Field{Name: &ast.Name{Value: "todos"}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		field    string
		path     []string
		expected bool
	}{
		{"has edges at root", "edges", nil, true},
		{"has node in edges", "node", []string{"edges"}, true},
		{"has id in edges.node", "id", []string{"edges", "node"}, true},
		{"has todos in edges.node", "todos", []string{"edges", "node"}, true},
		{"does not have email in edges.node", "email", []string{"edges", "node"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasNestedField(info, tt.field, tt.path...)
			assert.Equal(t, tt.expected, result)
		})
	}
}
