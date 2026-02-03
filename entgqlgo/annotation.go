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
	"encoding/json"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema"
)

type (
	// Annotation annotates fields and edges with metadata for templates.
	Annotation struct {
		// OrderField is the ordering field as defined in graphql schema.
		OrderField []string `json:"OrderField,omitempty"`
		// MultiOrder indicates that orderBy should accept a list of OrderField terms.
		MultiOrder bool `json:"MultiOrder,omitempty"`
		// Unbind implies the edge field name in GraphQL schema is not equivalent
		// to the name used in ent schema. That means, by default, edges with this
		// annotation will not be eager-loaded on Paginate calls. See the `MapsTo`
		// option in order to load edges be different name mapping.
		Unbind bool `json:"Unbind,omitempty"`
		// Mapping is the edge field names as defined in graphql schema.
		Mapping []string `json:"Mapping,omitempty"`
		// Type is the underlying GraphQL type name (e.g. Boolean).
		Type string `json:"Type,omitempty"`
		// Skip exclude the type
		Skip SkipMode `json:"Skip,omitempty"`
		// RelayConnection enables the Relay Connection specification for the entity.
		// It's also can apply on an edge to create the Relay-style filter.
		RelayConnection bool `json:"RelayConnection,omitempty"`
		// QueryField exposes the generated type with the given string under the Query object.
		QueryField *FieldConfig `json:"QueryField,omitempty"`
		// MutationInputs defines the input types for the mutation.
		MutationInputs []MutationConfig `json:"MutationInputs,omitempty"`
		// UseEnumNames can be used on `Enum` fields that use the `NamedValues` function to specify values.
		// when true, the graphql enums will use the Name instead of the value. This is useful when the value
		// is something that is not a valid graphql enum. The value will be added to the description of the
		// resulting enum in the graphql schema.
		UseEnumNames bool `json:"UseEnumNames,omitempty"`
	}

	// SkipMode is a bit flag for the Skip annotation.
	SkipMode int

	FieldConfig struct {
		// Name is the name of the field in the Query object.
		Name string `json:"Name,omitempty"`

		// Description is the description of the field.
		Description string `json:"Description,omitempty"`
	}

	// MutationConfig hold config for mutation
	MutationConfig struct {
		IsCreate    bool   `json:"IsCreate,omitempty"`
		Description string `json:"Description,omitempty"`
	}
)

const (
	// SkipType skips generating GraphQL types or fields in the schema.
	SkipType SkipMode = 1 << iota
	// SkipEnumField skips generating GraphQL enums for enum fields in the schema.
	SkipEnumField
	// SkipOrderField skips generating GraphQL order inputs and enums for ordered-fields in the schema.
	SkipOrderField
	// SkipWhereInput skips generating GraphQL WhereInput types.
	// If defined on a field, the type will be generated without the field.
	SkipWhereInput
	// SkipMutationCreateInput skips generating GraphQL Create<Type>Input types.
	// If defined on a field, the type will be generated without the field.
	SkipMutationCreateInput
	// SkipMutationUpdateInput skips generating GraphQL Update<Type>Input types.
	// If defined on a field, the type will be generated without the field.
	SkipMutationUpdateInput

	// SkipAll is default mode to skip all.
	SkipAll = SkipType |
		SkipEnumField |
		SkipOrderField |
		SkipWhereInput |
		SkipMutationCreateInput |
		SkipMutationUpdateInput
)

// Name implements ent.Annotation interface.
func (Annotation) Name() string {
	return "EntGQL"
}

// OrderField enables ordering in GraphQL for the annotated Ent field
// with the given name. Note that, the field type must be comparable.
func OrderField(fields ...string) Annotation {
	return Annotation{OrderField: fields}
}

// MultiOrder indicates that orderBy should accept a list of OrderField terms.
func MultiOrder() Annotation {
	return Annotation{MultiOrder: true}
}

// Bind returns a binding annotation.
//
// Deprecated: the Bind option predates the Unbind option, and it is planned
// to be removed in future versions.
func Bind() Annotation {
	return Annotation{}
}

// Unbind implies the edge field name in GraphQL schema is not equivalent
// to the name used in ent schema.
func Unbind() Annotation {
	return Annotation{Unbind: true}
}

// MapsTo returns a mapping annotation.
func MapsTo(names ...string) Annotation {
	return Annotation{
		Mapping: names,
		Unbind:  true,
	}
}

// Type returns a type mapping annotation.
func Type(name string) Annotation {
	return Annotation{Type: name}
}

// Skip returns a skip annotation.
func Skip(flags ...SkipMode) Annotation {
	if len(flags) == 0 {
		return Annotation{Skip: SkipAll}
	}

	skip := SkipMode(0)
	for _, f := range flags {
		skip |= f
	}
	return Annotation{Skip: skip}
}

// RelayConnection returns an annotation indicating that the node/edge should support pagination.
func RelayConnection() Annotation {
	return Annotation{RelayConnection: true}
}

type queryFieldAnnotation struct {
	Annotation
}

// QueryField returns an annotation for expose the field on the Query type.
func QueryField(name ...string) queryFieldAnnotation {
	a := Annotation{QueryField: &FieldConfig{}}
	if len(name) > 0 {
		a.QueryField.Name = name[0]
	}
	return queryFieldAnnotation{Annotation: a}
}

// Description allows you to set the description for the field.
func (a queryFieldAnnotation) Description(text string) queryFieldAnnotation {
	a.QueryField.Description = text
	return a
}

type MutationOption interface {
	IsCreate() bool
	GetDescription() string
	Description(string) MutationOption
}

type builtinMutation struct {
	description string
	isCreate    bool
}

func (v builtinMutation) IsCreate() bool         { return v.isCreate }
func (v builtinMutation) GetDescription() string { return v.description }
func (v builtinMutation) Description(desc string) MutationOption {
	v.description = desc
	return v
}

func MutationCreate() MutationOption {
	return builtinMutation{isCreate: true}
}

func MutationUpdate() MutationOption {
	return builtinMutation{isCreate: false}
}

// Mutations returns an annotation for generate input types for mutation.
func Mutations(inputs ...MutationOption) Annotation {
	if len(inputs) == 0 {
		inputs = []MutationOption{MutationCreate(), MutationUpdate()}
	}

	a := []MutationConfig{}
	for _, f := range inputs {
		a = append(a, MutationConfig{
			IsCreate:    f.IsCreate(),
			Description: f.GetDescription(),
		})
	}
	return Annotation{MutationInputs: a}
}

func UseEnumNames() Annotation {
	return Annotation{UseEnumNames: true}
}

// Merge implements the schema.Merger interface.
func (a Annotation) Merge(other schema.Annotation) schema.Annotation {
	var ant Annotation
	switch other := other.(type) {
	case Annotation:
		ant = other
	case *Annotation:
		if other != nil {
			ant = *other
		}
	case queryFieldAnnotation:
		ant = other.Annotation
	case *queryFieldAnnotation:
		if other != nil {
			ant = other.Annotation
		}
	default:
		return a
	}
	if len(ant.OrderField) > 0 {
		a.OrderField = ant.OrderField
	}
	if ant.MultiOrder {
		a.MultiOrder = true
	}
	if ant.Unbind {
		a.Unbind = true
	}
	if len(ant.Mapping) != 0 {
		a.Mapping = ant.Mapping
	}
	if ant.Type != "" {
		a.Type = ant.Type
	}
	if ant.Skip.Any() {
		a.Skip |= ant.Skip
	}
	if len(ant.MutationInputs) > 0 {
		a.MutationInputs = append(a.MutationInputs, ant.MutationInputs...)
	}
	if ant.RelayConnection {
		a.RelayConnection = true
	}
	if ant.QueryField != nil {
		if a.QueryField == nil {
			a.QueryField = &FieldConfig{}
		}
		a.QueryField.merge(ant.QueryField)
	}
	if ant.UseEnumNames {
		a.UseEnumNames = true
	}
	return a
}

// Decode unmarshalls the annotation.
func (a *Annotation) Decode(annotation interface{}) error {
	buf, err := json.Marshal(annotation)
	if err != nil {
		return err
	}
	return json.Unmarshal(buf, a)
}

// Any returns true if the skip annotation was set.
func (f SkipMode) Any() bool {
	return f != 0
}

// Is checks if the skip annotation has a specific flag.
func (f SkipMode) Is(mode SkipMode) bool {
	return f&mode != 0
}

func (c FieldConfig) fieldName(gqlType string) string {
	if c.Name != "" {
		return c.Name
	}
	return camel(snake(plural(gqlType)))
}

func (c *FieldConfig) merge(ant *FieldConfig) {
	if ant == nil {
		return
	}
	if ant.Name != "" {
		c.Name = ant.Name
	}
	if ant.Description != "" {
		c.Description = ant.Description
	}
}

// annotation extracts the entgqlgo.Annotation or returns its empty value.
func annotation(ants gen.Annotations) (*Annotation, error) {
	ant := &Annotation{}
	if ants != nil && ants[ant.Name()] != nil {
		if err := ant.Decode(ants[ant.Name()]); err != nil {
			return nil, err
		}
	}
	return ant, nil
}

var (
	_ schema.Annotation = (*Annotation)(nil)
	_ schema.Merger     = (*Annotation)(nil)
)
