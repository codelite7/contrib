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
	"github.com/google/uuid"
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
			Default("ENABLED").
			Annotations(
				entgqlgo.DeprecatedEnumValues("DISABLED"),
			),
		field.Enum("kind").
			NamedValues(
				"Primary", "PRIMARY",
				"Secondary", "SECONDARY",
			).
			Default("PRIMARY").
			Annotations(
				entgqlgo.UseEnumNames(),
				entgqlgo.DeprecatedEnumValues("Secondary"),
			),
		// Optional+Nillable enum with UseEnumNames, mirroring gemini's
		// ContactPhoneNumber.phone_type. The object field must render as the
		// enum type (nullable), and its WhereInput IsNil/NotNil predicates must
		// be Boolean — the two halves of bug A2.
		field.Enum("config_type").
			NamedValues(
				"Internal", "INTERNAL",
				"External", "EXTERNAL",
				"Legacy", "LEGACY",
			).
			Optional().
			Nillable().
			Annotations(
				entgqlgo.UseEnumNames(),
				entgqlgo.DeprecatedEnumValues("Legacy"),
			),
		// String-list field annotated with a GraphQL SDL list type. Exercises the
		// SDL-type translator: "[String!]" must render as
		// graphql.NewList(graphql.NewNonNull(graphql.String)), not raw SDL.
		field.Strings("tags").
			Optional().
			Annotations(
				entgqlgo.Type("[String!]"),
			),
		// Field annotated with an unknown named type. Exercises the CustomTypes
		// registry fallback: "CustomScalarXYZ" must render as
		// customTypeOr("CustomScalarXYZ", graphql.String).
		field.JSON("config", map[string]string{}).
			Optional().
			Annotations(
				entgqlgo.Type("CustomScalarXYZ"),
			),
		// Scalar-routing coverage: a (non-ID) UUID field routes through the
		// CustomTypes registry as customTypeOr("UUID", graphql.ID).
		field.UUID("external_id", uuid.UUID{}).
			Optional(),
		// A map[string]interface{} JSON field routes as customTypeOr("Map",
		// graphql.String) to match entgql's Map scalar.
		field.JSON("attributes", map[string]interface{}{}).
			Optional(),
		// A Bytes field annotated with entgqlgo.Type("Upload") honors the
		// annotation, routing as customTypeOr("Upload", graphql.String).
		field.Bytes("payload").
			Optional().
			Annotations(
				entgqlgo.Type("Upload"),
			),
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
