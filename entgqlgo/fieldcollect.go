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
	"strconv"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
)


// FieldInfo contains information about a selected field in a GraphQL query.
type FieldInfo struct {
	Name      string
	Alias     string
	Arguments map[string]interface{}
	Children  []*FieldInfo
}

// CollectFields extracts the list of selected field names from ResolveInfo.
// This is used to determine which edges should be eager-loaded.
func CollectFields(info graphql.ResolveInfo) []string {
	var fields []string
	if len(info.FieldASTs) == 0 {
		return fields
	}
	selSet := info.FieldASTs[0].SelectionSet
	if selSet == nil {
		return fields
	}
	for _, sel := range selSet.Selections {
		if field, ok := sel.(*ast.Field); ok {
			fields = append(fields, field.Name.Value)
		}
	}
	return fields
}

// HasField checks if a specific field was requested in the query.
func HasField(info graphql.ResolveInfo, name string) bool {
	for _, f := range CollectFields(info) {
		if f == name {
			return true
		}
	}
	return false
}

// CollectFieldsWithArgs extracts selected fields along with their arguments.
// This is useful for determining pagination parameters on edge connections.
func CollectFieldsWithArgs(info graphql.ResolveInfo) []*FieldInfo {
	var fields []*FieldInfo
	if len(info.FieldASTs) == 0 {
		return fields
	}
	selSet := info.FieldASTs[0].SelectionSet
	if selSet == nil {
		return fields
	}
	for _, sel := range selSet.Selections {
		if field, ok := sel.(*ast.Field); ok {
			fi := &FieldInfo{
				Name:      field.Name.Value,
				Arguments: extractArguments(field.Arguments, info.VariableValues),
			}
			if field.Alias != nil {
				fi.Alias = field.Alias.Value
			}
			if field.SelectionSet != nil {
				fi.Children = collectNestedFields(field.SelectionSet, info.VariableValues, info.Fragments)
			}
			fields = append(fields, fi)
		}
	}
	return fields
}

// GetFieldInfo returns information about a specific requested field by name.
func GetFieldInfo(info graphql.ResolveInfo, name string) *FieldInfo {
	for _, f := range CollectFieldsWithArgs(info) {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// CollectNestedFields extracts field names at a given path.
// For example, path ["edges", "node"] would return the fields selected on the node within edges.
func CollectNestedFields(info graphql.ResolveInfo, path ...string) []string {
	var fields []string
	if len(info.FieldASTs) == 0 {
		return fields
	}
	selSet := info.FieldASTs[0].SelectionSet
	for _, p := range path {
		found := false
		if selSet == nil {
			return fields
		}
		for _, sel := range selSet.Selections {
			if field, ok := sel.(*ast.Field); ok && field.Name.Value == p {
				selSet = field.SelectionSet
				found = true
				break
			}
		}
		if !found {
			return fields
		}
	}
	if selSet == nil {
		return fields
	}
	for _, sel := range selSet.Selections {
		if field, ok := sel.(*ast.Field); ok {
			fields = append(fields, field.Name.Value)
		}
	}
	return fields
}

// HasNestedField checks if a specific nested field was requested.
func HasNestedField(info graphql.ResolveInfo, name string, path ...string) bool {
	for _, f := range CollectNestedFields(info, path...) {
		if f == name {
			return true
		}
	}
	return false
}

// GetPaginationArgs extracts pagination arguments (first, last, after, before) from a field.
func GetPaginationArgs(info graphql.ResolveInfo, fieldName string) (first, last *int, after, before *string) {
	fi := GetFieldInfo(info, fieldName)
	if fi == nil {
		return
	}
	if v, ok := fi.Arguments["first"].(int); ok {
		first = &v
	}
	if v, ok := fi.Arguments["last"].(int); ok {
		last = &v
	}
	if v, ok := fi.Arguments["after"].(string); ok {
		after = &v
	}
	if v, ok := fi.Arguments["before"].(string); ok {
		before = &v
	}
	return
}

// collectNestedFields recursively collects field information from a selection set.
func collectNestedFields(selSet *ast.SelectionSet, vars map[string]interface{}, fragments map[string]ast.Definition) []*FieldInfo {
	var fields []*FieldInfo
	if selSet == nil {
		return fields
	}
	for _, sel := range selSet.Selections {
		switch s := sel.(type) {
		case *ast.Field:
			fi := &FieldInfo{
				Name:      s.Name.Value,
				Arguments: extractArguments(s.Arguments, vars),
			}
			if s.Alias != nil {
				fi.Alias = s.Alias.Value
			}
			if s.SelectionSet != nil {
				fi.Children = collectNestedFields(s.SelectionSet, vars, fragments)
			}
			fields = append(fields, fi)
		case *ast.FragmentSpread:
			if frag, ok := fragments[s.Name.Value]; ok {
				if fragDef, ok := frag.(*ast.FragmentDefinition); ok {
					fields = append(fields, collectNestedFields(fragDef.SelectionSet, vars, fragments)...)
				}
			}
		case *ast.InlineFragment:
			fields = append(fields, collectNestedFields(s.SelectionSet, vars, fragments)...)
		}
	}
	return fields
}

// extractArguments extracts argument values from AST arguments.
func extractArguments(args []*ast.Argument, vars map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for _, arg := range args {
		result[arg.Name.Value] = extractValue(arg.Value, vars)
	}
	return result
}

// extractValue extracts a value from an AST value node.
func extractValue(value ast.Value, vars map[string]interface{}) interface{} {
	switch v := value.(type) {
	case *ast.IntValue:
		if i, err := strconv.Atoi(v.Value); err == nil {
			return i
		}
		return v.Value
	case *ast.FloatValue:
		if f, err := strconv.ParseFloat(v.Value, 64); err == nil {
			return f
		}
		return v.Value
	case *ast.StringValue:
		return v.Value
	case *ast.BooleanValue:
		return v.Value
	case *ast.EnumValue:
		return v.Value
	case *ast.Variable:
		if val, ok := vars[v.Name.Value]; ok {
			return val
		}
		return nil
	case *ast.ListValue:
		var list []interface{}
		for _, item := range v.Values {
			list = append(list, extractValue(item, vars))
		}
		return list
	case *ast.ObjectValue:
		obj := make(map[string]interface{})
		for _, field := range v.Fields {
			obj[field.Name.Value] = extractValue(field.Value, vars)
		}
		return obj
	default:
		return nil
	}
}

// CollectEdgeFields returns edge field names that should be eager-loaded
// based on the selection set in the GraphQL query.
// This examines the selection for Relay connection patterns.
func CollectEdgeFields(info graphql.ResolveInfo) []string {
	var edges []string
	// Check for direct edges
	for _, f := range CollectFields(info) {
		edges = append(edges, f)
	}
	// Check for Relay connection pattern: edges -> node -> <edge>
	edgeNodeFields := CollectNestedFields(info, "edges", "node")
	edges = append(edges, edgeNodeFields...)
	return edges
}
