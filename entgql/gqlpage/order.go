// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package gqlpage is the shared runtime behind generated <entity>/gql_pagination.go
// files. It replaces the per-entity OrderField var block (one struct literal
// per orderable field, ~22 lines each, ~35 for an expression-backed one) with
// a single Column/Computed call per field; the field-index resolution,
// String/MarshalGQL/UnmarshalGQL and the expression-ORDER-BY literal live
// here exactly once.
package gqlpage

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"sync"

	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
)

// Valuer is the model-side accessor for non-column order terms (edge counts,
// computed fields). Generated entity structs already have it.
type Valuer interface {
	Value(name string) (ent.Value, error)
}

// fieldIndex resolves a struct field's reflect index lazily and caches it.
// It is held by pointer inside OrderField so the cache survives value-receiver
// method calls (OrderField's Value/Cursor/Term methods use a value receiver,
// like the fields they replace).
type fieldIndex struct {
	once sync.Once
	idx  []int
}

// resolve looks up name on t once, panicking loudly if the field is absent —
// a codegen bug here must fail loudly at first use, not silently produce a
// zero cursor value.
//
// sync.Once.Do marks itself done even when f panics, so a caller that
// recovers the first panic (e.g. gqlgen's default recover middleware) would
// otherwise leave fi.idx permanently nil; every later call would then feed
// reflect.Value.FieldByIndex(nil) — which returns the whole struct, not a
// zero value or a panic — silently corrupting the cursor. Re-check after Do
// returns and re-panic on every call while unresolved so a recovered first
// panic can't open that window.
func (fi *fieldIndex) resolve(t reflect.Type, name string) []int {
	fi.once.Do(func() {
		sf, ok := t.FieldByName(name)
		if !ok {
			panic(fmt.Sprintf("gqlpage: %s has no field %q", t, name))
		}
		fi.idx = sf.Index
	})
	if fi.idx == nil {
		panic(fmt.Sprintf("gqlpage: %s has no field %q", t, name))
	}
	return fi.idx
}

func fieldValue(fi *fieldIndex, name string, v any) ent.Value {
	rv := reflect.ValueOf(v).Elem()
	return rv.FieldByIndex(fi.resolve(rv.Type(), name)).Interface()
}

// OrderField defines one orderable field of an entity of type T with ID type
// ID. Built via Column or Computed; never constructed as a literal outside
// this package (the unexported fields carry state the constructors set up).
type OrderField[T any, ID any] struct {
	// Value extracts the ordering value from the given entity.
	Value func(*T) (ent.Value, error)

	gql         string
	column      string
	expression  string
	structField string
	toTerm      func(...sql.OrderTermOption) func(*sql.Selector)
	fieldIdx    *fieldIndex
	idIdx       *fieldIndex
}

// Column is the SQL column backing this field (empty for a non-field term
// keyed only by its computed/edge name).
func (f OrderField[T, ID]) Column() string { return f.column }

// Expression is the ORDER BY / cursor expression set via Expr, if any.
func (f OrderField[T, ID]) Expression() string { return f.expression }

// Term builds the ordering func for a *sql.Selector from the term builder
// (either the handle's Order method value, or the Expr-supplied literal).
func (f OrderField[T, ID]) Term(opts ...sql.OrderTermOption) func(*sql.Selector) {
	return f.toTerm(opts...)
}

// Cursor builds the pagination cursor for v: its ID plus this field's value.
func (f OrderField[T, ID]) Cursor(v *T) entgql.Cursor[ID] {
	val, _ := f.Value(v)
	idVal := fieldValue(f.idIdx, "ID", v)
	id, ok := idVal.(ID)
	if !ok {
		// Reachable only if T's ID field type doesn't match the ID type
		// param this OrderField was instantiated with — a codegen/wiring
		// bug. Loud, not a silent zero-value cursor ID.
		panic(fmt.Sprintf("gqlpage: %T.ID is %T, not assignable to cursor ID type %T", v, idVal, *new(ID)))
	}
	return entgql.Cursor[ID]{ID: id, Value: val}
}

// String implements fmt.Stringer.
func (f OrderField[T, ID]) String() string { return f.gql }

// MarshalGQL implements graphql.Marshaler.
func (f OrderField[T, ID]) MarshalGQL(w io.Writer) {
	io.WriteString(w, strconv.Quote(f.String()))
}

// UnmarshalGQL implements graphql.Unmarshaler, resolving v against the
// fields Register recorded for T.
func (f *OrderField[T, ID]) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("%sOrderField %T must be a string", reflect.TypeFor[T]().Name(), v)
	}
	found, ok := lookup[T, ID](str)
	if !ok {
		return fmt.Errorf("%s is not a valid %sOrderField", str, reflect.TypeFor[T]().Name())
	}
	*f = *found
	return nil
}

// ColumnOption configures Column.
type ColumnOption struct {
	expression string
}

// Expr overrides the ORDER BY / cursor expression and supplies the term
// builder that writes it (replacing the ~15-line inline OrderExprFunc
// literal the template used to emit per field).
//
// R5: Expr's term builder overrides the term passed to Column — the
// template passes both the handle's Order method value and Expr(...) for
// expression-backed fields so it stays uniform across arms; the Column term
// argument is simply unused when Expr is present.
func Expr(expression string) ColumnOption {
	return ColumnOption{expression: expression}
}

// exprTerm reproduces the generated OrderExprFunc literal exactly: the raw
// expression, then " DESC" when descending, then " NULLS FIRST"/"NULLS LAST".
func exprTerm(expression string) func(...sql.OrderTermOption) func(*sql.Selector) {
	return func(opts ...sql.OrderTermOption) func(*sql.Selector) {
		o := sql.NewOrderTermOptions(opts...)
		return func(s *sql.Selector) {
			s.OrderExprFunc(func(b *sql.Builder) {
				b.WriteString(expression)
				if o.Desc {
					b.WriteString(" DESC")
				}
				if o.NullsFirst {
					b.WriteString(" NULLS FIRST")
				} else if o.NullsLast {
					b.WriteString(" NULLS LAST")
				}
			})
		}
	}
}

// Column builds an order field backed by a real column.
//
//	gql:         the GraphQL enum value (e.g. "NAME")
//	column:      the SQL column constant
//	structField: the Go struct field to read for Value/Cursor (e.g. "Name")
//	term:        the entity handle's Order method value (F.Name.Order)
//	opts:        Expr(...) for an ORDER BY expression override
func Column[T any, ID any](gql, column, structField string,
	term func(...sql.OrderTermOption) func(*sql.Selector),
	opts ...ColumnOption,
) *OrderField[T, ID] {
	f := &OrderField[T, ID]{
		gql:         gql,
		column:      column,
		structField: structField,
		toTerm:      term,
		fieldIdx:    &fieldIndex{},
		idIdx:       &fieldIndex{},
	}
	for _, opt := range opts {
		if opt.expression != "" {
			f.expression = opt.expression
			f.toTerm = exprTerm(opt.expression)
		}
	}
	f.Value = func(t *T) (ent.Value, error) {
		return fieldValue(f.fieldIdx, structField, t), nil
	}
	return f
}

// Computed builds an order field whose value comes from T's Value(name)
// method rather than a struct field — edge counts and computed terms.
func Computed[T any, ID any](gql, column, valueField string,
	term func(...sql.OrderTermOption) func(*sql.Selector),
) *OrderField[T, ID] {
	f := &OrderField[T, ID]{
		gql:    gql,
		column: column,
		toTerm: term,
		idIdx:  &fieldIndex{},
	}
	f.Value = func(t *T) (ent.Value, error) {
		vv, ok := any(t).(Valuer)
		if !ok {
			panic(fmt.Sprintf("gqlpage: %T does not implement Valuer", t))
		}
		return vv.Value(valueField)
	}
	return f
}

// registry backs UnmarshalGQL: Register stores an entity's order fields
// keyed by reflect.TypeFor[T](), so distinct entity instantiations of
// OrderField[T, ID] (T differs per entity) never collide.
var (
	registryMu sync.RWMutex
	registry   = map[reflect.Type]map[string]any{}
)

// Register records an entity's order fields for UnmarshalGQL lookup and for
// %s-is-not-a-valid-XOrderField error text. Call once per entity from an
// init or a var.
func Register[T any, ID any](fields ...*OrderField[T, ID]) {
	m := make(map[string]any, len(fields))
	for _, f := range fields {
		m[f.gql] = f
	}
	registryMu.Lock()
	registry[reflect.TypeFor[T]()] = m
	registryMu.Unlock()
}

func lookup[T any, ID any](name string) (*OrderField[T, ID], bool) {
	registryMu.RLock()
	m := registry[reflect.TypeFor[T]()]
	registryMu.RUnlock()
	f, ok := m[name]
	if !ok {
		return nil, false
	}
	return f.(*OrderField[T, ID]), true
}

// Order defines the ordering of T: the field to order by, and its direction.
type Order[T any, ID any] struct {
	Direction      entgql.OrderDirection `json:"direction"`
	Field          *OrderField[T, ID]    `json:"field"`
	NullsDirection entgql.NullsDirection `json:"nullsDirection"`
}
