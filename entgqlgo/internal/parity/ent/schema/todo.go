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
	"time"

	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Todo mirrors the entgqlgo example schema but uses entgql annotations.
type Todo struct {
	ent.Schema
}

func (Todo) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(
				entgql.OrderField("CREATED_AT"),
				entgql.Skip(entgql.SkipMutationCreateInput),
			),
		field.Enum("status").
			NamedValues(
				"InProgress", "IN_PROGRESS",
				"Completed", "COMPLETED",
				"Pending", "PENDING",
			).
			Annotations(entgql.OrderField("STATUS")),
		field.Int("priority").
			Default(0).
			Annotations(entgql.OrderField("PRIORITY")),
		field.Text("text").
			NotEmpty().
			Annotations(entgql.OrderField("TEXT")),
		// Nillable but NOT Optional: a Go pointer field that is still required in
		// the GraphQL schema (String!). entgql keys nullability off Optional only,
		// so this must render NonNull despite being Nillable.
		field.String("note").
			Nillable(),
	}
}

func (Todo) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("children", Todo.Type).
			Annotations(entgql.RelayConnection()).
			From("parent").
			Unique(),
		edge.From("category", Category.Type).
			Ref("todos").
			Unique(),
		// Required unique edge — its create-input ID field must be ID! (A5a).
		edge.To("owner", Category.Type).
			Unique().
			Required(),
	}
}

func (Todo) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entgql.Mutations(entgql.MutationCreate(), entgql.MutationUpdate()),
		entgql.MultiOrder(),
	}
}
