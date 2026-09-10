package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type Bot struct{ ent.Schema }

func (Bot) Fields() []ent.Field {
	return []ent.Field{field.String("model")}
}

func (Bot) Annotations() []schema.Annotation {
	return []schema.Annotation{entgql.QueryField()}
}
