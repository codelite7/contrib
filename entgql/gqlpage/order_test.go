// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package gqlpage_test

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"entgo.io/contrib/entgql/gqlpage"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// fakeEntity stands in for a generated ent model: a struct field for a plain
// column order, an ID, and a Value(name) accessor for computed/edge terms.
type fakeEntity struct {
	ID   uuid.UUID
	Name string
}

func (e *fakeEntity) Value(name string) (ent.Value, error) {
	if name == "owner_name" {
		return "acme-owner", nil
	}
	return nil, fmt.Errorf("fakeEntity: unknown field %q", name)
}

// noopTerm stands in for a handle's Order method value (F.Name.Order).
func noopTerm(...sql.OrderTermOption) func(*sql.Selector) {
	return func(*sql.Selector) {}
}

func TestColumn_ValueAndCursorReadStructField(t *testing.T) {
	id := uuid.New()
	e := &fakeEntity{ID: id, Name: "acme"}

	f := gqlpage.Column[fakeEntity, uuid.UUID]("NAME", "name", "Name", noopTerm)

	v, err := f.Value(e)
	require.NoError(t, err)
	require.Equal(t, "acme", v)

	require.Equal(t, "name", f.Column())
	require.Equal(t, "", f.Expression())

	c := f.Cursor(e)
	require.Equal(t, id, c.ID)
	require.Equal(t, "acme", c.Value)
}

func TestColumn_MissingStructFieldPanics(t *testing.T) {
	e := &fakeEntity{}
	f := gqlpage.Column[fakeEntity, uuid.UUID]("NOPE", "nope", "DoesNotExist", noopTerm)

	require.PanicsWithValue(t,
		`gqlpage: gqlpage_test.fakeEntity has no field "DoesNotExist"`,
		func() { _, _ = f.Value(e) },
	)
}

func TestColumn_FieldIndexResolvedOnceAndCached(t *testing.T) {
	e := &fakeEntity{Name: "acme"}
	f := gqlpage.Column[fakeEntity, uuid.UUID]("NAME", "name", "Name", noopTerm)

	// Concurrent first use should not race and every call must observe the
	// same resolved field, proving the resolution happened exactly once and
	// the cached result is what's being reused (not re-resolved per call).
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := f.Value(e)
			require.NoError(t, err)
			require.Equal(t, "acme", v)
		}()
	}
	wg.Wait()
}

func TestExpr_OverridesTermAndExpression(t *testing.T) {
	e := &fakeEntity{ID: uuid.New(), Name: "acme"}
	f := gqlpage.Column[fakeEntity, uuid.UUID]("NAME", "name", "Name", noopTerm,
		gqlpage.Expr(`left("name", 256)`))

	require.Equal(t, `left("name", 256)`, f.Expression())

	// Cursor value still reads the struct field, unaffected by Expr.
	c := f.Cursor(e)
	require.Equal(t, "acme", c.Value)

	for _, tc := range []struct {
		name string
		opts []sql.OrderTermOption
		want string
	}{
		{"desc", []sql.OrderTermOption{sql.OrderDesc()}, `left("name", 256) DESC`},
		{"nulls-first", []sql.OrderTermOption{sql.OrderNullsFirst()}, `left("name", 256) NULLS FIRST`},
		{"nulls-last", []sql.OrderTermOption{sql.OrderNullsLast()}, `left("name", 256) NULLS LAST`},
		{"desc-nulls-first", []sql.OrderTermOption{sql.OrderDesc(), sql.OrderNullsFirst()}, `left("name", 256) DESC NULLS FIRST`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := sql.Dialect(dialect.Postgres).Select("id").From(sql.Table("fakes"))
			term := f.Term(tc.opts...)
			term(s)
			sqlText, _ := s.Query()
			require.Contains(t, sqlText, tc.want)
		})
	}
}

func TestComputed_RoutesThroughValuer(t *testing.T) {
	e := &fakeEntity{ID: uuid.New()}
	f := gqlpage.Computed[fakeEntity, uuid.UUID]("OWNER_NAME", "owner_name", "owner_name", noopTerm)

	v, err := f.Value(e)
	require.NoError(t, err)
	require.Equal(t, "acme-owner", v)

	require.Equal(t, "owner_name", f.Column())

	c := f.Cursor(e)
	require.Equal(t, e.ID, c.ID)
	require.Equal(t, "acme-owner", c.Value)
}

func TestStringAndMarshalGQL(t *testing.T) {
	f := gqlpage.Column[fakeEntity, uuid.UUID]("NAME", "name", "Name", noopTerm)
	require.Equal(t, "NAME", f.String())

	var buf bytes.Buffer
	f.MarshalGQL(&buf)
	require.Equal(t, `"NAME"`, buf.String())
}

func TestUnmarshalGQL(t *testing.T) {
	fName := gqlpage.Column[fakeEntity, uuid.UUID]("NAME", "name", "Name", noopTerm)
	fOwner := gqlpage.Computed[fakeEntity, uuid.UUID]("OWNER_NAME", "owner_name", "owner_name", noopTerm)
	gqlpage.Register[fakeEntity, uuid.UUID](fName, fOwner)

	t.Run("resolves registered name", func(t *testing.T) {
		var got gqlpage.OrderField[fakeEntity, uuid.UUID]
		require.NoError(t, got.UnmarshalGQL("OWNER_NAME"))
		require.Equal(t, "OWNER_NAME", got.String())
	})

	t.Run("rejects non-string", func(t *testing.T) {
		var got gqlpage.OrderField[fakeEntity, uuid.UUID]
		err := got.UnmarshalGQL(123)
		require.EqualError(t, err, "fakeEntityOrderField int must be a string")
	})

	t.Run("rejects unknown name", func(t *testing.T) {
		var got gqlpage.OrderField[fakeEntity, uuid.UUID]
		err := got.UnmarshalGQL("BOGUS")
		require.EqualError(t, err, "BOGUS is not a valid fakeEntityOrderField")
	})
}
