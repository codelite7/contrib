// Package gqlwhere is a reflection-driven replacement for the per-WhereInput
// P() method bodies that entgql/template/where_input_subpkg.tmpl used to
// generate (one ~1200-line function per entity, in the worst observed case).
// A Registry, built once per entity at package-init time from that entity's
// F/E field-handle structs, walks any *XWhereInput value via reflection and
// reproduces exactly what the generated P() body used to do, predicate for
// predicate, in the same order, producing the same emitted SQL text.
//
// # Field-ordering contract
//
// The generated WhereInput struct declares its fields in exactly the order
// the old P() body consumed them: comparable fields (in schema order) times
// their safe ops (in a fixed op order), then filtered edges (in schema
// order). Walking reflect.VisibleFields(structType) in declaration order
// therefore reproduces the predicate sequence — and consequently the
// emitted SQL text — byte for byte, as long as the struct's field order is
// left untouched by the generator.
//
// Four field names are handled by a fixed prologue, not the per-field walk:
// Predicates, Not, Or, And. They are skipped when building the per-field
// plan. Unexported fields (e.g. gemini's entsearch injects searchQuery and
// meilisearchFilters on some WhereInput types for non-predicate purposes)
// are skipped too.
package gqlwhere

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"entgo.io/ent/dialect/sql"
)

// ErrEmpty is the shared parent of every generated ErrEmpty<X>WhereInput.
// Every leaf sentinel returned by NewEmptyError wraps ErrEmpty, so the
// walker tests nested inputs (across entity types, e.g. inside HasXWith)
// with errors.Is(err, ErrEmpty). A caller's own Filter still tests its own
// leaf (via Registry.FilterP, which compares against the specific error
// passed to NewRegistry), so a nested entity's empty error does not satisfy
// the parent's leaf — matching the generated code's behavior exactly.
var ErrEmpty = errors.New("gqlwhere: empty predicate")

// emptyError is a leaf sentinel: distinct per entity (so Registry.FilterP's
// own-leaf comparison stays precise) while wrapping the shared ErrEmpty (so
// errors.Is(err, ErrEmpty) succeeds for any entity's leaf).
type emptyError struct{ msg string }

func (e *emptyError) Error() string { return e.msg }
func (e *emptyError) Unwrap() error { return ErrEmpty }

// NewEmptyError returns a leaf sentinel wrapping ErrEmpty.
func NewEmptyError(msg string) error {
	return &emptyError{msg: msg}
}

// predicateOpNames is the allow-list of handle methods that produce a
// predicate. Filtering by this list (rather than trusting return-type
// assignability alone) is required: Order/Asc/Desc on a field handle return
// entfield.Order, which is also an alias of func(*sql.Selector) — identical
// to P — so a return-type-only filter would register them as bogus ops.
var predicateOpNames = [...]string{
	"EQ", "NEQ", "In", "NotIn", "GT", "GTE", "LT", "LTE",
	"Contains", "HasPrefix", "HasSuffix", "EqualFold", "ContainsFold",
	"IsNil", "NotNil",
}

// argKind classifies how a registered field op's single non-receiver
// parameter (if any) must be read off the WhereInput struct field.
type argKind int

const (
	argBool  argKind = iota // niladic: NumIn() == 0
	argSlice                // variadic: call via CallSlice
	argPtr                  // single-arg: direct or one-level-deref
)

// fieldOp is one registered "<HandleFieldName><MethodName>" (or bare
// "<HandleFieldName>" EQ alias) entry: a bound method value on a field
// handle (e.g. company.Field.SalesforceID.Contains), plus enough about its
// signature to dispatch a WhereInput struct field's value into a call.
type fieldOp struct {
	method  reflect.Value
	kind    argKind
	argType reflect.Type // meaningful for argSlice/argPtr; nil for argBool
}

// edgeOp is one registered "Has<Edge>" or "Has<Edge>With" entry: a bound
// method value on an edge handle (e.g. company.Edge.CreatedBy.HasWith), plus
// its variadic slice/element type for the HasWith case.
type edgeOp struct {
	method    reflect.Value
	isWith    bool
	sliceType reflect.Type // []TP, valid when isWith
}

// Registry holds the reflection tables for one WhereInput type, built once
// at package-init time from that entity's F/E handle structs. It also
// caches, per concrete WhereInput struct type it is asked to walk, the
// resolved field-to-op plan — so per-request work (after the first call for
// a given input type) is an indexed struct-field walk plus reflect calls,
// not repeated method lookups.
type Registry[P ~func(*sql.Selector)] struct {
	fieldOps map[string]fieldOp
	edgeOps  map[string]edgeOp
	not      func(P) P
	and      func(...P) P
	or       func(...P) P
	empty    error

	plans sync.Map // reflect.Type -> *plan
}

// step is one resolved entry in a plan: a single WhereInput struct field,
// already matched to its registered op and (for field ops) its dispatch
// mode, in the field's struct-declaration position.
type step struct {
	fieldIndex int
	name       string // the WhereInput struct field name, used in error wrapping
	isEdge     bool
	field      fieldOp
	edge       edgeOp
	deref      bool // argPtr only: dereference the field value before the call
	// pMethod is the method index of P on the element type of a HasXWith
	// slice field ([]*XWhereInput -> *XWhereInput). Resolved here so the
	// per-element hot loop never does a name lookup.
	pMethod int
}

// plan is the resolved, cached walk order for one concrete WhereInput
// struct type.
type plan struct {
	predicatesIdx int // -1 if absent
	notIdx        int
	orIdx         int
	andIdx        int
	steps         []step
}

// NewRegistry builds the tables once, at package-init time.
//
//	fields, edges: the entity's F and E handle structs (passed by value)
//	not, and, or:  the entity package's Not/And/Or combinators
//	empty:         this entity's leaf sentinel (see NewEmptyError)
func NewRegistry[P ~func(*sql.Selector)](
	fields, edges any,
	not func(P) P, and func(...P) P, or func(...P) P,
	empty error,
) *Registry[P] {
	var zero P
	pType := reflect.TypeOf(zero)

	r := &Registry[P]{
		fieldOps: buildFieldOps(fields, pType),
		edgeOps:  buildEdgeOps(edges, pType),
		not:      not,
		and:      and,
		or:       or,
		empty:    empty,
	}
	return r
}

// buildFieldOps reflects over a fields (F) struct value and registers every
// allow-listed predicate method found on each handle field.
func buildFieldOps(fields any, pType reflect.Type) map[string]fieldOp {
	ops := make(map[string]fieldOp)
	v := reflect.ValueOf(fields)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		handleName := t.Field(i).Name
		handle := v.Field(i)
		for _, opName := range predicateOpNames {
			m := handle.MethodByName(opName)
			if !m.IsValid() {
				continue
			}
			mt := m.Type()
			if mt.NumOut() != 1 || !mt.Out(0).AssignableTo(pType) {
				panic(fmt.Sprintf("gqlwhere: NewRegistry: %s.%s does not return a single predicate assignable to the registry's P type", handleName, opName))
			}
			var fo fieldOp
			switch {
			case mt.NumIn() == 0:
				fo = fieldOp{method: m, kind: argBool}
			case mt.IsVariadic():
				fo = fieldOp{method: m, kind: argSlice, argType: mt.In(0)}
			case mt.NumIn() == 1:
				fo = fieldOp{method: m, kind: argPtr, argType: mt.In(0)}
			default:
				panic(fmt.Sprintf("gqlwhere: NewRegistry: %s.%s has an unsupported signature %s (want niladic, variadic, or single-arg)", handleName, opName, mt))
			}
			if opName == "EQ" {
				// The template names the EQ filter without a suffix
				// (e.g. "name_eq" -> "name"), so register the bare
				// handle name as an alias in addition to the full key.
				ops[handleName] = fo
			}
			ops[handleName+opName] = fo
		}
	}
	return ops
}

// buildEdgeOps reflects over an edges (E) struct value and registers each
// edge handle's Has and HasWith methods.
func buildEdgeOps(edges any, pType reflect.Type) map[string]edgeOp {
	ops := make(map[string]edgeOp)
	v := reflect.ValueOf(edges)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		edgeName := t.Field(i).Name
		handle := v.Field(i)

		has := handle.MethodByName("Has")
		hasWith := handle.MethodByName("HasWith")
		if !has.IsValid() || !hasWith.IsValid() {
			panic(fmt.Sprintf("gqlwhere: NewRegistry: edge %s is missing Has/HasWith", edgeName))
		}
		hmt := has.Type()
		if hmt.NumIn() != 0 || hmt.NumOut() != 1 || !hmt.Out(0).AssignableTo(pType) {
			panic(fmt.Sprintf("gqlwhere: NewRegistry: edge %s.Has has an unsupported signature %s", edgeName, hmt))
		}
		hwmt := hasWith.Type()
		if !hwmt.IsVariadic() || hwmt.NumIn() != 1 || hwmt.NumOut() != 1 || !hwmt.Out(0).AssignableTo(pType) {
			panic(fmt.Sprintf("gqlwhere: NewRegistry: edge %s.HasWith has an unsupported signature %s", edgeName, hwmt))
		}

		ops["Has"+edgeName] = edgeOp{method: has, isWith: false}
		ops["Has"+edgeName+"With"] = edgeOp{method: hasWith, isWith: true, sliceType: hwmt.In(0)}
	}
	return ops
}

// planFor returns the cached plan for structType, building and caching it
// on first use.
func (r *Registry[P]) planFor(structType reflect.Type) *plan {
	if cached, ok := r.plans.Load(structType); ok {
		return cached.(*plan)
	}

	pl := &plan{predicatesIdx: -1, notIdx: -1, orIdx: -1, andIdx: -1}
	for _, sf := range reflect.VisibleFields(structType) {
		if len(sf.Index) != 1 {
			continue // not expected in generated (flat) structs; ignore defensively
		}
		// The four prologue fields are validated here rather than trusted
		// at walk time, so a shape drift in the generated struct fails
		// eagerly in Warm like every other binding mismatch instead of
		// silently dropping predicates (Predicates) or nil-dereferencing
		// (Not/Or/And) on the first request.
		switch sf.Name {
		case "Predicates":
			if want := reflect.TypeFor[[]P](); sf.Type != want {
				panic(fmt.Sprintf("gqlwhere: Predicates: struct field type %s is not %s", sf.Type, want))
			}
			pl.predicatesIdx = sf.Index[0]
			continue
		case "Not":
			if sf.Type.Kind() != reflect.Ptr {
				panic(fmt.Sprintf("gqlwhere: Not: struct field type %s is not a pointer", sf.Type))
			}
			pl.notIdx = sf.Index[0]
			continue
		case "Or", "And":
			if sf.Type.Kind() != reflect.Slice {
				panic(fmt.Sprintf("gqlwhere: %s: struct field type %s is not a slice", sf.Name, sf.Type))
			}
			if sf.Name == "Or" {
				pl.orIdx = sf.Index[0]
			} else {
				pl.andIdx = sf.Index[0]
			}
			continue
		}
		if !sf.IsExported() {
			continue
		}

		if fo, ok := r.fieldOps[sf.Name]; ok {
			st := step{fieldIndex: sf.Index[0], name: sf.Name, field: fo}
			switch fo.kind {
			case argPtr:
				switch {
				case sf.Type == fo.argType:
					st.deref = false
				case sf.Type.Kind() == reflect.Ptr && sf.Type.Elem() == fo.argType:
					st.deref = true
				default:
					panic(fmt.Sprintf("gqlwhere: %s: struct field type %s is incompatible with op arg type %s", sf.Name, sf.Type, fo.argType))
				}
			case argSlice:
				if !sf.Type.AssignableTo(fo.argType) {
					panic(fmt.Sprintf("gqlwhere: %s: struct field type %s is not assignable to op arg type %s", sf.Name, sf.Type, fo.argType))
				}
			case argBool:
				if sf.Type.Kind() != reflect.Bool {
					panic(fmt.Sprintf("gqlwhere: %s: struct field type %s is not bool", sf.Name, sf.Type))
				}
			}
			pl.steps = append(pl.steps, st)
			continue
		}

		if eo, ok := r.edgeOps[sf.Name]; ok {
			st := step{fieldIndex: sf.Index[0], name: sf.Name, isEdge: true, edge: eo}
			if eo.isWith {
				if sf.Type.Kind() != reflect.Slice {
					panic(fmt.Sprintf("gqlwhere: %s: struct field type %s is not a slice", sf.Name, sf.Type))
				}
				m, ok := sf.Type.Elem().MethodByName("P")
				if !ok {
					panic(fmt.Sprintf("gqlwhere: %s: %s has no P method", sf.Name, sf.Type.Elem()))
				}
				st.pMethod = m.Index
			}
			pl.steps = append(pl.steps, st)
			continue
		}

		panic(fmt.Sprintf("gqlwhere: no registered predicate or edge for field %q", sf.Name))
	}

	actual, _ := r.plans.LoadOrStore(structType, pl)
	return actual.(*plan)
}

// Warm eagerly resolves and caches the plan for prototype's element type, so
// a binding mismatch (a WhereInput struct field with no matching registered
// op, or an incompatible type) panics at package-init time rather than on
// the first request that walks that type. prototype is a typed nil, e.g.
// (*CompanyWhereInput)(nil). Returns r for chaining onto a NewRegistry call.
func (r *Registry[P]) Warm(prototype any) *Registry[P] {
	t := reflect.TypeOf(prototype)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	r.planFor(t)
	return r
}

// isNilable reports whether v's kind supports IsNil.
func isNilable(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return true
	default:
		return false
	}
}

// combine implements the shared Or/And prologue shape: vals is the
// []*XWhereInput slice value (i.Or or i.And); fieldName is "or" or "and",
// used only in the wrapped-error text; agg is r.or or r.and. n==1 unwraps
// the single child directly (agg is never called); n>1 resolves every
// child, silently dropping ones whose own error is r.empty, and calls agg
// on the survivors when at least one remains. ok reports whether p should
// be appended to the caller's predicates; it is false both when vals is
// empty and when every child collapsed to r.empty.
func (r *Registry[P]) combine(vals reflect.Value, fieldName string, agg func(...P) P) (p P, ok bool, err error) {
	switch n := vals.Len(); {
	case n == 1:
		child, err := r.P(vals.Index(0).Interface())
		if err != nil {
			if !errors.Is(err, r.empty) {
				return p, false, fmt.Errorf("%w: field '%s'", err, fieldName)
			}
			return p, false, nil
		}
		return child, true, nil
	case n > 1:
		children := make([]P, 0, n)
		for j := 0; j < n; j++ {
			child, err := r.P(vals.Index(j).Interface())
			if err != nil {
				if !errors.Is(err, r.empty) {
					return p, false, fmt.Errorf("%w: field '%s'", err, fieldName)
				}
				continue
			}
			children = append(children, child)
		}
		if len(children) > 0 {
			return agg(children...), true, nil
		}
	}
	return p, false, nil
}

// P walks input (a *XWhereInput) and returns the combined predicate.
// Returns (nil, r.empty) when nothing was set.
func (r *Registry[P]) P(input any) (P, error) {
	var zero P

	v := reflect.ValueOf(input)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return zero, r.empty
		}
		v = v.Elem()
	}
	pl := r.planFor(v.Type())

	var predicates []P

	if pl.notIdx >= 0 {
		if notVal := v.Field(pl.notIdx); !notVal.IsNil() {
			p, err := r.P(notVal.Interface())
			if err != nil {
				if !errors.Is(err, r.empty) {
					return zero, fmt.Errorf("%w: field 'not'", err)
				}
			} else {
				predicates = append(predicates, r.not(p))
			}
		}
	}

	if pl.orIdx >= 0 {
		p, ok, err := r.combine(v.Field(pl.orIdx), "or", r.or)
		if err != nil {
			return zero, err
		}
		if ok {
			predicates = append(predicates, p)
		}
	}

	if pl.andIdx >= 0 {
		p, ok, err := r.combine(v.Field(pl.andIdx), "and", r.and)
		if err != nil {
			return zero, err
		}
		if ok {
			predicates = append(predicates, p)
		}
	}

	if pl.predicatesIdx >= 0 {
		// planFor already proved the field's type is []P, so this asserts
		// rather than testing-and-dropping: a mismatch is a binding bug and
		// must be as loud as every other one, not a silent loss of the
		// user's AddPredicates calls.
		predicates = append(predicates, v.Field(pl.predicatesIdx).Interface().([]P)...)
	}

	for _, st := range pl.steps {
		fv := v.Field(st.fieldIndex)
		if st.isEdge {
			if !st.edge.isWith {
				if fv.IsNil() {
					continue
				}
				p := st.edge.method.Call(nil)[0].Interface().(P)
				if !fv.Elem().Bool() {
					p = r.not(p)
				}
				predicates = append(predicates, p)
				continue
			}

			n := fv.Len()
			if n == 0 {
				continue
			}
			with := reflect.MakeSlice(st.edge.sliceType, 0, n)
			hasEmpty := false
			for j := 0; j < n; j++ {
				elem := fv.Index(j)
				results := elem.Method(st.pMethod).Call(nil)
				if errVal := results[1]; !errVal.IsNil() {
					err := errVal.Interface().(error)
					if errors.Is(err, ErrEmpty) {
						// An empty nested predicate (e.g. hasXWith: {idIn:[]})
						// means "IN empty set" — nothing can match. Emit a
						// never-match predicate instead of propagating the
						// error past the caller's own empty-predicate guard.
						hasEmpty = true
						break
					}
					return zero, fmt.Errorf("%w: field '%s'", err, st.name)
				}
				with = reflect.Append(with, results[0])
			}
			if hasEmpty {
				predicates = append(predicates, P(func(s *sql.Selector) {
					s.Where(sql.False())
				}))
			} else {
				p := st.edge.method.CallSlice([]reflect.Value{with})[0].Interface().(P)
				predicates = append(predicates, p)
			}
			continue
		}

		switch st.field.kind {
		case argBool:
			if fv.Bool() {
				p := st.field.method.Call(nil)[0].Interface().(P)
				predicates = append(predicates, p)
			}
		case argSlice:
			if fv.Len() > 0 {
				p := st.field.method.CallSlice([]reflect.Value{fv})[0].Interface().(P)
				predicates = append(predicates, p)
			}
		case argPtr:
			arg := fv
			if st.deref {
				if fv.IsNil() {
					continue
				}
				arg = fv.Elem()
			} else if isNilable(fv) && fv.IsNil() {
				continue
			}
			p := st.field.method.Call([]reflect.Value{arg})[0].Interface().(P)
			predicates = append(predicates, p)
		}
	}

	switch len(predicates) {
	case 0:
		return zero, r.empty
	case 1:
		return predicates[0], nil
	default:
		return r.and(predicates...), nil
	}
}

// FilterP is P with the empty case flattened: returns (nil, nil) when empty.
func (r *Registry[P]) FilterP(input any) (P, error) {
	p, err := r.P(input)
	if err != nil {
		var zero P
		if errors.Is(err, r.empty) {
			return zero, nil
		}
		return zero, err
	}
	return p, nil
}
