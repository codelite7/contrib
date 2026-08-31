// Package gqlinput is a reflection-driven replacement for the per-input
// Mutate() method bodies that entgql/template/mutation_input_sibling.tmpl
// used to generate (roughly half of each ~800-line mutationinputs/<entity>.go
// file, in long runs of "if v := i.Name; v != nil { _ = m.SetField(...) }").
// Mutate walks any mutation-input struct value via reflection and reproduces
// exactly what the generated Mutate body used to do, call for call, in the
// same order.
//
// # Tags, not name inference
//
// The walker is driven by `mutate:"<op>:<name>"` struct tags, not by
// converting Go field names to the descriptor's snake_case field/edge names.
// That round trip is lossy — ZoomInfoCompanyID -> zoom_info_company_id,
// SfObject -> sf_object — and a wrong guess would write to the wrong column,
// silently, with no compile-time or even test-time signal unless the exact
// field happened to be covered. Struct tags cost zero generated lines: they
// attach to struct fields the generator already emits, so the walker reads
// the descriptor name straight from the tag instead of re-deriving it.
//
// # Field-ordering contract
//
// The generated input struct declares its fields in exactly the order the
// old Mutate body consumed them: per field, ClearX then X then AppendX; per
// edge, ClearX then the unique/non-unique and create/update ID variants.
// Walking reflect.VisibleFields(structType) in declaration order therefore
// reproduces the old call sequence exactly, as long as the struct's field
// order is left untouched by the generator.
package gqlinput

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"entgo.io/ent"
)

// Mutator is the subset of entbuilder.Mutation[T, I]'s method set that
// mutation inputs need. Declared locally so contrib does not have to import
// (or pin) the ent fork's runtime package. Note the asymmetry: field ops
// take ent.Value, edge ops take plain any — that mirrors the generic
// Mutation's real signatures exactly.
type Mutator interface {
	SetField(name string, value ent.Value) error
	AppendField(name string, value ent.Value) error
	ClearField(name string) error
	SetEdgeID(edge string, id any) error
	AddEdgeIDs(edge string, ids ...any) error
	RemoveEdgeIDs(edge string, ids ...any) error
	ClearEdge(edge string) error
}

// opKind is the resolved dispatch for one struct-tag op.
type opKind int

const (
	opSetField opKind = iota
	opClearField
	opAppendField
	opSetEdgeID
	opAddEdgeIDs
	opRemoveEdgeIDs
	opClearEdge
)

// step is one resolved entry in a plan: a single input struct field, already
// matched to its op and descriptor name, in the field's struct-declaration
// position.
type step struct {
	kind       opKind
	name       string // descriptor field/edge name, from the tag
	fieldIndex int    // guard/value field (ClearField/AddEdgeIDs/etc read this)
	valueIndex int    // opAppendField only: the paired "f" field carrying the value
	nilGuard   bool   // opSetField/opSetEdgeID only: skip when this field is nil
	deref      bool   // dereference the field's pointer before passing it on
}

// plan is the resolved, cached walk order for one concrete input struct
// type.
type plan struct {
	steps []step
}

// plans caches the resolved plan per concrete input struct type, the way
// gqlwhere.Registry.plans does, so per-request work (after the first call
// for a given input type) is an indexed struct-field walk, not repeated tag
// parsing and reflection lookups.
var plans sync.Map // reflect.Type -> *plan

// Mutate applies every set field of input onto m, in struct declaration
// order, driven by each field's `mutate:"<op>:<name>"` tag.
//
// Ops: f (SetField), fc (ClearField), fa (AppendField),
//
//	e (SetEdgeID), ea (AddEdgeIDs), er (RemoveEdgeIDs), ec (ClearEdge).
//
// Errors from the Mutator can only fire on descriptor mismatch — a codegen
// bug, not a runtime condition — so they are discarded, matching the
// behavior of the generated bodies this replaces.
func Mutate(input any, m Mutator) {
	v := reflect.ValueOf(input)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	pl := planFor(v.Type())

	for _, st := range pl.steps {
		fv := v.Field(st.fieldIndex)
		switch st.kind {
		case opSetField:
			if st.nilGuard && fv.IsNil() {
				continue
			}
			_ = m.SetField(st.name, derefIfNeeded(fv, st.deref).Interface())
		case opClearField:
			if fv.Bool() {
				_ = m.ClearField(st.name)
			}
		case opAppendField:
			if fv.IsNil() {
				continue
			}
			src := v.Field(st.valueIndex)
			_ = m.AppendField(st.name, derefIfNeeded(src, st.deref).Interface())
		case opSetEdgeID:
			if st.nilGuard && fv.IsNil() {
				continue
			}
			_ = m.SetEdgeID(st.name, derefIfNeeded(fv, st.deref).Interface())
		case opAddEdgeIDs:
			if fv.Len() == 0 {
				continue
			}
			_ = m.AddEdgeIDs(st.name, toAny(fv)...)
		case opRemoveEdgeIDs:
			if fv.Len() == 0 {
				continue
			}
			_ = m.RemoveEdgeIDs(st.name, toAny(fv)...)
		case opClearEdge:
			if fv.Bool() {
				_ = m.ClearEdge(st.name)
			}
		}
	}
}

// derefIfNeeded returns v.Elem() when deref is set, else v unchanged.
func derefIfNeeded(v reflect.Value, deref bool) reflect.Value {
	if deref {
		return v.Elem()
	}
	return v
}

// toAny boxes a typed slice (e.g. []uuid.UUID) into []any for a variadic
// AddEdgeIDs/RemoveEdgeIDs call, mirroring what the generated bodies did via
// entbuilder.ToAny.
func toAny(v reflect.Value) []any {
	n := v.Len()
	out := make([]any, n)
	for i := 0; i < n; i++ {
		out[i] = v.Index(i).Interface()
	}
	return out
}

// planFor returns the cached plan for structType, building and caching it
// on first use.
func planFor(structType reflect.Type) *plan {
	if cached, ok := plans.Load(structType); ok {
		return cached.(*plan)
	}
	pl := buildPlan(structType)
	actual, _ := plans.LoadOrStore(structType, pl)
	return actual.(*plan)
}

// buildPlan parses every field's `mutate` tag, in struct declaration order,
// into a resolved plan. fFieldIndex tracks the most recently seen "f"-tagged
// field per descriptor name, so a following "fa" field (which the template
// always emits immediately after its paired "f" field) can find the field
// its value is read from.
func buildPlan(structType reflect.Type) *plan {
	pl := &plan{}
	fFieldIndex := make(map[string]int)

	for _, sf := range reflect.VisibleFields(structType) {
		if len(sf.Index) != 1 || !sf.IsExported() {
			continue // not expected in generated (flat) structs; ignore defensively
		}
		tag, ok := sf.Tag.Lookup("mutate")
		if !ok {
			continue
		}
		op, name, ok := strings.Cut(tag, ":")
		if !ok {
			panic(fmt.Sprintf("gqlinput: field %s has malformed mutate tag %q (want \"<op>:<name>\")", sf.Name, tag))
		}

		switch op {
		case "f":
			nilGuard, deref := guardKind(sf.Type)
			pl.steps = append(pl.steps, step{kind: opSetField, name: name, fieldIndex: sf.Index[0], nilGuard: nilGuard, deref: deref})
			fFieldIndex[name] = sf.Index[0]
		case "fc":
			pl.steps = append(pl.steps, step{kind: opClearField, name: name, fieldIndex: sf.Index[0]})
		case "fa":
			srcIdx, ok := fFieldIndex[name]
			if !ok {
				panic(fmt.Sprintf("gqlinput: field %s tagged fa:%s has no preceding f:%s field to append from", sf.Name, name, name))
			}
			_, deref := guardKind(structType.Field(srcIdx).Type)
			pl.steps = append(pl.steps, step{kind: opAppendField, name: name, fieldIndex: sf.Index[0], valueIndex: srcIdx, deref: deref})
		case "e":
			nilGuard, deref := guardKind(sf.Type)
			pl.steps = append(pl.steps, step{kind: opSetEdgeID, name: name, fieldIndex: sf.Index[0], nilGuard: nilGuard, deref: deref})
		case "ea":
			pl.steps = append(pl.steps, step{kind: opAddEdgeIDs, name: name, fieldIndex: sf.Index[0]})
		case "er":
			pl.steps = append(pl.steps, step{kind: opRemoveEdgeIDs, name: name, fieldIndex: sf.Index[0]})
		case "ec":
			pl.steps = append(pl.steps, step{kind: opClearEdge, name: name, fieldIndex: sf.Index[0]})
		default:
			panic(fmt.Sprintf("gqlinput: field %s has unknown mutate op %q", sf.Name, op))
		}
	}
	return pl
}

// guardKind reports, for a SetField/SetEdgeID-eligible field type, whether
// it needs a nil guard before reading it (pointer or nillable — map, slice,
// chan, func, interface) and whether it needs a deref (pointer only).
// A non-pointer, non-nillable type (string, int, bool, a value struct, ...)
// is set unconditionally, matching the template exactly.
func guardKind(t reflect.Type) (nilGuard, deref bool) {
	switch t.Kind() {
	case reflect.Ptr:
		return true, true
	case reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface, reflect.UnsafePointer:
		return true, false
	default:
		return false, false
	}
}
