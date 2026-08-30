package gqlwhere

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
)

// --- fakes: a local, hand-built entity mirroring the generated F/E/WhereInput shape ---

type fakeP func(*sql.Selector)

// callLog records, in order, every fake handle/combinator invocation. Each
// fake method logs synchronously when called (at predicate-construction
// time, i.e. inside Registry.P's walk), not when the returned fakeP is
// later invoked against a *sql.Selector — so callLog directly reflects the
// order Registry.P constructed the predicate tree in.
var callLog []string

func resetLog() { callLog = nil }

func logf(format string, args ...any) fakeP {
	callLog = append(callLog, fmt.Sprintf(format, args...))
	return func(*sql.Selector) {}
}

// stringField is an entfield.String-shaped handle: EQ/In/IsNil.
type stringField struct{ col string }

func (f stringField) EQ(v string) fakeP     { return logf("%s.EQ(%s)", f.col, v) }
func (f stringField) In(vs ...string) fakeP { return logf("%s.In(%v)", f.col, vs) }
func (f stringField) IsNil() fakeP          { return logf("%s.IsNil()", f.col) }

// sliceField is a handle whose EQ takes a slice directly (the RType.IsPtr
// case: the WhereInput struct field's own type is []string, identical to
// the parameter type, so dispatch passes it straight through with no deref).
type sliceField struct{ col string }

func (f sliceField) EQ(v []string) fakeP { return logf("%s.EQ(%v)", f.col, v) }

// edgeHandle is an entfield.Edge-shaped handle: Has/HasWith.
type edgeHandle struct{ name string }

func (e edgeHandle) Has() fakeP { return logf("%s.Has()", e.name) }
func (e edgeHandle) HasWith(preds ...fakeP) fakeP {
	return logf("%s.HasWith(n=%d)", e.name, len(preds))
}

func fakeNot(p fakeP) fakeP { return logf("Not") }
func fakeAnd(preds ...fakeP) fakeP {
	return logf("And(n=%d)", len(preds))
}
func fakeOr(preds ...fakeP) fakeP {
	return logf("Or(n=%d)", len(preds))
}

// --- nested entity (the neighbor type for the Owner edge) ---

type nestedField struct{ col string }

func (f nestedField) EQ(v string) fakeP { return logf("nested.%s.EQ(%s)", f.col, v) }

type nestedF struct{ X nestedField }
type nestedE struct{}

var nestedErrEmpty = NewEmptyError("nested: empty predicate NestedWhereInput")
var nestedRegistry = NewRegistry[fakeP](
	nestedF{X: nestedField{col: "nested_x"}},
	nestedE{},
	fakeNot, fakeAnd, fakeOr,
	nestedErrEmpty,
)

type nestedWhereInput struct {
	Predicates []fakeP
	Not        *nestedWhereInput
	Or         []*nestedWhereInput
	And        []*nestedWhereInput
	X          *string
}

func (i *nestedWhereInput) P() (fakeP, error) { return nestedRegistry.P(i) }

// --- the primary fake entity ---

type fakeF struct {
	Tags sliceField
	Name stringField
}

type fakeE struct {
	Owner edgeHandle
}

var fakeErrEmpty = NewEmptyError("fake: empty predicate FakeWhereInput")
var fakeRegistry = NewRegistry[fakeP](
	fakeF{Tags: sliceField{col: "tags"}, Name: stringField{col: "name"}},
	fakeE{Owner: edgeHandle{name: "owner"}},
	fakeNot, fakeAnd, fakeOr,
	fakeErrEmpty,
)

// fakeWhereInput's field order below is deliberate and non-alphabetical
// (Tags before Name) so the order test actually exercises declaration-order
// preservation rather than accidentally passing under alphabetical order.
type fakeWhereInput struct {
	Predicates []fakeP
	Not        *fakeWhereInput
	Or         []*fakeWhereInput
	And        []*fakeWhereInput

	Tags      []string
	Name      *string
	NameIn    []string
	NameIsNil bool

	HasOwner     *bool
	HasOwnerWith []*nestedWhereInput

	unexportedNoise string //lint:ignore U1000 exercised via reflection, must be skipped without panicking
}

func (i *fakeWhereInput) P() (fakeP, error) { return fakeRegistry.P(i) }

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }

// --- tests ---

func TestSingleEQ(t *testing.T) {
	resetLog()
	p, err := fakeRegistry.P(&fakeWhereInput{Name: strp("bob")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil predicate")
	}
	want := []string{"name.EQ(bob)"}
	if !equalLogs(callLog, want) {
		t.Fatalf("callLog = %v, want %v", callLog, want)
	}
}

func TestVariadicIn(t *testing.T) {
	t.Run("empty slice skipped", func(t *testing.T) {
		resetLog()
		_, err := fakeRegistry.P(&fakeWhereInput{NameIn: []string{}})
		if !errors.Is(err, fakeErrEmpty) {
			t.Fatalf("err = %v, want fakeErrEmpty", err)
		}
		if len(callLog) != 0 {
			t.Fatalf("callLog = %v, want empty", callLog)
		}
	})

	t.Run("len 2 calls In", func(t *testing.T) {
		resetLog()
		p, err := fakeRegistry.P(&fakeWhereInput{NameIn: []string{"a", "b"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil predicate")
		}
		want := []string{"name.In([a b])"}
		if !equalLogs(callLog, want) {
			t.Fatalf("callLog = %v, want %v", callLog, want)
		}
	})
}

func TestNiladicBool(t *testing.T) {
	t.Run("false skipped", func(t *testing.T) {
		resetLog()
		_, err := fakeRegistry.P(&fakeWhereInput{NameIsNil: false})
		if !errors.Is(err, fakeErrEmpty) {
			t.Fatalf("err = %v, want fakeErrEmpty", err)
		}
		if len(callLog) != 0 {
			t.Fatalf("callLog = %v, want empty", callLog)
		}
	})

	t.Run("true calls IsNil", func(t *testing.T) {
		resetLog()
		p, err := fakeRegistry.P(&fakeWhereInput{NameIsNil: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil predicate")
		}
		want := []string{"name.IsNil()"}
		if !equalLogs(callLog, want) {
			t.Fatalf("callLog = %v, want %v", callLog, want)
		}
	})
}

// TestSliceArgNoDeref covers the RType.IsPtr case: the WhereInput struct
// field's own type ([]string) equals the handle method's parameter type, so
// dispatch must pass the field value straight through with no deref, and
// must distinguish nil (skip) from a non-nil empty slice (call with []).
func TestSliceArgNoDeref(t *testing.T) {
	t.Run("nil skipped", func(t *testing.T) {
		resetLog()
		_, err := fakeRegistry.P(&fakeWhereInput{Tags: nil})
		if !errors.Is(err, fakeErrEmpty) {
			t.Fatalf("err = %v, want fakeErrEmpty", err)
		}
		if len(callLog) != 0 {
			t.Fatalf("callLog = %v, want empty", callLog)
		}
	})

	t.Run("non-nil empty slice still calls EQ", func(t *testing.T) {
		resetLog()
		p, err := fakeRegistry.P(&fakeWhereInput{Tags: []string{}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil predicate")
		}
		want := []string{"tags.EQ([])"}
		if !equalLogs(callLog, want) {
			t.Fatalf("callLog = %v, want %v", callLog, want)
		}
	})

	t.Run("populated slice", func(t *testing.T) {
		resetLog()
		p, err := fakeRegistry.P(&fakeWhereInput{Tags: []string{"x", "y"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil predicate")
		}
		want := []string{"tags.EQ([x y])"}
		if !equalLogs(callLog, want) {
			t.Fatalf("callLog = %v, want %v", callLog, want)
		}
	})
}

func TestNotOrAndNesting(t *testing.T) {
	t.Run("not wraps nested result", func(t *testing.T) {
		resetLog()
		p, err := fakeRegistry.P(&fakeWhereInput{Not: &fakeWhereInput{Name: strp("x")}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil predicate")
		}
		want := []string{"name.EQ(x)", "Not"}
		if !equalLogs(callLog, want) {
			t.Fatalf("callLog = %v, want %v", callLog, want)
		}
	})

	t.Run("or n==1 shortcut skips Or() wrapper", func(t *testing.T) {
		resetLog()
		p, err := fakeRegistry.P(&fakeWhereInput{Or: []*fakeWhereInput{{Name: strp("a")}}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil predicate")
		}
		want := []string{"name.EQ(a)"}
		if !equalLogs(callLog, want) {
			t.Fatalf("callLog = %v, want %v (Or() must not be called for n==1)", callLog, want)
		}
	})

	t.Run("or n>1 calls Or()", func(t *testing.T) {
		resetLog()
		p, err := fakeRegistry.P(&fakeWhereInput{Or: []*fakeWhereInput{{Name: strp("a")}, {Name: strp("b")}}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil predicate")
		}
		want := []string{"name.EQ(a)", "name.EQ(b)", "Or(n=2)"}
		if !equalLogs(callLog, want) {
			t.Fatalf("callLog = %v, want %v", callLog, want)
		}
	})

	t.Run("and n==1 shortcut skips And() wrapper", func(t *testing.T) {
		resetLog()
		p, err := fakeRegistry.P(&fakeWhereInput{And: []*fakeWhereInput{{Name: strp("a")}}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil predicate")
		}
		want := []string{"name.EQ(a)"}
		if !equalLogs(callLog, want) {
			t.Fatalf("callLog = %v, want %v (And() must not be called for n==1)", callLog, want)
		}
	})

	t.Run("and n>1 calls And()", func(t *testing.T) {
		resetLog()
		p, err := fakeRegistry.P(&fakeWhereInput{And: []*fakeWhereInput{{Name: strp("a")}, {Name: strp("b")}}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil predicate")
		}
		want := []string{"name.EQ(a)", "name.EQ(b)", "And(n=2)"}
		if !equalLogs(callLog, want) {
			t.Fatalf("callLog = %v, want %v", callLog, want)
		}
	})
}

func TestNestedEmptyProducesErrEmpty(t *testing.T) {
	t.Run("empty Not is silently suppressed", func(t *testing.T) {
		resetLog()
		_, err := fakeRegistry.P(&fakeWhereInput{Not: &fakeWhereInput{}})
		if !errors.Is(err, fakeErrEmpty) {
			t.Fatalf("err = %v, want fakeErrEmpty (nothing else was set)", err)
		}
		if len(callLog) != 0 {
			t.Fatalf("callLog = %v, want empty (no Not() call, no wrapped error)", callLog)
		}
	})

	t.Run("empty Or (n==1) is silently suppressed", func(t *testing.T) {
		resetLog()
		_, err := fakeRegistry.P(&fakeWhereInput{Or: []*fakeWhereInput{{}}})
		if !errors.Is(err, fakeErrEmpty) {
			t.Fatalf("err = %v, want fakeErrEmpty", err)
		}
		if len(callLog) != 0 {
			t.Fatalf("callLog = %v, want empty", callLog)
		}
	})

	t.Run("direct P call on empty input surfaces ErrEmpty", func(t *testing.T) {
		_, err := (&nestedWhereInput{}).P()
		if !errors.Is(err, ErrEmpty) {
			t.Fatalf("err = %v, want to satisfy errors.Is(err, ErrEmpty)", err)
		}
	})
}

func TestHasXWithEmptyNestedProducesNeverMatch(t *testing.T) {
	resetLog()
	p, err := fakeRegistry.P(&fakeWhereInput{
		HasOwnerWith: []*nestedWhereInput{{}}, // nested is entirely empty
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil predicate")
	}
	for _, entry := range callLog {
		if strings.Contains(entry, "owner.HasWith") {
			t.Fatalf("callLog = %v: HasWith must not be called when the nested predicate is empty", callLog)
		}
	}

	sel := sql.Dialect(dialect.Postgres).Select("*").From(sql.Table("t"))
	p(sel)
	query, _ := sel.Query()
	if !strings.Contains(strings.ToUpper(query), "FALSE") {
		t.Fatalf("query = %q, want a never-match (FALSE) predicate", query)
	}
}

func TestHasXWithPopulatedNestedCallsHasWith(t *testing.T) {
	resetLog()
	p, err := fakeRegistry.P(&fakeWhereInput{
		HasOwnerWith: []*nestedWhereInput{{X: strp("v")}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil predicate")
	}
	want := []string{"nested.nested_x.EQ(v)", "owner.HasWith(n=1)"}
	if !equalLogs(callLog, want) {
		t.Fatalf("callLog = %v, want %v", callLog, want)
	}
}

func TestHasEdgeBool(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		resetLog()
		p, err := fakeRegistry.P(&fakeWhereInput{HasOwner: boolp(true)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil predicate")
		}
		want := []string{"owner.Has()"}
		if !equalLogs(callLog, want) {
			t.Fatalf("callLog = %v, want %v", callLog, want)
		}
	})

	t.Run("false wraps with Not", func(t *testing.T) {
		resetLog()
		p, err := fakeRegistry.P(&fakeWhereInput{HasOwner: boolp(false)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil predicate")
		}
		want := []string{"owner.Has()", "Not"}
		if !equalLogs(callLog, want) {
			t.Fatalf("callLog = %v, want %v", callLog, want)
		}
	})
}

func TestEmptyInputReturnsLeafSentinel(t *testing.T) {
	resetLog()
	p, err := fakeRegistry.P(&fakeWhereInput{})
	if p != nil {
		t.Fatalf("p = %v, want nil", p)
	}
	if err != fakeErrEmpty {
		t.Fatalf("err = %v, want fakeErrEmpty exactly", err)
	}

	// FilterP flattens the empty case.
	p, err = fakeRegistry.FilterP(&fakeWhereInput{})
	if p != nil || err != nil {
		t.Fatalf("FilterP(empty) = (%v, %v), want (nil, nil)", p, err)
	}

	// FilterP passes through a populated result unchanged.
	resetLog()
	p, err = fakeRegistry.FilterP(&fakeWhereInput{Name: strp("x")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil predicate")
	}
}

// TestFieldOrder asserts predicates arrive in the WhereInput struct's
// declaration order (Tags before Name, in this fixture — deliberately not
// alphabetical), not e.g. Go's randomized map-iteration order.
func TestFieldOrder(t *testing.T) {
	resetLog()
	_, err := fakeRegistry.P(&fakeWhereInput{
		Tags:         []string{"t"},
		Name:         strp("n"),
		NameIn:       []string{"a", "b"},
		NameIsNil:    true,
		HasOwner:     boolp(true),
		HasOwnerWith: []*nestedWhereInput{{X: strp("v")}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{
		"tags.EQ([t])",
		"name.EQ(n)",
		"name.In([a b])",
		"name.IsNil()",
		"owner.Has()",
		"nested.nested_x.EQ(v)",
		"owner.HasWith(n=1)",
		"And(n=6)", // final aggregation: len(predicates) > 1
	}
	if !equalLogs(callLog, want) {
		t.Fatalf("callLog = %v, want %v", callLog, want)
	}
}

// badWhereInput has a field ("Bogus") with no matching registered op on
// fakeF/fakeE, simulating a codegen bug in a generated WhereInput struct.
type badWhereInput struct {
	Predicates []fakeP
	Not        *badWhereInput
	Or         []*badWhereInput
	And        []*badWhereInput
	Bogus      *string
}

func TestWarmPanicsAtConstructionOnUnregisteredField(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected Warm to panic on a field with no registered op")
		}
	}()
	fakeRegistry.Warm((*badWhereInput)(nil))
}

// TestWarmCachesPlanForSubsequentUse asserts that Warm populates the same
// plan cache Registry.P reads from, so a later P() call for that type reuses
// the plan Warm already built instead of building it again.
func TestWarmCachesPlanForSubsequentUse(t *testing.T) {
	reg := NewRegistry[fakeP](
		fakeF{Tags: sliceField{col: "tags"}, Name: stringField{col: "name"}},
		fakeE{Owner: edgeHandle{name: "owner"}},
		fakeNot, fakeAnd, fakeOr,
		NewEmptyError("warm: empty predicate FakeWhereInput"),
	).Warm((*fakeWhereInput)(nil))

	structType := reflect.TypeOf(fakeWhereInput{})
	warmed, ok := reg.plans.Load(structType)
	if !ok {
		t.Fatal("Warm did not populate the plan cache")
	}

	resetLog()
	if _, err := reg.P(&fakeWhereInput{Name: strp("x")}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	afterP, ok := reg.plans.Load(structType)
	if !ok || afterP != warmed {
		t.Fatal("P() rebuilt the plan instead of reusing the one Warm cached")
	}
}

func equalLogs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
