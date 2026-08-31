// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package gqledge is the shared runtime behind generated <pkg>/gqledges/*.go
// files (lever E-1): one call each for a unique edge, a non-unique edge, and
// a relay-connection edge, replacing the 6-to-25-line hand-rolled body the
// template used to emit per edge.
package gqledge

import (
	"context"

	"entgo.io/contrib/entgql"
	"entgo.io/contrib/entgql/gqlpage"
	"github.com/99designs/gqlgen/graphql"
)

// One resolves a unique edge: try the loaded Edges value, fall back to a
// query when not loaded, and apply mask to the result error.
//
// notLoaded and mask are both generated code (they test the generated
// internal.NotLoadedError/NotFoundError types, which this package cannot
// import) and are threaded in as parameters rather than registered in a
// package-level var: a binary linking two generated ent schemas has two
// gqledges packages, and a single global would let the second one's
// not-found test silently replace the first's.
//
// mask is the generated MaskNotFound for an optional edge (a not-found
// becomes a nil result and a nil error, i.e. GraphQL null) and nil for a
// required one.
func One[T any](loaded func() (*T, error), query func() (*T, error), notLoaded func(error) bool, mask func(error) error) (*T, error) {
	result, err := loaded()
	if notLoaded(err) {
		result, err = query()
	}
	if mask != nil {
		err = mask(err)
	}
	return result, err
}

// Many resolves a non-unique, non-connection edge: prefer the aliased
// Named<Edge> value inside a GraphQL context, else the plain Edges value,
// falling back to a query when not loaded.
func Many[T any](ctx context.Context, named func(string) ([]*T, error), loaded func() ([]*T, error), query func() ([]*T, error), notLoaded func(error) bool) ([]*T, error) {
	var result []*T
	var err error
	if fc := graphql.GetFieldContext(ctx); fc != nil && fc.Field.Alias != "" {
		result, err = named(fc.Field.Alias)
	} else {
		result, err = loaded()
	}
	if notLoaded(err) {
		result, err = query()
	}
	return result, err
}

// Conn resolves a relay-connection edge from preloaded nodes when they are
// available (the aliased edges were eager-loaded, or the totalCount alone
// was collected), else by paginating the edge query.
func Conn[Q any, T any, ID any](ctx context.Context, ops *gqlpage.Ops[Q, T, ID],
	opts []gqlpage.Option[Q, T, ID], alias string,
	totalCount int, hasTotalCount bool,
	named func(string) ([]*T, error), query func() *Q,
	after *entgql.Cursor[ID], first *int, before *entgql.Cursor[ID], last *int,
) (*gqlpage.Connection[T, ID], error) {
	if nodes, err := named(alias); err == nil || hasTotalCount {
		pager, err := gqlpage.NewPager(ops, opts, last != nil)
		if err != nil {
			return nil, err
		}
		conn := &gqlpage.Connection[T, ID]{Edges: []*gqlpage.Edge[T, ID]{}, TotalCount: totalCount}
		conn.Build(nodes, pager.ToCursor, pager.Reverse(), after, first, before, last)
		return conn, nil
	}
	return gqlpage.Paginate(query(), ctx, after, first, before, last, ops, opts...)
}
