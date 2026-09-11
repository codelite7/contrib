package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Post struct{ ent.Schema }

var (
	authorPerson = edge.To("author_person", Person.Type).Unique()
	authorBot    = edge.To("author_bot", Bot.Type).Unique()
)

func (Post) Fields() []ent.Field {
	return []ent.Field{field.String("title")}
}

func (Post) Edges() []ent.Edge {
	return []ent.Edge{
		authorPerson,
		authorBot,
		edge.To("reviewer", Person.Type).Unique(),
	}
}

func (Post) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.UnionField("author", "Author", authorPerson, authorBot),
	}
}
