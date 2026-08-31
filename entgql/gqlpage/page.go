// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package gqlpage

import (
	"context"
	"fmt"
	"reflect"

	"entgo.io/contrib/entgql"
	"entgo.io/ent/dialect/sql"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// Edge is the edge representation of an entity of type T with ID type ID.
type Edge[T any, ID any] struct {
	Node   *T                `json:"node"`
	Cursor entgql.Cursor[ID] `json:"cursor"`
}

// Connection is the relay-style connection to entities of type T with ID
// type ID.
type Connection[T any, ID any] struct {
	Edges      []*Edge[T, ID]      `json:"edges"`
	PageInfo   entgql.PageInfo[ID] `json:"pageInfo"`
	TotalCount int                 `json:"totalCount"`
}

// Build fills c's Edges and PageInfo from the (possibly one-over-fetched)
// query results, trimming the extra row and computing next/previous page
// flags. reverse must equal the pager's Reverse() -- the pager over-fetches
// in reversed physical order when paginating backwards (last != nil), and
// nodeAt below un-reverses it.
func (c *Connection[T, ID]) Build(nodes []*T, toCursor func(*T) entgql.Cursor[ID], reverse bool,
	after *entgql.Cursor[ID], first *int, before *entgql.Cursor[ID], last *int,
) {
	c.PageInfo.HasNextPage = before != nil
	c.PageInfo.HasPreviousPage = after != nil
	if first != nil && *first+1 == len(nodes) {
		c.PageInfo.HasNextPage = true
		nodes = nodes[:len(nodes)-1]
	} else if last != nil && *last+1 == len(nodes) {
		c.PageInfo.HasPreviousPage = true
		nodes = nodes[:len(nodes)-1]
	}
	var nodeAt func(int) *T
	if reverse {
		n := len(nodes) - 1
		nodeAt = func(i int) *T {
			return nodes[n-i]
		}
	} else {
		nodeAt = func(i int) *T {
			return nodes[i]
		}
	}
	c.Edges = make([]*Edge[T, ID], len(nodes))
	for i := range nodes {
		node := nodeAt(i)
		c.Edges[i] = &Edge[T, ID]{
			Node:   node,
			Cursor: toCursor(node),
		}
	}
	if l := len(c.Edges); l > 0 {
		c.PageInfo.StartCursor = &c.Edges[0].Cursor
		c.PageInfo.EndCursor = &c.Edges[l-1].Cursor
	}
	if c.TotalCount == 0 {
		c.TotalCount = len(nodes)
	}
}

// Ops is the per-entity glue: everything the pager needs from *Q that is not
// expressible as a shared method set. Emitted once per entity (~14 lines).
type Ops[Q any, T any, ID any] struct {
	Where       func(*Q, func(*sql.Selector)) *Q
	Order       func(*Q, func(*sql.Selector)) *Q
	Limit       func(*Q, int) *Q
	Clone       func(*Q) *Q
	All         func(*Q, context.Context) ([]*T, error)
	Count       func(*Q, context.Context) (int, error)
	Fields      func(*Q) []string
	AppendField func(*Q, string)
	ClearFields func(*Q)
	ID          func(*T) ID
	Default     *Order[T, ID]
	MultiOrder  bool
	// MaxPageSize is the entity's EntGQLExtension MaxPageSize annotation, or
	// 0 if absent. Paginate passes it straight through to PaginateLimit.
	MaxPageSize int
	// EdgeTermColumns are the columns of non-column order terms (the old
	// $byEdges switch). ApplyOrder/OrderExpr must not AppendFieldOnce these.
	EdgeTermColumns []string
	// Collect is the registered collectField hook, or nil. It is a getter
	// (rather than a plain func value) because the sibling gqlcollections
	// package registers the real function at its own package-init time,
	// after this Ops value is constructed.
	Collect func() func(*Q, context.Context, bool, *graphql.OperationContext, graphql.CollectedField, []string, ...string) error
}

func (o *Ops[Q, T, ID]) isEdgeTerm(column string) bool {
	for _, c := range o.EdgeTermColumns {
		if c == column {
			return true
		}
	}
	return false
}

// Pager drives one paginated query for an entity of type T queried via *Q.
type Pager[Q any, T any, ID any] struct {
	ops     *Ops[Q, T, ID]
	order   []*Order[T, ID]
	filter  func(*Q) (*Q, error)
	reverse bool
}

// Option configures a Pager.
type Option[Q any, T any, ID any] func(*Pager[Q, T, ID]) error

// WithOrder configures pagination ordering. Nil elements are skipped rather
// than validated/appended: the single-order forwarder (gql_edge_subpkg.tmpl)
// calls WithOrder unconditionally and wraps a nil orderBy argument (the
// common case -- no orderBy in the GraphQL query) into a one-element slice
// holding a nil *Order. R12: NewPager's existing empty-order fallback to
// ops.Default then applies. Multi-order codegen never builds a slice
// containing a nil order, since it only ever appends non-nil elements
// (mirroring the old template, which had no nil check here either); skipping
// nils here is a harmless superset, not something multi-order relies on.
//
// M-3: options accumulate, and a single-order Pager reads order[0], so two
// WithOrder calls are first-wins where the old generated WithXOrder replaced
// the pager's order (last-wins). Unreachable from generated code, which
// always passes exactly one WithOrder.
func WithOrder[Q any, T any, ID any](order []*Order[T, ID]) Option[Q, T, ID] {
	return func(pager *Pager[Q, T, ID]) error {
		for _, o := range order {
			if o == nil {
				continue
			}
			if err := o.Direction.Validate(); err != nil {
				return err
			}
			pager.order = append(pager.order, o)
		}
		return nil
	}
}

// WithFilter configures the pagination filter.
func WithFilter[Q any, T any, ID any](filter func(*Q) (*Q, error)) Option[Q, T, ID] {
	return func(pager *Pager[Q, T, ID]) error {
		if filter == nil {
			return fmt.Errorf("%s filter cannot be nil", reflect.TypeFor[Q]().Name())
		}
		pager.filter = filter
		return nil
	}
}

// NewPager builds a Pager for ops, applying opts and validating the result.
//
// MultiOrder is an explicit flag on ops, never inferred from len(order): a
// multi-order entity with exactly one order term must still take the
// multi-order validation/query-building path everywhere in this file.
func NewPager[Q any, T any, ID any](ops *Ops[Q, T, ID], opts []Option[Q, T, ID], reverse bool) (*Pager[Q, T, ID], error) {
	pager := &Pager[Q, T, ID]{ops: ops, reverse: reverse}
	for _, opt := range opts {
		if err := opt(pager); err != nil {
			return nil, err
		}
	}
	if ops.MultiOrder {
		for i, o := range pager.order {
			if i > 0 && o.Field == pager.order[i-1].Field {
				return nil, fmt.Errorf("duplicate order direction %q", o.Direction)
			}
		}
	} else {
		switch {
		case len(pager.order) == 0:
			pager.order = []*Order[T, ID]{ops.Default}
		case pager.order[0].Field == nil:
			o := *pager.order[0]
			o.Field = ops.Default.Field
			pager.order[0] = &o
		}
	}
	return pager, nil
}

// ApplyFilter applies the configured filter, if any.
func (p *Pager[Q, T, ID]) ApplyFilter(q *Q) (*Q, error) {
	if p.filter != nil {
		return p.filter(q)
	}
	return q, nil
}

// cursorValue is the Value half of a cursor over one order field.
//
// The default (ID) order field carries no Value: the old generated
// toCursor emitted `Cursor{ID: ...}` for it, and that nil Value is what
// keeps entgql.CursorsPredicate on its plain `id > X` branch instead of a
// composite (id, id) comparison — which on a mixed-ID graph would compare
// the id column against the *marshaled* global id.
func cursorValue[Q any, T any, ID any](ops *Ops[Q, T, ID], f *OrderField[T, ID], v *T) any {
	if f == ops.Default.Field {
		return nil
	}
	val, _ := f.Value(v)
	return val
}

// cursor builds the single-order pagination cursor for v: the entity ID as
// ops.ID reports it (which is the generated marshalID() on a mixed-ID
// graph, where T.ID's native type is not the package-wide cursor ID type),
// plus the order field's value.
func cursor[Q any, T any, ID any](ops *Ops[Q, T, ID], f *OrderField[T, ID], v *T) entgql.Cursor[ID] {
	return entgql.Cursor[ID]{ID: ops.ID(v), Value: cursorValue(ops, f, v)}
}

// ToCursor builds the pagination cursor for v.
func (p *Pager[Q, T, ID]) ToCursor(v *T) entgql.Cursor[ID] {
	if p.ops.MultiOrder {
		cs := make([]any, 0, len(p.order))
		for _, o := range p.order {
			cs = append(cs, cursorValue(p.ops, o.Field, v))
		}
		return entgql.Cursor[ID]{ID: p.ops.ID(v), Value: cs}
	}
	return cursor(p.ops, p.order[0].Field, v)
}

// Reverse reports whether the pager is walking the result set backwards
// (last != nil).
func (p *Pager[Q, T, ID]) Reverse() bool { return p.reverse }

// ApplyCursors adds the WHERE predicates implementing the after/before
// cursors.
func (p *Pager[Q, T, ID]) ApplyCursors(q *Q, after, before *entgql.Cursor[ID]) (*Q, error) {
	if p.ops.MultiOrder {
		idDirection := entgql.OrderDirectionAsc
		if p.reverse {
			idDirection = entgql.OrderDirectionDesc
		}
		fields := make([]string, 0, len(p.order))
		directions := make([]entgql.OrderDirection, 0, len(p.order))
		nullsDirections := make([]entgql.NullsDirection, 0, len(p.order))
		for _, o := range p.order {
			fields = append(fields, o.Field.Column())
			direction := o.Direction
			nullsDirection := o.NullsDirection
			if p.reverse {
				direction = direction.Reverse()
				nullsDirection = nullsDirection.Reverse()
			}
			directions = append(directions, direction)
			nullsDirections = append(nullsDirections, nullsDirection)
		}
		predicates, err := entgql.MultiCursorsPredicate(after, before, &entgql.MultiCursorsOptions{
			FieldID:         p.ops.Default.Field.Column(),
			DirectionID:     idDirection,
			Fields:          fields,
			Directions:      directions,
			NullsDirections: nullsDirections,
		})
		if err != nil {
			return nil, err
		}
		for _, predicate := range predicates {
			q = p.ops.Where(q, predicate)
		}
		return q, nil
	}

	order := p.order[0]
	direction := order.Direction
	if p.reverse {
		direction = direction.Reverse()
	}
	var predicates []func(s *sql.Selector)
	if order.Field.Expression() != "" {
		predicates = entgql.CursorsPredicateExpr(after, before, p.ops.Default.Field.Column(), order.Field.Expression(), direction)
	} else {
		predicates = entgql.CursorsPredicate(after, before, p.ops.Default.Field.Column(), order.Field.Column(), direction)
	}
	for _, predicate := range predicates {
		q = p.ops.Where(q, predicate)
	}
	return q, nil
}

// ApplyOrder adds the ORDER BY terms (and the default-order fallback, and
// the field-selection bookkeeping so cursor values can be read back).
func (p *Pager[Q, T, ID]) ApplyOrder(q *Q) *Q {
	if p.ops.MultiOrder {
		var defaultOrdered bool
		for _, o := range p.order {
			direction := o.Direction
			nullsDirection := o.NullsDirection
			if p.reverse {
				direction = direction.Reverse()
				nullsDirection = nullsDirection.Reverse()
			}
			q = p.ops.Order(q, o.Field.Term(direction.OrderTermOption(), nullsDirection.OrderTermOption()))
			if o.Field.Column() == p.ops.Default.Field.Column() {
				defaultOrdered = true
			}
			if !p.ops.isEdgeTerm(o.Field.Column()) && len(p.ops.Fields(q)) > 0 {
				p.ops.AppendField(q, o.Field.Column())
			}
		}
		if !defaultOrdered {
			direction := entgql.OrderDirectionAsc
			if p.reverse {
				direction = direction.Reverse()
			}
			q = p.ops.Order(q, p.ops.Default.Field.Term(direction.OrderTermOption()))
		}
		return q
	}

	order := p.order[0]
	direction := order.Direction
	nullsDirection := order.NullsDirection
	if p.reverse {
		direction = direction.Reverse()
		nullsDirection = nullsDirection.Reverse()
	}
	q = p.ops.Order(q, order.Field.Term(direction.OrderTermOption(), nullsDirection.OrderTermOption()))
	if order.Field != p.ops.Default.Field {
		q = p.ops.Order(q, p.ops.Default.Field.Term(direction.OrderTermOption()))
	}
	if !p.ops.isEdgeTerm(order.Field.Column()) && len(p.ops.Fields(q)) > 0 {
		p.ops.AppendField(q, order.Field.Column())
	}
	return q
}

// OrderExpr builds the ORDER BY expression used by keyset-free consumers
// (e.g. a raw COUNT/window query) that need the same ordering as ApplyOrder
// expressed as a single SQL fragment rather than as query.Order calls.
func (p *Pager[Q, T, ID]) OrderExpr(q *Q) sql.Querier {
	if p.ops.MultiOrder {
		for _, o := range p.order {
			if p.ops.isEdgeTerm(o.Field.Column()) {
				direction := o.Direction
				nullsDirection := o.NullsDirection
				if p.reverse {
					direction = direction.Reverse()
					nullsDirection = nullsDirection.Reverse()
				}
				q = p.ops.Order(q, o.Field.Term(direction.OrderTermOption(), nullsDirection.OrderTermOption()))
			} else if len(p.ops.Fields(q)) > 0 {
				p.ops.AppendField(q, o.Field.Column())
			}
		}
		return sql.ExprFunc(func(b *sql.Builder) {
			for _, o := range p.order {
				direction := o.Direction
				nullsDirection := o.NullsDirection
				if nullsDirection == "" {
					nullsDirection = entgql.NullsLast
				}
				if p.reverse {
					direction = direction.Reverse()
					nullsDirection = nullsDirection.Reverse()
				}
				if o.Field.Expression() != "" {
					b.WriteString(o.Field.Expression())
				} else {
					b.Ident(o.Field.Column())
				}
				b.Pad().WriteString(string(direction))
				b.Pad().WriteString("NULLS").Pad().WriteString(string(nullsDirection))
				b.Comma()
			}
			direction := entgql.OrderDirectionAsc
			if p.reverse {
				direction = direction.Reverse()
			}
			b.Ident(p.ops.Default.Field.Column()).Pad().WriteString(string(direction))
		})
	}

	order := p.order[0]
	direction := order.Direction
	nullsDirection := order.NullsDirection
	if nullsDirection == "" {
		nullsDirection = entgql.NullsLast
	}
	if p.reverse {
		direction = direction.Reverse()
		nullsDirection = nullsDirection.Reverse()
	}
	if p.ops.isEdgeTerm(order.Field.Column()) {
		q = p.ops.Order(q, order.Field.Term(direction.OrderTermOption(), nullsDirection.OrderTermOption()))
	} else if len(p.ops.Fields(q)) > 0 {
		p.ops.AppendField(q, order.Field.Column())
	}
	return sql.ExprFunc(func(b *sql.Builder) {
		if order.Field.Expression() != "" {
			b.WriteString(order.Field.Expression())
		} else {
			b.Ident(order.Field.Column())
		}
		b.Pad().WriteString(string(direction))
		b.Pad().WriteString("NULLS").Pad().WriteString(string(nullsDirection))
		if order.Field != p.ops.Default.Field {
			b.Comma().Ident(p.ops.Default.Field.Column()).Pad().WriteString(string(direction))
		}
	})
}

const errInvalidPagination = "INVALID_PAGINATION"

// ValidateFirstLast validates the first/last connection arguments.
func ValidateFirstLast(first, last *int) (err *gqlerror.Error) {
	switch {
	case first != nil && last != nil:
		err = &gqlerror.Error{
			Message: "Passing both `first` and `last` to paginate a connection is not supported.",
		}
	case first != nil && *first < 0:
		err = &gqlerror.Error{
			Message: "`first` on a connection cannot be less than zero.",
		}
		errcode.Set(err, errInvalidPagination)
	case last != nil && *last < 0:
		err = &gqlerror.Error{
			Message: "`last` on a connection cannot be less than zero.",
		}
		errcode.Set(err, errInvalidPagination)
	}
	return err
}

// CollectedField walks path from the current gqlgen field selection,
// returning the collected field it names, or nil if any segment is absent.
func CollectedField(ctx context.Context, path ...string) *graphql.CollectedField {
	fc := graphql.GetFieldContext(ctx)
	if fc == nil {
		return nil
	}
	field := fc.Field
	oc := graphql.GetOperationContext(ctx)
walk:
	for _, name := range path {
		for _, f := range graphql.CollectFields(oc, field.Selections, nil) {
			if f.Alias == name {
				field = f
				continue walk
			}
		}
		return nil
	}
	return &field
}

// HasCollectedField reports whether path is selected in the current gqlgen
// field selection. Outside a gqlgen resolver (no FieldContext), it reports
// true -- there is no selection set to check against.
func HasCollectedField(ctx context.Context, path ...string) bool {
	if graphql.GetFieldContext(ctx) == nil {
		return true
	}
	return CollectedField(ctx, path...) != nil
}

// PaginateLimit computes the query LIMIT for the given first/last
// connection arguments: one more than requested, so Connection.Build can
// detect and report an additional page. max is the entity's MaxPageSize
// annotation (Ops.MaxPageSize); max <= 0 means no cap -- reproducing the
// old codegen exactly, which omitted the const and the clamp entirely when
// the annotation was absent.
//
// Uncapped, no first/last yields limit == 0 (Paginate then skips
// ops.Limit): an unbounded SELECT. Capped, a missing/oversized first or
// last is clamped to max+1, which is also what guarantees callers like the
// nested-edge collection path get a positive limit out of this function.
func PaginateLimit(first, last *int, max int) int {
	var limit int
	if first != nil {
		limit = *first + 1
	} else if last != nil {
		limit = *last + 1
	}
	if max > 0 && (limit == 0 || limit > max+1) {
		limit = max + 1
	}
	return limit
}

const (
	edgesField      = "edges"
	nodeField       = "node"
	totalCountField = "totalCount"
)

// Paginate executes q and returns a relay-style cursor connection to T.
//
// The sequence below is load-bearing: validate -> new pager -> ApplyFilter
// -> totalCount (only when the totalCount field was collected AND either
// pagination args were given or edges were ignored) -> early return on
// ignored edges or a zero first/last -> ApplyCursors -> limit ->
// collectField -> ApplyOrder -> All -> Build.
func Paginate[Q any, T any, ID any](q *Q, ctx context.Context,
	after *entgql.Cursor[ID], first *int, before *entgql.Cursor[ID], last *int,
	ops *Ops[Q, T, ID], opts ...Option[Q, T, ID],
) (*Connection[T, ID], error) {
	if err := ValidateFirstLast(first, last); err != nil {
		return nil, err
	}
	pager, err := NewPager(ops, opts, last != nil)
	if err != nil {
		return nil, err
	}
	if q, err = pager.ApplyFilter(q); err != nil {
		return nil, err
	}
	conn := &Connection[T, ID]{Edges: []*Edge[T, ID]{}}
	ignoredEdges := !HasCollectedField(ctx, edgesField)
	if HasCollectedField(ctx, totalCountField) {
		hasPagination := after != nil || first != nil || before != nil || last != nil
		if hasPagination || ignoredEdges {
			c := ops.Clone(q)
			ops.ClearFields(c)
			if conn.TotalCount, err = ops.Count(c, ctx); err != nil {
				return nil, err
			}
			conn.PageInfo.HasNextPage = first != nil && conn.TotalCount > 0
			conn.PageInfo.HasPreviousPage = last != nil && conn.TotalCount > 0
		}
	}
	if ignoredEdges || (first != nil && *first == 0) || (last != nil && *last == 0) {
		return conn, nil
	}
	if q, err = pager.ApplyCursors(q, after, before); err != nil {
		return nil, err
	}
	limit := PaginateLimit(first, last, ops.MaxPageSize)
	if limit != 0 {
		q = ops.Limit(q, limit)
	}
	if field := CollectedField(ctx, edgesField, nodeField); field != nil && ops.Collect != nil {
		if fn := ops.Collect(); fn != nil {
			if err := fn(q, ctx, limit == 1, graphql.GetOperationContext(ctx), *field, []string{edgesField, nodeField}); err != nil {
				return nil, err
			}
		}
	}
	q = pager.ApplyOrder(q)
	nodes, err := ops.All(q, ctx)
	if err != nil {
		return nil, err
	}
	conn.Build(nodes, pager.ToCursor, pager.Reverse(), after, first, before, last)
	return conn, nil
}

// ToEdge converts v into an Edge, using order (or ops.Default if order is
// nil) to build the cursor. It takes ops rather than just the default order
// because the cursor ID comes from ops.ID.
func ToEdge[Q any, T any, ID any](v *T, order *Order[T, ID], ops *Ops[Q, T, ID]) *Edge[T, ID] {
	if order == nil {
		order = ops.Default
	}
	return &Edge[T, ID]{
		Node:   v,
		Cursor: cursor(ops, order.Field, v),
	}
}
