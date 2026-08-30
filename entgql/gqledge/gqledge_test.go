// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package gqledge_test

import (
	"context"
	"errors"
	"testing"

	"entgo.io/contrib/entgql"
	"entgo.io/contrib/entgql/gqledge"
	"entgo.io/contrib/entgql/gqlpage"
	"entgo.io/ent/dialect/sql"
	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
)

// --- shared fakes -------------------------------------------------

type fakeEntity struct {
	ID   uuid.UUID
	Name string
}

// notLoadedErr/notFoundErr stand in for the generated internal.NotLoadedError
// and internal.NotFoundError, which contrib cannot import (that's the whole
// reason notLoaded/mask are parameters -- see gqledge.go).
type notLoadedErr struct{}

func (notLoadedErr) Error() string { return "not loaded" }

type notFoundErr struct{}

func (notFoundErr) Error() string { return "not found" }

func isNotLoaded(err error) bool {
	var e notLoadedErr
	return errors.As(err, &e)
}

func isNotFound(err error) bool {
	var e notFoundErr
	return errors.As(err, &e)
}

// maskNotFound mimics the generated MaskNotFound: nils out a not-found
// error, passes everything else through.
func maskNotFound(err error) error {
	if isNotFound(err) {
		return nil
	}
	return err
}

// fieldCtx builds a context carrying a gqlgen FieldContext with the given
// alias (mirrors gqlpage_test's fieldSelection helper).
func fieldCtx(alias string) context.Context {
	return graphql.WithFieldContext(context.Background(), &graphql.FieldContext{
		Field: graphql.CollectedField{Field: &ast.Field{Alias: alias}},
	})
}

// --- One -------------------------------------------------

func TestOne_LoadedNoFallback(t *testing.T) {
	user := &fakeEntity{Name: "acme"}
	queryCalled := false
	result, err := gqledge.One(
		func() (*fakeEntity, error) { return user, nil },
		func() (*fakeEntity, error) { queryCalled = true; return nil, errors.New("must not be called") },
		isNotLoaded, maskNotFound,
	)
	require.NoError(t, err)
	require.Same(t, user, result)
	require.False(t, queryCalled, "query must not run when loaded succeeds")
}

func TestOne_NotLoadedFallsBackToQuery(t *testing.T) {
	queried := &fakeEntity{Name: "from-query"}
	result, err := gqledge.One(
		func() (*fakeEntity, error) { return nil, notLoadedErr{} },
		func() (*fakeEntity, error) { return queried, nil },
		isNotLoaded, nil,
	)
	require.NoError(t, err)
	require.Same(t, queried, result)
}

func TestOne_NotFoundMaskedWhenOptional(t *testing.T) {
	result, err := gqledge.One(
		func() (*fakeEntity, error) { return nil, notFoundErr{} },
		func() (*fakeEntity, error) {
			panic("must not be called: loaded already returned a non-notLoaded error")
		},
		isNotLoaded, maskNotFound,
	)
	require.NoError(t, err, "a mask func must nil out a not-found error")
	require.Nil(t, result)
}

func TestOne_NotFoundUnmaskedWhenRequired(t *testing.T) {
	result, err := gqledge.One(
		func() (*fakeEntity, error) { return nil, notFoundErr{} },
		func() (*fakeEntity, error) { panic("must not be called") },
		isNotLoaded, nil,
	)
	require.Error(t, err, "a nil mask must leave a not-found error on a required edge")
	require.Nil(t, result)
}

func TestOne_MaskedButGenericError_PassesErrThrough(t *testing.T) {
	// The mask func decides; One must not unconditionally nil out whatever
	// error came back -- a DB/context error on an optional edge must still
	// surface.
	dbErr := errors.New("connection refused")
	result, err := gqledge.One(
		func() (*fakeEntity, error) { return nil, dbErr },
		func() (*fakeEntity, error) { panic("must not be called") },
		isNotLoaded, maskNotFound,
	)
	require.Same(t, dbErr, err, "a mask func must not swallow a non-not-found error")
	require.Nil(t, result)
}

// TestOne_MaskIsPerCallNotGlobal pins I-2: the mask used to live in a single
// package-level var that every generated gqledges package overwrote from its
// own init(), so in a binary linking two ent schemas the second registration
// silently decided masking for the first -- an optional edge's not-found then
// surfaced as a GraphQL error instead of null. Two callers with different
// not-found types must each get their own answer, interleaved. Under the old
// global this test could not even be written: whichever init ran last won.
func TestOne_MaskIsPerCallNotGlobal(t *testing.T) {
	// A second generated schema's NotFoundError, unrelated to notFoundErr.
	type otherNotFoundErr struct{ error }
	otherMask := func(err error) error {
		var e otherNotFoundErr
		if errors.As(err, &e) {
			return nil
		}
		return err
	}

	// Schema A's not-found, masked by schema A's mask -> nil.
	_, err := gqledge.One(
		func() (*fakeEntity, error) { return nil, notFoundErr{} },
		func() (*fakeEntity, error) { panic("must not be called") },
		isNotLoaded, maskNotFound,
	)
	require.NoError(t, err)

	// The same error through schema B's mask -> untouched, proving the mask
	// is the caller's and not a process-wide last-registration-wins var.
	_, err = gqledge.One(
		func() (*fakeEntity, error) { return nil, notFoundErr{} },
		func() (*fakeEntity, error) { panic("must not be called") },
		isNotLoaded, otherMask,
	)
	require.Error(t, err)

	// And schema B's own not-found through schema B's mask -> nil, with
	// schema A's mask still working right after.
	_, err = gqledge.One(
		func() (*fakeEntity, error) { return nil, otherNotFoundErr{errors.New("b: not found")} },
		func() (*fakeEntity, error) { panic("must not be called") },
		isNotLoaded, otherMask,
	)
	require.NoError(t, err)

	_, err = gqledge.One(
		func() (*fakeEntity, error) { return nil, notFoundErr{} },
		func() (*fakeEntity, error) { panic("must not be called") },
		isNotLoaded, maskNotFound,
	)
	require.NoError(t, err)
}

// --- Many -------------------------------------------------

func TestMany_NoFieldContext_UsesLoaded(t *testing.T) {
	loadedResult := []*fakeEntity{{Name: "a"}}
	result, err := gqledge.Many(context.Background(),
		func(string) ([]*fakeEntity, error) { panic("named must not be called outside a GraphQL context") },
		func() ([]*fakeEntity, error) { return loadedResult, nil },
		func() ([]*fakeEntity, error) { panic("query must not be called") },
		isNotLoaded,
	)
	require.NoError(t, err)
	require.Equal(t, loadedResult, result)
}

func TestMany_FieldContextNoAlias_UsesLoaded(t *testing.T) {
	loadedResult := []*fakeEntity{{Name: "a"}}
	result, err := gqledge.Many(fieldCtx(""),
		func(string) ([]*fakeEntity, error) { panic("named must not be called when alias is empty") },
		func() ([]*fakeEntity, error) { return loadedResult, nil },
		func() ([]*fakeEntity, error) { panic("query must not be called") },
		isNotLoaded,
	)
	require.NoError(t, err)
	require.Equal(t, loadedResult, result)
}

func TestMany_AliasedFieldContext_UsesNamed(t *testing.T) {
	namedResult := []*fakeEntity{{Name: "b"}}
	var gotAlias string
	result, err := gqledge.Many(fieldCtx("myAlias"),
		func(alias string) ([]*fakeEntity, error) { gotAlias = alias; return namedResult, nil },
		func() ([]*fakeEntity, error) { panic("loaded must not be called when an alias is present") },
		func() ([]*fakeEntity, error) { panic("query must not be called") },
		isNotLoaded,
	)
	require.NoError(t, err)
	require.Equal(t, "myAlias", gotAlias)
	require.Equal(t, namedResult, result)
}

func TestMany_NotLoaded_FallsBackToQuery_Aliased(t *testing.T) {
	queried := []*fakeEntity{{Name: "queried"}}
	result, err := gqledge.Many(fieldCtx("myAlias"),
		func(string) ([]*fakeEntity, error) { return nil, notLoadedErr{} },
		func() ([]*fakeEntity, error) { panic("loaded must not be called") },
		func() ([]*fakeEntity, error) { return queried, nil },
		isNotLoaded,
	)
	require.NoError(t, err)
	require.Equal(t, queried, result)
}

func TestMany_NotLoaded_FallsBackToQuery_NonGraphQL(t *testing.T) {
	queried := []*fakeEntity{{Name: "queried"}}
	result, err := gqledge.Many(context.Background(),
		func(string) ([]*fakeEntity, error) { panic("named must not be called") },
		func() ([]*fakeEntity, error) { return nil, notLoadedErr{} },
		func() ([]*fakeEntity, error) { return queried, nil },
		isNotLoaded,
	)
	require.NoError(t, err)
	require.Equal(t, queried, result)
}

// --- Conn -------------------------------------------------

type fakeQuery struct {
	Ctx struct {
		Fields []string
	}
	CloneCalls int
	CountCalls int
	AllCalls   int
	AllResult  []*fakeEntity
	AllErr     error
}

func newFakeConnOps() *gqlpage.Ops[fakeQuery, fakeEntity, uuid.UUID] {
	def := &gqlpage.Order[fakeEntity, uuid.UUID]{
		Direction: entgql.OrderDirectionAsc,
		Field: gqlpage.Column[fakeEntity, uuid.UUID]("ID", "id", "ID",
			func(...sql.OrderTermOption) func(*sql.Selector) { return func(*sql.Selector) {} }),
	}
	return &gqlpage.Ops[fakeQuery, fakeEntity, uuid.UUID]{
		Where: func(q *fakeQuery, _ func(*sql.Selector)) *fakeQuery { return q },
		Order: func(q *fakeQuery, _ func(*sql.Selector)) *fakeQuery { return q },
		Limit: func(q *fakeQuery, _ int) *fakeQuery { return q },
		Clone: func(q *fakeQuery) *fakeQuery { q.CloneCalls++; c := *q; return &c },
		All: func(q *fakeQuery, _ context.Context) ([]*fakeEntity, error) {
			q.AllCalls++
			return q.AllResult, q.AllErr
		},
		Count:       func(q *fakeQuery, _ context.Context) (int, error) { q.CountCalls++; return len(q.AllResult), nil },
		Fields:      func(q *fakeQuery) []string { return q.Ctx.Fields },
		AppendField: func(q *fakeQuery, f string) { q.Ctx.Fields = append(q.Ctx.Fields, f) },
		ClearFields: func(q *fakeQuery) { q.Ctx.Fields = nil },
		ID:          func(v *fakeEntity) uuid.UUID { return v.ID },
		Default:     def,
	}
}

func TestConn_PreloadedNodes_BuildsWithoutQuerying(t *testing.T) {
	ops := newFakeConnOps()
	nodes := []*fakeEntity{{Name: "a"}, {Name: "b"}}
	queried := false

	conn, err := gqledge.Conn[fakeQuery, fakeEntity, uuid.UUID](
		context.Background(), ops, nil, "myAlias", 2, true,
		func(alias string) ([]*fakeEntity, error) {
			require.Equal(t, "myAlias", alias)
			return nodes, nil
		},
		func() *fakeQuery { queried = true; return &fakeQuery{} },
		nil, nil, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, queried, "preloaded nodes must skip the query fallback")
	require.Len(t, conn.Edges, 2)
	require.Equal(t, 2, conn.TotalCount)
}

func TestConn_NamedErrButTotalCountLoaded_StillBuildsFromNodes(t *testing.T) {
	// hasTotalCount alone (totalCount collected but edges weren't) is enough
	// to take the "preloaded" branch, mirroring the old template's
	// `err == nil || hasTotalCount` condition.
	ops := newFakeConnOps()
	queried := false

	conn, err := gqledge.Conn[fakeQuery, fakeEntity, uuid.UUID](
		context.Background(), ops, nil, "myAlias", 5, true,
		func(string) ([]*fakeEntity, error) { return nil, errors.New("edges not loaded") },
		func() *fakeQuery { queried = true; return &fakeQuery{} },
		nil, nil, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, queried)
	require.Empty(t, conn.Edges)
	require.Equal(t, 5, conn.TotalCount)
}

func TestConn_NotPreloaded_FallsBackToQuery(t *testing.T) {
	ops := newFakeConnOps()
	q := &fakeQuery{AllResult: []*fakeEntity{{Name: "queried-a"}, {Name: "queried-b"}}}

	conn, err := gqledge.Conn[fakeQuery, fakeEntity, uuid.UUID](
		context.Background(), ops, nil, "", 0, false,
		func(string) ([]*fakeEntity, error) { return nil, errors.New("not loaded") },
		func() *fakeQuery { return q },
		nil, nil, nil, nil,
	)
	require.NoError(t, err)
	require.Equal(t, 1, q.AllCalls, "not-preloaded path must run the query")
	require.Len(t, conn.Edges, 2)
}
