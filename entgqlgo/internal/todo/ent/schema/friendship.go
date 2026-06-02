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

// Friendship is an explicit join entity between Todo and Category whose only
// fields are edge-bound foreign keys that are themselves skipped from GraphQL.
// It is the regression fixture for the "empty CreateFriendshipInput" bug:
// CreateFriendshipInput must carry the two edge ID fields (todoID/categoryID),
// not be an empty input that graphql-go rejects at schema construction.
type Friendship struct {
	ent.Schema
}

// Fields returns friendship fields. Both FKs are skipped from GraphQL; their
// input must instead be contributed by the bound edges below.
func (Friendship) Fields() []ent.Field {
	return []ent.Field{
		field.Int("todo_id").
			Annotations(entgqlgo.Skip()),
		field.Int("category_id").
			Annotations(entgqlgo.Skip()),
	}
}

// Edges returns friendship edges. Each is a required, unique edge bound to one
// of the skipped FK fields, so the create input exposes todoID/categoryID.
func (Friendship) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("todo", Todo.Type).
			Field("todo_id").
			Unique().
			Required(),
		edge.To("category", Category.Type).
			Field("category_id").
			Unique().
			Required(),
	}
}

// Annotations returns Friendship annotations. Only a create mutation is
// generated, matching the join-entity pattern.
func (Friendship) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgqlgo.Mutations(entgqlgo.MutationCreate()),
	}
}
