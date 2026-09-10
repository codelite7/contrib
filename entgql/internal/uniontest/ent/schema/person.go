package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type Person struct{ ent.Schema }

func (Person) Fields() []ent.Field {
	return []ent.Field{field.String("name")}
}

func (Person) Annotations() []schema.Annotation {
	return []schema.Annotation{entgql.QueryField()}
}
