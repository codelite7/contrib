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

package schema

import (
	"entgo.io/contrib/entgqlgo"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Category defines the category type schema.
type Category struct {
	ent.Schema
}

// Fields returns category fields.
func (Category) Fields() []ent.Field {
	return []ent.Field{
		field.String("text").
			NotEmpty().
			Annotations(
				entgqlgo.OrderField("TEXT"),
			),
		field.Enum("status").
			NamedValues(
				"Enabled", "ENABLED",
				"Disabled", "DISABLED",
			).
			Default("ENABLED"),
	}
}

// Edges returns category edges.
func (Category) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("todos", Todo.Type).
			Annotations(
				entgqlgo.RelayConnection(),
			),
	}
}

// Annotations returns Category annotations.
func (Category) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgqlgo.RelayConnection(),
		entgqlgo.QueryField(),
		entgqlgo.Mutations(entgqlgo.MutationCreate(), entgqlgo.MutationUpdate()),
		entgqlgo.Implements("NamedNode"),
	}
}
