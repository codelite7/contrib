// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package gqlpage_test

import (
	"context"
	"testing"

	"entgo.io/contrib/entgql"
	"entgo.io/contrib/entgql/gqlpage"
	"entgo.io/ent/dialect/sql"
	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
)

// fakeQuery stands in for a generated *XQuery: a recorder for everything
// Ops routes through it, so tests can assert on what the pager applied
// without a real SQL backend.
type fakeQuery struct {
	Ctx struct {
		Fields []string
	}
	Wheres []func(*sql.Selector)
	Orders []func(*sql.Selector)

	LimitVal    int
	LimitCalled bool
	CloneCalls  int
	CountCalls  int
	AllCalls    int

	CountResult int
	CountErr    error
	AllResult   []*fakeEntity
	AllErr      error
}

func newFakeOps(multiOrder bool, def *gqlpage.Order[fakeEntity, uuid.UUID], edgeCols []string) *gqlpage.Ops[fakeQuery, fakeEntity, uuid.UUID] {
	return &gqlpage.Ops[fakeQuery, fakeEntity, uuid.UUID]{
		Where: func(q *fakeQuery, p func(*sql.Selector)) *fakeQuery {
			q.Wheres = append(q.Wheres, p)
			return q
		},
		Order: func(q *fakeQuery, o func(*sql.Selector)) *fakeQuery {
			q.Orders = append(q.Orders, o)
			return q
		},
		Limit: func(q *fakeQuery, n int) *fakeQuery {
			q.LimitVal = n
			q.LimitCalled = true
			return q
		},
		Clone: func(q *fakeQuery) *fakeQuery {
			q.CloneCalls++
			clone := *q
			clone.Ctx.Fields = append([]string(nil), q.Ctx.Fields...)
			return &clone
		},
		All: func(q *fakeQuery, _ context.Context) ([]*fakeEntity, error) {
			q.AllCalls++
			return q.AllResult, q.AllErr
		},
		Count: func(q *fakeQuery, _ context.Context) (int, error) {
			q.CountCalls++
			return q.CountResult, q.CountErr
		},
		Fields: func(q *fakeQuery) []string { return q.Ctx.Fields },
		AppendField: func(q *fakeQuery, f string) {
			for _, x := range q.Ctx.Fields {
				if x == f {
					return
				}
			}
			q.Ctx.Fields = append(q.Ctx.Fields, f)
		},
		ClearFields:     func(q *fakeQuery) { q.Ctx.Fields = nil },
		ID:              func(v *fakeEntity) uuid.UUID { return v.ID },
		Default:         def,
		MultiOrder:      multiOrder,
		EdgeTermColumns: edgeCols,
	}
}

// Shared order fields for the tests below. fakeEntity and noopTerm are
// defined in order_test.go (same package).
var (
	fIDField    = gqlpage.Column[fakeEntity, uuid.UUID]("ID", "id", "ID", noopTerm)
	fNameField  = gqlpage.Column[fakeEntity, uuid.UUID]("NAME", "name", "Name", noopTerm)
	fExprField  = gqlpage.Column[fakeEntity, uuid.UUID]("NAME_EXPR", "name", "Name", noopTerm, gqlpage.Expr(`left("name", 256)`))
	fOwnerField = gqlpage.Computed[fakeEntity, uuid.UUID]("OWNER_NAME", "owner_name", "owner_name", noopTerm)

	defaultOrder = &gqlpage.Order[fakeEntity, uuid.UUID]{Direction: entgql.OrderDirectionAsc, Field: fIDField}
)

// --- ValidateFirstLast -------------------------------------------------

func TestValidateFirstLast(t *testing.T) {
	require.Nil(t, gqlpage.ValidateFirstLast(nil, nil))

	one := 1
	err := gqlpage.ValidateFirstLast(&one, &one)
	require.NotNil(t, err)
	require.Equal(t, "Passing both `first` and `last` to paginate a connection is not supported.", err.Message)

	neg := -1
	err = gqlpage.ValidateFirstLast(&neg, nil)
	require.NotNil(t, err)
	require.Equal(t, "`first` on a connection cannot be less than zero.", err.Message)
	require.Equal(t, "INVALID_PAGINATION", err.Extensions["code"])

	err = gqlpage.ValidateFirstLast(nil, &neg)
	require.NotNil(t, err)
	require.Equal(t, "`last` on a connection cannot be less than zero.", err.Message)
	require.Equal(t, "INVALID_PAGINATION", err.Extensions["code"])
}

// --- PaginateLimit -------------------------------------------------

func TestPaginateLimit(t *testing.T) {
	first, last := 10, 20
	require.Equal(t, 11, gqlpage.PaginateLimit(&first, nil, 0))
	require.Equal(t, 21, gqlpage.PaginateLimit(nil, &last, 0))
	require.Equal(t, 0, gqlpage.PaginateLimit(nil, nil, 0))
}

// TestPaginateLimit_MaxPageSize covers ruling R11: max <= 0 means no cap,
// reproducing the old codegen exactly for an entity with no MaxPageSize
// annotation. Without a cap, no first/last yields limit == 0, which is what
// makes Paginate skip ops.Limit entirely -- an unbounded SELECT -- so that
// case matters as much as the oversized-first case.
func TestPaginateLimit_MaxPageSize(t *testing.T) {
	small, huge := 10, 100000

	// capped-from-zero: no first/last, max > 0 must not leave limit at 0.
	require.Equal(t, 51, gqlpage.PaginateLimit(nil, nil, 50))
	// capped-from-oversized: first is clamped to max+1.
	require.Equal(t, 51, gqlpage.PaginateLimit(&huge, nil, 50))
	// uncapped passthrough under the cap: max > 0 but under it, untouched.
	require.Equal(t, 11, gqlpage.PaginateLimit(&small, nil, 50))
	// max <= 0 disables the clamp entirely, even for an oversized first.
	require.Equal(t, 100001, gqlpage.PaginateLimit(&huge, nil, 0))
	require.Equal(t, 0, gqlpage.PaginateLimit(nil, nil, -1))
}

// --- Connection.Build -------------------------------------------------

func newEntities(n int) []*fakeEntity {
	nodes := make([]*fakeEntity, n)
	for i := range nodes {
		nodes[i] = &fakeEntity{ID: uuid.New(), Name: "n"}
	}
	return nodes
}

func toCursor(v *fakeEntity) entgql.Cursor[uuid.UUID] {
	return entgql.Cursor[uuid.UUID]{ID: v.ID, Value: v.Name}
}

func TestConnectionBuild_Forward_NoOverfetch(t *testing.T) {
	nodes := newEntities(2)
	first := 5
	var c gqlpage.Connection[fakeEntity, uuid.UUID]
	c.Build(nodes, toCursor, false, nil, &first, nil, nil)

	require.Len(t, c.Edges, 2)
	require.Equal(t, nodes[0], c.Edges[0].Node)
	require.Equal(t, nodes[1], c.Edges[1].Node)
	require.False(t, c.PageInfo.HasNextPage)
	require.False(t, c.PageInfo.HasPreviousPage)
	require.Equal(t, 2, c.TotalCount)
}

func TestConnectionBuild_Forward_Overfetch(t *testing.T) {
	nodes := newEntities(3) // first=2, so len(nodes) == first+1: an extra row signals a next page.
	first := 2
	var c gqlpage.Connection[fakeEntity, uuid.UUID]
	c.Build(nodes, toCursor, false, nil, &first, nil, nil)

	require.Len(t, c.Edges, 2)
	require.Equal(t, nodes[0], c.Edges[0].Node)
	require.Equal(t, nodes[1], c.Edges[1].Node)
	require.True(t, c.PageInfo.HasNextPage)
}

func TestConnectionBuild_Backward_OverfetchReverses(t *testing.T) {
	nodes := newEntities(3) // physical order is reversed (descending); last=2 means 1 extra row.
	last := 2
	var c gqlpage.Connection[fakeEntity, uuid.UUID]
	c.Build(nodes, toCursor, true, nil, nil, nil, &last)

	require.Len(t, c.Edges, 2)
	// nodeAt un-reverses: after trimming the overfetched last physical row
	// (nodes[2]), the surviving nodes[0], nodes[1] are read back to front.
	require.Equal(t, nodes[1], c.Edges[0].Node)
	require.Equal(t, nodes[0], c.Edges[1].Node)
	require.True(t, c.PageInfo.HasPreviousPage)
}

func TestConnectionBuild_Empty(t *testing.T) {
	var c gqlpage.Connection[fakeEntity, uuid.UUID]
	c.Build(nil, toCursor, false, nil, nil, nil, nil)

	require.Empty(t, c.Edges)
	require.Nil(t, c.PageInfo.StartCursor)
	require.Nil(t, c.PageInfo.EndCursor)
	require.Equal(t, 0, c.TotalCount)
}

func TestConnectionBuild_AfterBeforeSetPageInfoFlags(t *testing.T) {
	nodes := newEntities(1)
	after := entgql.Cursor[uuid.UUID]{ID: uuid.New()}
	before := entgql.Cursor[uuid.UUID]{ID: uuid.New()}
	var c gqlpage.Connection[fakeEntity, uuid.UUID]
	c.Build(nodes, toCursor, false, &after, nil, &before, nil)

	require.True(t, c.PageInfo.HasNextPage)     // before != nil
	require.True(t, c.PageInfo.HasPreviousPage) // after != nil
}

// --- WithOrder / NewPager -------------------------------------------------

// TestWithOrder_NilElementFallsBackToDefault guards R12: gql_edge_subpkg's
// single-order forwarder calls WithOrder unconditionally, wrapping a nil
// orderBy argument -- the common case, no orderBy in the GraphQL query --
// into a one-element slice holding a nil *Order. That must not panic; it
// must behave exactly like omitting WithOrder, falling back to ops.Default.
func TestWithOrder_NilElementFallsBackToDefault(t *testing.T) {
	ops := newFakeOps(false, defaultOrder, nil)
	pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
		gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID]([]*gqlpage.Order[fakeEntity, uuid.UUID]{nil}),
	}, false)
	require.NoError(t, err)

	require.NotPanics(t, func() {
		pager.ApplyOrder(&fakeQuery{})
	})
	// order.Field == defaultOrder.Field (both fIDField): no fallback term.
	q := pager.ApplyOrder(&fakeQuery{})
	require.Len(t, q.Orders, 1)
}

// --- ApplyOrder -------------------------------------------------

func TestApplyOrder_AppendsDefaultWhenFieldDiffers(t *testing.T) {
	ops := newFakeOps(false, defaultOrder, nil)
	pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
		gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID]([]*gqlpage.Order[fakeEntity, uuid.UUID]{
			{Direction: entgql.OrderDirectionAsc, Field: fNameField},
		}),
	}, false)
	require.NoError(t, err)

	q := &fakeQuery{}
	q.Ctx.Fields = []string{"id"} // non-empty selection: AppendFieldOnce is live.
	q = pager.ApplyOrder(q)

	// One term for NAME, one fallback term for the default (ID) order.
	require.Len(t, q.Orders, 2)
	require.Equal(t, []string{"id", "name"}, q.Ctx.Fields)
}

func TestApplyOrder_NoAppendWhenFieldsEmpty(t *testing.T) {
	ops := newFakeOps(false, defaultOrder, nil)
	pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
		gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID]([]*gqlpage.Order[fakeEntity, uuid.UUID]{
			{Direction: entgql.OrderDirectionAsc, Field: fNameField},
		}),
	}, false)
	require.NoError(t, err)

	q := &fakeQuery{} // Ctx.Fields empty: "select all columns" -- must not narrow it.
	q = pager.ApplyOrder(q)

	require.Empty(t, q.Ctx.Fields)
}

func TestApplyOrder_SkipsAppendForEdgeTermColumn(t *testing.T) {
	ops := newFakeOps(true, defaultOrder, []string{"owner_name"})
	pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
		gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID]([]*gqlpage.Order[fakeEntity, uuid.UUID]{
			{Direction: entgql.OrderDirectionAsc, Field: fOwnerField},
			{Direction: entgql.OrderDirectionAsc, Field: fNameField},
		}),
	}, false)
	require.NoError(t, err)

	q := &fakeQuery{}
	q.Ctx.Fields = []string{"id"}
	q = pager.ApplyOrder(q)

	// owner_name (an edge term) must never be appended; name must.
	require.Equal(t, []string{"id", "name"}, q.Ctx.Fields)
	// Both order terms still get an Order() call, plus the default-order
	// fallback (neither owner_name nor name equals the default ID column).
	require.Len(t, q.Orders, 3)
}

// TestApplyOrder_MultiOrderFlagNotInferredFromLength guards the runtime
// analogue of the old $multiOrder codegen-time branch: an entity flagged
// MultiOrder must take the multi-order path even with exactly one order
// term, which differs observably from the single-order path when the
// order field is a *different* OrderField pointer than Default but shares
// its column -- single-order compares Field pointers (an extra fallback
// term is appended), multi-order compares columns (defaultOrdered becomes
// true, no fallback term is appended).
func TestApplyOrder_MultiOrderFlagNotInferredFromLength(t *testing.T) {
	// A second OrderField instance that happens to share the "id" column
	// with defaultOrder.Field, but is not the same pointer.
	altIDField := gqlpage.Column[fakeEntity, uuid.UUID]("ID_ALT", "id", "ID", noopTerm)

	order := []*gqlpage.Order[fakeEntity, uuid.UUID]{
		{Direction: entgql.OrderDirectionAsc, Field: altIDField},
	}

	t.Run("single order: pointer inequality appends a fallback term", func(t *testing.T) {
		ops := newFakeOps(false, defaultOrder, nil)
		pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
			gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID](order),
		}, false)
		require.NoError(t, err)

		q := pager.ApplyOrder(&fakeQuery{})
		require.Len(t, q.Orders, 2) // altIDField term + default fallback term
	})

	t.Run("multi order: column equality suppresses the fallback term", func(t *testing.T) {
		ops := newFakeOps(true, defaultOrder, nil)
		pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
			gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID](order),
		}, false)
		require.NoError(t, err)

		q := pager.ApplyOrder(&fakeQuery{})
		require.Len(t, q.Orders, 1) // altIDField term only; column == default column.
	})
}

// TestApplyOrder_DirectionAndNullsDirection pins the actual direction and
// nulls-direction ApplyOrder resolves, both forward and reversed. Without
// this, inverting `if p.reverse`, dropping the nullsDirection option from
// the Term() call, or adding a stray NullsLast default all leave a
// count-only assertion (require.Len(q.Orders, n)) green -- wrong ordering
// direction is exactly the failure mode this task was flagged riskiest
// for. Uses fIDField (== defaultOrder.Field) so single-order's
// pointer-identity check suppresses the fallback term, leaving exactly one
// recorded term to inspect.
func TestApplyOrder_DirectionAndNullsDirection(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		reverse                            bool
		dir                                entgql.OrderDirection
		nulls                              entgql.NullsDirection
		wantDesc, wantNullsFirst, wantLast bool
	}{
		{"asc/nulls-last forward", false, entgql.OrderDirectionAsc, entgql.NullsLast, false, false, true},
		{"asc/nulls-last reversed", true, entgql.OrderDirectionAsc, entgql.NullsLast, true, true, false},
		{"desc/nulls-first forward", false, entgql.OrderDirectionDesc, entgql.NullsFirst, true, true, false},
		{"desc/nulls-first reversed", true, entgql.OrderDirectionDesc, entgql.NullsFirst, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recordedTerms = nil
			ops := newFakeOps(false, defaultOrder, nil)
			pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
				gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID]([]*gqlpage.Order[fakeEntity, uuid.UUID]{
					{Direction: tc.dir, NullsDirection: tc.nulls, Field: fIDField},
				}),
			}, tc.reverse)
			require.NoError(t, err)

			pager.ApplyOrder(&fakeQuery{})

			require.Len(t, recordedTerms, 1)
			require.Equal(t, tc.wantDesc, recordedTerms[0].Desc, "Desc")
			require.Equal(t, tc.wantNullsFirst, recordedTerms[0].NullsFirst, "NullsFirst")
			require.Equal(t, tc.wantLast, recordedTerms[0].NullsLast, "NullsLast")
		})
	}
}

// --- OrderExpr -------------------------------------------------

func TestOrderExpr_PlainColumn(t *testing.T) {
	ops := newFakeOps(false, defaultOrder, nil)
	pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
		gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID]([]*gqlpage.Order[fakeEntity, uuid.UUID]{
			{Direction: entgql.OrderDirectionDesc, Field: fNameField},
		}),
	}, false)
	require.NoError(t, err)

	q := &fakeQuery{}
	q.Ctx.Fields = []string{"id"}
	querier := pager.OrderExpr(q)
	sqlText, _ := querier.Query()

	require.Contains(t, sqlText, "name")
	require.Contains(t, sqlText, "DESC")
	require.Contains(t, sqlText, "NULLS LAST") // default applied since NullsDirection was unset.
	require.Contains(t, sqlText, "id")         // trailing default-order column term.
	// Not an edge term and Fields is non-empty: AppendFieldOnce ran, not Order().
	require.Empty(t, q.Orders)
	require.Equal(t, []string{"id", "name"}, q.Ctx.Fields)
}

func TestOrderExpr_ExprField(t *testing.T) {
	ops := newFakeOps(false, defaultOrder, nil)
	pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
		gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID]([]*gqlpage.Order[fakeEntity, uuid.UUID]{
			{Direction: entgql.OrderDirectionAsc, Field: fExprField},
		}),
	}, false)
	require.NoError(t, err)

	querier := pager.OrderExpr(&fakeQuery{})
	sqlText, _ := querier.Query()

	require.Contains(t, sqlText, `left("name", 256)`)
	require.Contains(t, sqlText, "ASC")
}

func TestOrderExpr_MultiOrder(t *testing.T) {
	ops := newFakeOps(true, defaultOrder, []string{"owner_name"})
	pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
		gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID]([]*gqlpage.Order[fakeEntity, uuid.UUID]{
			{Direction: entgql.OrderDirectionAsc, Field: fOwnerField},
			{Direction: entgql.OrderDirectionDesc, Field: fNameField},
		}),
	}, false)
	require.NoError(t, err)

	q := &fakeQuery{}
	q.Ctx.Fields = []string{"id"}
	querier := pager.OrderExpr(q)
	sqlText, _ := querier.Query()

	// owner_name is an edge term: goes through Order() (unlike a plain
	// column, which does not) and is never appended to Fields. It still
	// appears in the raw expression text -- the exprFunc loop below writes
	// every order term's ident/expression unconditionally, edge terms
	// included, matching the old template exactly.
	require.Len(t, q.Orders, 1)
	require.Contains(t, sqlText, "owner_name")
	require.NotContains(t, q.Ctx.Fields, "owner_name")

	// name is a plain column: appears in the raw expression and gets
	// appended to Fields, not routed through Order().
	require.Contains(t, sqlText, "name")
	require.Contains(t, sqlText, "DESC")
	require.Contains(t, q.Ctx.Fields, "name")

	// trailing default-order (id, ascending) term.
	require.Contains(t, sqlText, "id")
}

// --- ApplyCursors -------------------------------------------------

// renderWhere applies a single recorded WHERE predicate to a fresh
// Selector and returns the rendered SQL text -- the only way to observe
// the comparison operator (">" vs "<") that `direction` actually produced.
func renderWhere(t *testing.T, p func(*sql.Selector)) string {
	t.Helper()
	s := sql.Dialect("postgres").Select("id").From(sql.Table("fakes"))
	p(s)
	sqlText, _ := s.Query()
	return sqlText
}

func TestApplyCursors_UsesExpressionPredicateWhenFieldHasExpression(t *testing.T) {
	for _, tc := range []struct {
		name        string
		reverse     bool
		wantCompare string
	}{
		{"forward: greater-than", false, ">"},
		{"reversed: less-than", true, "<"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ops := newFakeOps(false, defaultOrder, nil)
			pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
				gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID]([]*gqlpage.Order[fakeEntity, uuid.UUID]{
					{Direction: entgql.OrderDirectionAsc, Field: fExprField},
				}),
			}, tc.reverse)
			require.NoError(t, err)

			after := &entgql.Cursor[uuid.UUID]{ID: uuid.New(), Value: "acme"}
			q, err := pager.ApplyCursors(&fakeQuery{}, after, nil)
			require.NoError(t, err)
			require.Len(t, q.Wheres, 1)

			sqlText := renderWhere(t, q.Wheres[0])
			require.Contains(t, sqlText, `left("name", 256)`)
			require.Contains(t, sqlText, tc.wantCompare)
		})
	}
}

func TestApplyCursors_PlainColumnPredicate(t *testing.T) {
	for _, tc := range []struct {
		name        string
		reverse     bool
		wantCompare string
	}{
		{"forward: greater-than", false, ">"},
		{"reversed: less-than", true, "<"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ops := newFakeOps(false, defaultOrder, nil)
			pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
				gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID]([]*gqlpage.Order[fakeEntity, uuid.UUID]{
					{Direction: entgql.OrderDirectionAsc, Field: fNameField},
				}),
			}, tc.reverse)
			require.NoError(t, err)

			after := &entgql.Cursor[uuid.UUID]{ID: uuid.New(), Value: "acme"}
			q, err := pager.ApplyCursors(&fakeQuery{}, after, nil)
			require.NoError(t, err)
			require.Len(t, q.Wheres, 1)

			sqlText := renderWhere(t, q.Wheres[0])
			require.Contains(t, sqlText, tc.wantCompare)
		})
	}
}

// TestApplyCursors_MultiOrder covers the MultiCursorsPredicate block
// (page.go's MultiOrder branch of ApplyCursors), previously untested: the
// idDirection flip on p.reverse, and the parallel fields/directions/
// nullsDirections slices + FieldID that multiPredicate builds from.
func TestApplyCursors_MultiOrder(t *testing.T) {
	// A cursor with no Value exercises the ID-only branch directly --
	// idDirection is the only thing that can flip the comparison here,
	// isolating it from the field-value comparison logic below.
	t.Run("ID-only cursor: idDirection flips with reverse", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			reverse     bool
			wantCompare string
		}{
			{"forward: greater-than", false, ">"},
			{"reversed: less-than", true, "<"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ops := newFakeOps(true, defaultOrder, nil)
				pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
					gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID]([]*gqlpage.Order[fakeEntity, uuid.UUID]{
						{Direction: entgql.OrderDirectionAsc, Field: fNameField},
					}),
				}, tc.reverse)
				require.NoError(t, err)

				after := &entgql.Cursor[uuid.UUID]{ID: uuid.New()} // Value nil.
				q, err := pager.ApplyCursors(&fakeQuery{}, after, nil)
				require.NoError(t, err)
				require.Len(t, q.Wheres, 1)

				sqlText := renderWhere(t, q.Wheres[0])
				require.Contains(t, sqlText, `"id"`)
				require.Contains(t, sqlText, tc.wantCompare)
			})
		}
	})

	// A cursor with a Value slice exercises multiPredicate: Fields must
	// list every order term's column in order, Directions/NullsDirections
	// must be the same length (a mismatch is a length-check error from
	// multiPredicate, so require.NoError here already guards that wiring),
	// and FieldID (the default/ID order column) must be auto-appended.
	t.Run("value cursor: fields/directions wired, FieldID auto-appended", func(t *testing.T) {
		ops := newFakeOps(true, defaultOrder, []string{"owner_name"})
		pager, err := gqlpage.NewPager(ops, []gqlpage.Option[fakeQuery, fakeEntity, uuid.UUID]{
			gqlpage.WithOrder[fakeQuery, fakeEntity, uuid.UUID]([]*gqlpage.Order[fakeEntity, uuid.UUID]{
				{Direction: entgql.OrderDirectionAsc, Field: fNameField},
				{Direction: entgql.OrderDirectionDesc, Field: fOwnerField},
			}),
		}, false)
		require.NoError(t, err)

		after := &entgql.Cursor[uuid.UUID]{ID: uuid.New(), Value: []any{"acme", "acme-owner"}}
		q, err := pager.ApplyCursors(&fakeQuery{}, after, nil)
		require.NoError(t, err)
		require.Len(t, q.Wheres, 1)

		sqlText := renderWhere(t, q.Wheres[0])
		require.Contains(t, sqlText, "name")
		require.Contains(t, sqlText, "owner_name")
		require.Contains(t, sqlText, `"id"`) // FieldID, auto-appended since not already among the order fields.
	})
}

// --- Paginate -------------------------------------------------

func TestPaginate_FirstZero_EarlyReturn(t *testing.T) {
	ops := newFakeOps(false, defaultOrder, nil)
	q := &fakeQuery{}
	first := 0
	conn, err := gqlpage.Paginate[fakeQuery, fakeEntity, uuid.UUID](q, context.Background(), nil, &first, nil, nil, ops)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Equal(t, 0, q.AllCalls)
	require.Empty(t, q.Orders)
}

// fieldSelection builds a context whose gqlgen FieldContext selects exactly
// the given top-level names -- enough for HasCollectedField/CollectedField
// to walk without a full gqlgen execution.
func fieldSelection(names ...string) context.Context {
	sels := make(ast.SelectionSet, len(names))
	for i, name := range names {
		sels[i] = &ast.Field{Name: name, Alias: name}
	}
	ctx := graphql.WithOperationContext(context.Background(), &graphql.OperationContext{})
	root := graphql.CollectedField{Field: &ast.Field{}, Selections: sels}
	return graphql.WithFieldContext(ctx, &graphql.FieldContext{Field: root})
}

func TestPaginate_IgnoredEdges_EarlyReturn(t *testing.T) {
	ops := newFakeOps(false, defaultOrder, nil)
	q := &fakeQuery{}
	ctx := fieldSelection("pageInfo") // "edges" not selected.
	conn, err := gqlpage.Paginate[fakeQuery, fakeEntity, uuid.UUID](q, ctx, nil, nil, nil, nil, ops)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Equal(t, 0, q.AllCalls)
	require.Empty(t, q.Orders)
}

// TestPaginate_TotalCountSurvivesWhenEdgesIgnored exercises the totalCount
// block's PageInfo flags: they're computed from TotalCount rather than the
// (authoritative, over-fetch-based) computation Connection.Build does, so
// they only matter -- and are only observable -- on the ignoredEdges early
// return, before Build ever runs and overwrites them.
func TestPaginate_TotalCountSurvivesWhenEdgesIgnored(t *testing.T) {
	ops := newFakeOps(false, defaultOrder, nil)
	q := &fakeQuery{CountResult: 7}
	q.Ctx.Fields = []string{"id"}
	first := 5
	ctx := fieldSelection("totalCount") // "edges" not selected.
	conn, err := gqlpage.Paginate[fakeQuery, fakeEntity, uuid.UUID](q, ctx, nil, &first, nil, nil, ops)
	require.NoError(t, err)
	require.Equal(t, 7, conn.TotalCount)
	// Count runs against a clone with fields cleared, never the original.
	require.Equal(t, 1, q.CloneCalls)
	require.True(t, conn.PageInfo.HasNextPage)
	require.Equal(t, 0, q.AllCalls)
}

func TestPaginate_HappyPath(t *testing.T) {
	ops := newFakeOps(false, defaultOrder, nil)
	nodes := newEntities(1)
	q := &fakeQuery{AllResult: nodes}
	ctx := fieldSelection("edges")
	conn, err := gqlpage.Paginate[fakeQuery, fakeEntity, uuid.UUID](q, ctx, nil, nil, nil, nil, ops)
	require.NoError(t, err)
	require.Equal(t, 1, q.AllCalls)
	require.Len(t, conn.Edges, 1)
	require.Equal(t, nodes[0], conn.Edges[0].Node)
}

// --- ToEdge -------------------------------------------------

func TestToEdge_UsesGivenOrderOrFallsBackToDefault(t *testing.T) {
	e := &fakeEntity{ID: uuid.New(), Name: "acme"}

	edge := gqlpage.ToEdge(e, nil, defaultOrder)
	require.Equal(t, e, edge.Node)
	require.Equal(t, e.ID, edge.Cursor.ID)

	nameOrder := &gqlpage.Order[fakeEntity, uuid.UUID]{Direction: entgql.OrderDirectionAsc, Field: fNameField}
	edge = gqlpage.ToEdge(e, nameOrder, defaultOrder)
	require.Equal(t, "acme", edge.Cursor.Value)
}
