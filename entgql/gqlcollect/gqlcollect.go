// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package gqlcollect is a descriptor-driven replacement for the per-entity
// collectField<Entity>Query switch that entgql/template/gql_collection_subpkg.tmpl
// used to generate (one ~600-line file per entity).
//
// The generated file now declares a Spec — one Edge or Field entry per switch
// arm — and calls Collect, which reproduces exactly what the old switch body
// did: it walks graphql.CollectFields in order, dispatches each collected
// field to its arm, accumulates selectedFields under the same fieldSeen
// dedupe, and applies the trailing Select only when no unknown field was seen.
//
// # Type erasure boundary
//
// A []Edge is heterogeneous — every entry names a different parent/child query
// pair — so Edge itself stores one fully erased closure, Edge.Arm. The typed
// half is supplied by the generic constructors Unique, Named and Custom, whose
// type parameters are inferred from the generated attach helper, letting the
// generator emit one line per edge.
//
// # What stays generated
//
// Relay-connection arms are not uniform enough to erase profitably: their body
// threads a per-entity pager, a per-entity paginate-args struct, and one of two
// AppendLoadTotal closures (M2M join vs. FK group-by). They keep their
// generated body and enter the table through Custom.
package gqlcollect

import (
	"context"
	"sync"

	"github.com/99designs/gqlgen/graphql"
)

// CollectFn is the signature of a generated per-entity collectField helper.
type CollectFn[C any] func(q C, ctx context.Context, oneNode bool, opCtx *graphql.OperationContext, field graphql.CollectedField, path []string, satisfies ...string) error

// ArmFn is one fully type-erased collection arm. path already ends with the
// collected field's alias, and satisfies has already been widened by
// MayAddCondition with the edge's Implementors.
type ArmFn func(parent any, ctx context.Context, oneNode bool, opCtx *graphql.OperationContext, field graphql.CollectedField, path []string, satisfies []string) error

// Edge describes one collectable GraphQL edge field of an entity. Build one
// with Unique, Named or Custom rather than by hand.
type Edge struct {
	// GQL is the GraphQL field name this arm matches. An edge exposed under
	// several names gets one Edge entry per name.
	GQL string
	// FKColumn, when non-empty, is added to the parent's selected fields
	// after the arm runs — the old "{{ with $e.Field }}" block.
	FKColumn string
	// Implementors is the target entity's Implementors slice.
	Implementors []string
	// Arm runs the arm body.
	Arm ArmFn
}

// Field describes one collectable scalar field. An empty Column means the arm
// matches but selects nothing — the old empty "id" and "__typename" cases.
type Field struct {
	GQL    string
	Column string
}

// Spec is one entity's collection table. Declare it once as a package-level
// var; the name index behind it is built on first use.
type Spec struct {
	// IDColumn, when non-empty, is always the first selected column.
	IDColumn string
	Edges    []Edge
	Fields   []Field
	// Select applies the accumulated column selection to the parent query.
	// A nil Select means the entity generated no scalar arms, so the old
	// template emitted no trailing Select either.
	Select func(parent any, columns []string)

	once     sync.Once
	edgeIdx  map[string]int
	fieldIdx map[string]int
}

func (s *Spec) index() {
	s.once.Do(func() {
		s.edgeIdx = make(map[string]int, len(s.Edges))
		for i := range s.Edges {
			s.edgeIdx[s.Edges[i].GQL] = i
		}
		s.fieldIdx = make(map[string]int, len(s.Fields))
		for i := range s.Fields {
			s.fieldIdx[s.Fields[i].GQL] = i
		}
	})
}

// Collect is the generic replacement for the per-entity collectField switch.
func Collect(spec *Spec, parent any, ctx context.Context, oneNode bool, opCtx *graphql.OperationContext, collected graphql.CollectedField, path []string, satisfies ...string) error {
	spec.index()
	path = append([]string(nil), path...)
	var (
		unknownSeen    bool
		fieldSeen      = make(map[string]struct{}, len(spec.Fields))
		selectedFields []string
	)
	if spec.IDColumn != "" {
		selectedFields = []string{spec.IDColumn}
	}
	for _, field := range graphql.CollectFields(opCtx, collected.Selections, satisfies) {
		if i, ok := spec.edgeIdx[field.Name]; ok {
			e := &spec.Edges[i]
			if err := e.Arm(parent, ctx, oneNode, opCtx, field, append(path, field.Alias), MayAddCondition(satisfies, e.Implementors)); err != nil {
				return err
			}
			if e.FKColumn != "" {
				selectedFields = addColumn(selectedFields, fieldSeen, e.FKColumn)
			}
			continue
		}
		i, ok := spec.fieldIdx[field.Name]
		if !ok {
			unknownSeen = true
			continue
		}
		if c := spec.Fields[i].Column; c != "" {
			selectedFields = addColumn(selectedFields, fieldSeen, c)
		}
	}
	// In case the schema was extended, a non-selected field might be used by
	// a custom resolver, so an unknown field disables the projection.
	if !unknownSeen && spec.Select != nil {
		spec.Select(parent, selectedFields)
	}
	return nil
}

func addColumn(selected []string, seen map[string]struct{}, column string) []string {
	if _, ok := seen[column]; ok {
		return selected
	}
	seen[column] = struct{}{}
	return append(selected, column)
}

// Unique builds the arm of a unique edge: it inherits the parent's oneNode and
// attaches unnamed. attach is the generated edges.With<Parent><Edge> helper,
// which builds the sub-query itself and passes it to every option.
func Unique[P, C any](gql, fkColumn string, implementors []string, collect CollectFn[C], attach func(P, ...func(C)) P) Edge {
	return Edge{GQL: gql, FKColumn: fkColumn, Implementors: implementors,
		Arm: func(parent any, ctx context.Context, oneNode bool, opCtx *graphql.OperationContext, field graphql.CollectedField, path []string, satisfies []string) error {
			var err error
			attach(parent.(P), func(sub C) {
				err = collect(sub, ctx, oneNode, opCtx, field, path, satisfies...)
			})
			return err
		}}
}

// Named builds the arm of a non-unique, non-paginated edge: oneNode is always
// false and the sub-query is attached under the field's alias. attach is the
// generated edges.WithNamed<Parent><Edge> helper.
func Named[P, C any](gql, fkColumn string, implementors []string, collect CollectFn[C], attach func(P, string, ...func(C)) P) Edge {
	return Edge{GQL: gql, FKColumn: fkColumn, Implementors: implementors,
		Arm: func(parent any, ctx context.Context, _ bool, opCtx *graphql.OperationContext, field graphql.CollectedField, path []string, satisfies []string) error {
			var err error
			attach(parent.(P), field.Alias, func(sub C) {
				err = collect(sub, ctx, false, opCtx, field, path, satisfies...)
			})
			return err
		}}
}

// Custom is the escape hatch for an arm whose body stays generated —
// relay-connection edges. arm receives satisfies unwidened, so the generated
// body keeps its own MayAddCondition call.
func Custom(gql string, arm ArmFn) Edge {
	return Edge{GQL: gql, Arm: arm}
}

// MayAddCondition appends another type condition to the satisfies list if it
// does not exist in the list.
func MayAddCondition(satisfies []string, typeCond []string) []string {
Cond:
	for _, c := range typeCond {
		for _, s := range satisfies {
			if c == s {
				continue Cond
			}
		}
		satisfies = append(satisfies, c)
	}
	return satisfies
}
