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

// recordedTerms captures the sql.OrderTermOptions each noopTerm call
// resolved, in call order. Field.Term(opts...) invokes the stored term
// function synchronously -- before the func(*sql.Selector) it returns is
// ever called -- so appending here reflects exactly what ApplyOrder/
// OrderExpr resolved for direction/nulls-direction, including under
// reverse, without needing to render SQL. Reset it (recordedTerms = nil)
// at the top of any test that inspects it.
var recordedTerms []*sql.OrderTermOptions

// noopTerm stands in for a handle's Order method value (F.Name.Order). It
// records the resolved options into recordedTerms instead of (or in
// addition to) doing nothing, so tests can assert on the direction/
// nulls-direction ApplyOrder or OrderExpr actually computed.
func noopTerm(opts ...sql.OrderTermOption) func(*sql.Selector) {
	recordedTerms = append(recordedTerms, sql.NewOrderTermOptions(opts...))
	return func(*sql.Selector) {}
}

func TestColumn_ValueReadsStructField(t *testing.T) {
	e := &fakeEntity{ID: uuid.New(), Name: "acme"}

	f := gqlpage.Column[fakeEntity, uuid.UUID]("NAME", "name", "Name", noopTerm)

	v, err := f.Value(e)
	require.NoError(t, err)
	require.Equal(t, "acme", v)

	require.Equal(t, "name", f.Column())
	require.Equal(t, "", f.Expression())
}

// TestColumn_MissingStructFieldPanicsAtConstruction pins M-7: the struct
// field index is resolved inside Column, so a codegen bug fails while the
// generated package-level var block is initializing -- before any request --
// rather than on the first Value/cursor read. A test that only asserted the
// panic from f.Value would pass against the old lazy resolution too, so the
// discriminating assertion is that constructing the field is what panics and
// that no *OrderField ever comes back to be called.
func TestColumn_MissingStructFieldPanicsAtConstruction(t *testing.T) {
	require.PanicsWithValue(t,
		`gqlpage: gqlpage_test.fakeEntity has no field "DoesNotExist"`,
		func() {
			gqlpage.Column[fakeEntity, uuid.UUID]("NOPE", "nope", "DoesNotExist", noopTerm)
		},
	)
}

func TestColumn_FieldIndexIsRaceFreeUnderConcurrentUse(t *testing.T) {
	e := &fakeEntity{Name: "acme"}
	f := gqlpage.Column[fakeEntity, uuid.UUID]("NAME", "name", "Name", noopTerm)

	// The index is resolved once at construction and only read afterwards,
	// so concurrent first use must neither race (-race) nor disagree.
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

	// Value still reads the struct field, unaffected by Expr.
	v, err := f.Value(e)
	require.NoError(t, err)
	require.Equal(t, "acme", v)

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
	e := &fakeEntity{}
	f := gqlpage.Computed[fakeEntity, uuid.UUID]("OWNER_NAME", "owner_name", "owner_name", noopTerm)

	v, err := f.Value(e)
	require.NoError(t, err)
	require.Equal(t, "acme-owner", v)

	require.Equal(t, "owner_name", f.Column())
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

// registerOnce is a throwaway entity type so the duplicate-Register test
// does not collide with fakeEntity's registration in TestUnmarshalGQL.
type registerOnce struct {
	ID   uuid.UUID
	Name string
}

// TestRegister_PanicsOnDuplicate pins M-2: Register used to silently replace
// the whole field map for T, so a second registration (two generated packages
// wired to the same model, a bad merge) would leave UnmarshalGQL resolving
// against a map nobody expected. It now follows registerNodeResolver's
// policy. The discriminating half is the second call: under the old code it
// returned normally and the first field became unresolvable.
func TestRegister_PanicsOnDuplicate(t *testing.T) {
	first := gqlpage.Column[registerOnce, uuid.UUID]("NAME", "name", "Name", noopTerm)
	gqlpage.Register[registerOnce, uuid.UUID](first)

	require.PanicsWithValue(t,
		"gqlpage: duplicate Register for gqlpage_test.registerOnce",
		func() {
			gqlpage.Register[registerOnce, uuid.UUID](
				gqlpage.Column[registerOnce, uuid.UUID]("OTHER", "other", "ID", noopTerm),
			)
		},
	)

	// The first registration must survive the rejected one.
	var got gqlpage.OrderField[registerOnce, uuid.UUID]
	require.NoError(t, got.UnmarshalGQL("NAME"))
	require.Equal(t, "NAME", got.String())
}
