package gqlinput

import (
	"errors"
	"reflect"
	"testing"

	"entgo.io/ent"
)

// widgetID is a typed ID, standing in for e.g. uuid.UUID/int ID types the
// generated mutation-input structs actually use, so the boxing test can
// confirm the dynamic type survives the []any conversion.
type widgetID int

// recorder is a Mutator that logs every call it receives, in order, as a
// comparable string plus the raw args, so tests can assert both the call
// sequence and the values/types passed through.
type recorder struct {
	calls []call
}

type call struct {
	method string
	name   string
	args   []any
}

func (r *recorder) SetField(name string, value ent.Value) error {
	r.calls = append(r.calls, call{"SetField", name, []any{value}})
	return nil
}

func (r *recorder) AppendField(name string, value ent.Value) error {
	r.calls = append(r.calls, call{"AppendField", name, []any{value}})
	return nil
}

func (r *recorder) ClearField(name string) error {
	r.calls = append(r.calls, call{"ClearField", name, nil})
	return nil
}

func (r *recorder) SetEdgeID(edge string, id any) error {
	r.calls = append(r.calls, call{"SetEdgeID", edge, []any{id}})
	return nil
}

func (r *recorder) AddEdgeIDs(edge string, ids ...any) error {
	r.calls = append(r.calls, call{"AddEdgeIDs", edge, ids})
	// Exercise that Mutate discards this error, matching the generated
	// bodies it replaces.
	return errors.New("boom")
}

func (r *recorder) RemoveEdgeIDs(edge string, ids ...any) error {
	r.calls = append(r.calls, call{"RemoveEdgeIDs", edge, ids})
	return nil
}

func (r *recorder) ClearEdge(edge string) error {
	r.calls = append(r.calls, call{"ClearEdge", edge, nil})
	return nil
}

// mixedInput exercises every op, in a deliberately mixed field/edge
// declaration order, plus an untagged field that must be ignored.
type mixedInput struct {
	Untagged string // no mutate tag: must be ignored

	ClearName bool    `mutate:"fc:name"`
	Name      *string `mutate:"f:name"`

	Age int `mutate:"f:age"` // non-pointer, non-nillable: unconditional

	Tags       []string `mutate:"f:tags"`
	AppendTags []string `mutate:"fa:tags"`

	ClearOwner bool `mutate:"ec:owner"`
	OwnerID    *int `mutate:"e:owner"`

	AddChildIDs    []widgetID `mutate:"ea:children"`
	RemoveChildIDs []widgetID `mutate:"er:children"`
}

func ptr[T any](v T) *T { return &v }

func TestMutate_AllOpsFireInDeclarationOrder(t *testing.T) {
	in := mixedInput{
		Untagged:       "ignored",
		ClearName:      true,
		Name:           ptr("alice"),
		Age:            42,
		Tags:           []string{"a", "b"},
		AppendTags:     []string{"c"}, // distinct from Tags: proves AppendField reads the paired "f" field's value, not its own
		ClearOwner:     true,
		OwnerID:        ptr(7),
		AddChildIDs:    []widgetID{1, 2},
		RemoveChildIDs: []widgetID{3},
	}

	r := &recorder{}
	Mutate(in, r)

	want := []call{
		{"ClearField", "name", nil},
		{"SetField", "name", []any{"alice"}},
		{"SetField", "age", []any{42}},
		{"SetField", "tags", []any{[]string{"a", "b"}}},
		{"AppendField", "tags", []any{[]string{"a", "b"}}},
		{"ClearEdge", "owner", nil},
		{"SetEdgeID", "owner", []any{7}},
		{"AddEdgeIDs", "children", []any{widgetID(1), widgetID(2)}},
		{"RemoveEdgeIDs", "children", []any{widgetID(3)}},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls =\n%#v\nwant\n%#v", r.calls, want)
	}

	// Boxing must preserve the dynamic type, not just the value.
	if _, ok := r.calls[7].args[0].(widgetID); !ok {
		t.Fatalf("AddEdgeIDs arg 0 lost its widgetID type: %T", r.calls[7].args[0])
	}
}

func TestMutate_SkipsNilAndFalse(t *testing.T) {
	in := mixedInput{
		Untagged: "ignored",
		Age:      0, // still fires: unconditional
	}

	r := &recorder{}
	Mutate(in, r)

	want := []call{
		{"SetField", "age", []any{0}},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls =\n%#v\nwant\n%#v", r.calls, want)
	}
}

func TestMutate_EmptySliceSkipsEdgeIDOps(t *testing.T) {
	in := mixedInput{
		AddChildIDs:    []widgetID{},
		RemoveChildIDs: []widgetID{},
	}

	r := &recorder{}
	Mutate(in, r)

	want := []call{
		{"SetField", "age", []any{0}},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("empty (non-nil) slices must not fire AddEdgeIDs/RemoveEdgeIDs: calls = %#v", r.calls)
	}
}

func TestMutate_NilSliceSkipsAppendAndEdgeIDOps(t *testing.T) {
	in := mixedInput{
		Tags:       []string{"only-set"},
		AppendTags: nil, // guard nil: AppendField must not fire
	}

	r := &recorder{}
	Mutate(in, r)

	for _, c := range r.calls {
		if c.method == "AppendField" {
			t.Fatalf("AppendField fired despite nil guard field: %#v", c)
		}
	}
}

func TestMutate_ClearBoolFalseVsTrue(t *testing.T) {
	falseCase := mixedInput{}
	r := &recorder{}
	Mutate(falseCase, r)
	for _, c := range r.calls {
		if c.method == "ClearField" || c.method == "ClearEdge" {
			t.Fatalf("Clear op fired for false guard: %#v", c)
		}
	}

	trueCase := mixedInput{ClearName: true, ClearOwner: true}
	r2 := &recorder{}
	Mutate(trueCase, r2)
	var sawClearField, sawClearEdge bool
	for _, c := range r2.calls {
		if c.method == "ClearField" && c.name == "name" {
			sawClearField = true
		}
		if c.method == "ClearEdge" && c.name == "owner" {
			sawClearEdge = true
		}
	}
	if !sawClearField || !sawClearEdge {
		t.Fatalf("Clear op did not fire for true guard: %#v", r2.calls)
	}
}

func TestMutate_UntaggedFieldIgnored(t *testing.T) {
	in := mixedInput{Untagged: "should-never-appear"}
	r := &recorder{}
	Mutate(in, r)
	for _, c := range r.calls {
		for _, a := range c.args {
			if s, ok := a.(string); ok && s == "should-never-appear" {
				t.Fatalf("untagged field value leaked into a call: %#v", c)
			}
		}
		if c.name == "Untagged" || c.name == "untagged" {
			t.Fatalf("untagged field was treated as a descriptor name: %#v", c)
		}
	}
}

// pointerFieldInput isolates the pointer-nil-skip case (SetField/SetEdgeID)
// from the always-firing non-pointer field, since mixedInput's Age field
// would otherwise always produce a call.
type pointerFieldInput struct {
	Name    *string `mutate:"f:name"`
	OwnerID *int    `mutate:"e:owner"`
}

func TestMutate_SkipsNilPointerFields(t *testing.T) {
	in := pointerFieldInput{}
	r := &recorder{}
	Mutate(in, r)
	if len(r.calls) != 0 {
		t.Fatalf("nil pointer fields must not fire any call: %#v", r.calls)
	}
}
