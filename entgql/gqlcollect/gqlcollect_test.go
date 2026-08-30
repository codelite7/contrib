// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gqlcollect_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"entgo.io/contrib/entgql/gqlcollect"
	"github.com/99designs/gqlgen/graphql"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
)

// --- fakes standing in for a generated parent/child query pair ---------------

type parentQuery struct {
	selected []string
	// attached records one entry per Attach call: "<edge>:<alias>".
	attached []string
}

type childQuery struct{ owner string }

// withUniqueChild mirrors the generated edges.With<Parent><Edge> helper: it
// builds the sub-query itself and hands it to every option.
func withUniqueChild(q *parentQuery, opts ...func(*childQuery)) *parentQuery {
	sub := &childQuery{owner: "unique"}
	for _, opt := range opts {
		opt(sub)
	}
	q.attached = append(q.attached, "unique:")
	return q
}

// withNamedChild mirrors the generated edges.WithNamed<Parent><Edge> helper.
func withNamedChild(q *parentQuery, name string, opts ...func(*childQuery)) *parentQuery {
	sub := &childQuery{owner: "named"}
	for _, opt := range opts {
		opt(sub)
	}
	q.attached = append(q.attached, "named:"+name)
	return q
}

type collectCall struct {
	oneNode   bool
	alias     string
	path      []string
	satisfies []string
}

func newSpec(calls *[]collectCall) *gqlcollect.Spec {
	collect := func(q *childQuery, ctx context.Context, oneNode bool, opCtx *graphql.OperationContext, field graphql.CollectedField, path []string, satisfies ...string) error {
		*calls = append(*calls, collectCall{
			oneNode:   oneNode,
			alias:     field.Alias,
			path:      append([]string(nil), path...),
			satisfies: append([]string(nil), satisfies...),
		})
		return nil
	}
	return &gqlcollect.Spec{
		IDColumn: "id",
		Edges: []gqlcollect.Edge{
			gqlcollect.Unique("owner", "owner_id", []string{"User"}, collect, withUniqueChild),
			gqlcollect.Named("items", "", []string{"Item"}, collect, withNamedChild),
		},
		Fields: []gqlcollect.Field{
			{GQL: "name", Column: "name"},
			{GQL: "id", Column: ""},
			{GQL: "__typename", Column: ""},
		},
		Select: func(parent any, columns []string) {
			parent.(*parentQuery).selected = append([]string(nil), columns...)
		},
	}
}

func sel(fields ...*ast.Field) graphql.CollectedField {
	set := make(ast.SelectionSet, len(fields))
	for i, f := range fields {
		if f.Alias == "" {
			f.Alias = f.Name
		}
		set[i] = f
	}
	return graphql.CollectedField{Selections: set}
}

func run(t *testing.T, spec *gqlcollect.Spec, collected graphql.CollectedField, satisfies ...string) *parentQuery {
	t.Helper()
	p := &parentQuery{}
	require.NoError(t, gqlcollect.Collect(spec, p, context.Background(), false, &graphql.OperationContext{}, collected, nil, satisfies...))
	return p
}

// --- tests ------------------------------------------------------------------

func TestCollectMatchingArmFiresAndAliasReachesAttach(t *testing.T) {
	var calls []collectCall
	spec := newSpec(&calls)
	p := run(t, spec, sel(
		&ast.Field{Name: "owner"},
		&ast.Field{Name: "items", Alias: "myItems"},
	))
	require.Equal(t, []string{"unique:", "named:myItems"}, p.attached)
	require.Len(t, calls, 2)
	require.Equal(t, "owner", calls[0].alias)
	require.Equal(t, []string{"owner"}, calls[0].path)
	require.Equal(t, []string{"User"}, calls[0].satisfies)
	require.Equal(t, "myItems", calls[1].alias)
	require.Equal(t, []string{"myItems"}, calls[1].path)
	require.Equal(t, []string{"Item"}, calls[1].satisfies)
}

func TestCollectUniqueEdgeInheritsOneNodeNamedDoesNot(t *testing.T) {
	var calls []collectCall
	spec := newSpec(&calls)
	p := &parentQuery{}
	require.NoError(t, gqlcollect.Collect(spec, p, context.Background(), true, &graphql.OperationContext{},
		sel(&ast.Field{Name: "owner"}, &ast.Field{Name: "items"}), nil))
	require.True(t, calls[0].oneNode, "unique edge must inherit oneNode")
	require.False(t, calls[1].oneNode, "non-unique edge is always oneNode=false")
}

func TestCollectFKColumnAddedOnceAcrossAliases(t *testing.T) {
	var calls []collectCall
	spec := newSpec(&calls)
	p := run(t, spec, sel(
		&ast.Field{Name: "owner", Alias: "a"},
		&ast.Field{Name: "owner", Alias: "b"},
	))
	require.Len(t, calls, 2, "both aliases must collect")
	require.Equal(t, []string{"id", "owner_id"}, p.selected)
}

func TestCollectScalarFieldAddsColumnAndIDIsFirst(t *testing.T) {
	var calls []collectCall
	spec := newSpec(&calls)
	p := run(t, spec, sel(
		&ast.Field{Name: "name"},
		&ast.Field{Name: "id"},
		&ast.Field{Name: "__typename"},
		&ast.Field{Name: "owner"},
	))
	require.Equal(t, []string{"id", "name", "owner_id"}, p.selected)
}

func TestCollectUnknownFieldSuppressesSelect(t *testing.T) {
	var calls []collectCall
	spec := newSpec(&calls)
	p := run(t, spec, sel(
		&ast.Field{Name: "name"},
		&ast.Field{Name: "somethingElse"},
	))
	require.Nil(t, p.selected, "unknownSeen must suppress the final Select")
}

func TestCollectCustomArm(t *testing.T) {
	var got []string
	spec := &gqlcollect.Spec{
		IDColumn: "id",
		Edges: []gqlcollect.Edge{
			gqlcollect.Custom("conn", func(parent any, ctx context.Context, oneNode bool, opCtx *graphql.OperationContext, field graphql.CollectedField, path []string, satisfies []string) error {
				got = append(got, field.Alias)
				got = append(got, path...)
				got = append(got, satisfies...)
				return nil
			}),
		},
		Fields: []gqlcollect.Field{{GQL: "name", Column: "name"}},
		Select: func(parent any, columns []string) { parent.(*parentQuery).selected = columns },
	}
	p := run(t, spec, sel(&ast.Field{Name: "conn", Alias: "c"}), "Foo")
	require.Equal(t, []string{"c", "c", "Foo"}, got)
	require.Equal(t, []string{"id"}, p.selected)
}

func TestCollectNoSelectWhenSpecHasNoSelect(t *testing.T) {
	var calls []collectCall
	spec := newSpec(&calls)
	spec.Select = nil
	p := run(t, spec, sel(&ast.Field{Name: "name"}))
	require.Nil(t, p.selected)
}

// oldMayAddCondition is the helper this package replaces, copied verbatim from
// entgql/template/gql_collection_subpkg_runtime.tmpl.
func oldMayAddCondition(satisfies []string, typeCond []string) []string {
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

func TestMayAddConditionMatchesOldHelper(t *testing.T) {
	for _, tt := range []struct{ satisfies, cond []string }{
		{nil, nil},
		{nil, []string{"A", "B"}},
		{[]string{"A"}, nil},
		{[]string{"A", "B"}, []string{"B", "C"}}, // overlapping
		{[]string{"A", "B"}, []string{"C", "D"}}, // disjoint
		{[]string{"A", "B"}, []string{"A", "B"}}, // identical
		{[]string{"A"}, []string{"B", "B"}},      // duplicate in cond
	} {
		want := oldMayAddCondition(append([]string(nil), tt.satisfies...), tt.cond)
		got := gqlcollect.MayAddCondition(append([]string(nil), tt.satisfies...), tt.cond)
		if !reflect.DeepEqual(want, got) {
			t.Errorf("MayAddCondition(%v, %v) = %v, want %v", tt.satisfies, tt.cond, got, want)
		}
	}
}

func TestCollectPropagatesArmError(t *testing.T) {
	boom := errors.New("boom")
	fail := func(q *childQuery, ctx context.Context, oneNode bool, opCtx *graphql.OperationContext, field graphql.CollectedField, path []string, satisfies ...string) error {
		return boom
	}
	for name, e := range map[string]gqlcollect.Edge{
		"unique": gqlcollect.Unique("owner", "owner_id", nil, fail, withUniqueChild),
		"named":  gqlcollect.Named("items", "", nil, fail, withNamedChild),
	} {
		t.Run(name, func(t *testing.T) {
			spec := &gqlcollect.Spec{IDColumn: "id", Edges: []gqlcollect.Edge{e}}
			err := gqlcollect.Collect(spec, &parentQuery{}, context.Background(), false,
				&graphql.OperationContext{}, sel(&ast.Field{Name: e.GQL}), nil)
			require.ErrorIs(t, err, boom)
		})
	}
}

// --- duplicate GQL names (M-5) ----------------------------------------------
//
// Edges and Fields share one GQL-name namespace. The per-entity switch this
// package replaced turned a duplicate arm into a compile error ("duplicate
// case in switch"); descriptor-driven, it silently resolved last-wins (or
// edge-wins across the two tables), so a name collision would quietly route
// a field to the wrong arm. Spec.index now panics instead — and because
// every generated Spec is a package-level var, the first Collect in any test
// binary trips it.

func TestSpecIndexPanicsOnDuplicateGQLName(t *testing.T) {
	collect := func(q *childQuery, ctx context.Context, oneNode bool, opCtx *graphql.OperationContext, field graphql.CollectedField, path []string, satisfies ...string) error {
		return nil
	}

	for _, tc := range []struct {
		name string
		spec *gqlcollect.Spec
		want string
	}{
		{
			name: "duplicate edge",
			spec: &gqlcollect.Spec{Edges: []gqlcollect.Edge{
				gqlcollect.Unique("owner", "", nil, collect, withUniqueChild),
				gqlcollect.Named("owner", "", nil, collect, withNamedChild),
			}},
			want: `gqlcollect: duplicate GQL name "owner" in spec`,
		},
		{
			name: "duplicate field",
			spec: &gqlcollect.Spec{Fields: []gqlcollect.Field{
				{GQL: "name", Column: "name"},
				{GQL: "name", Column: "other_name"},
			}},
			want: `gqlcollect: duplicate GQL name "name" in spec`,
		},
		{
			name: "edge shadowed by field",
			spec: &gqlcollect.Spec{
				Edges:  []gqlcollect.Edge{gqlcollect.Unique("owner", "", nil, collect, withUniqueChild)},
				Fields: []gqlcollect.Field{{GQL: "owner", Column: "owner"}},
			},
			want: `gqlcollect: duplicate GQL name "owner" in spec`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.PanicsWithValue(t, tc.want, func() {
				_ = gqlcollect.Collect(tc.spec, &parentQuery{}, context.Background(),
					false, &graphql.OperationContext{}, sel(&ast.Field{Name: "name"}), nil)
			})
		})
	}
}

// TestSpecIndexAcceptsDistinctNames guards against the duplicate check
// rejecting a legitimate spec.
func TestSpecIndexAcceptsDistinctNames(t *testing.T) {
	var calls []collectCall
	spec := newSpec(&calls)
	require.NotPanics(t, func() {
		_ = gqlcollect.Collect(spec, &parentQuery{}, context.Background(),
			false, &graphql.OperationContext{}, sel(&ast.Field{Name: "name"}), nil)
	})
}
