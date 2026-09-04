package gqlwhere

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
)

// Coercer converts a raw GraphQL input value (as produced by the request
// decoder: string, json.Number/float64/int, bool, map[string]any, []any, nil)
// into a Go value of the registered type.
type Coercer func(ctx context.Context, v any) (any, error)

var (
	coercersMu sync.RWMutex
	// scalars maps a GraphQL scalar name to the gqlgen coercer bound to it,
	// keyed by the Go type of the bound model. This mirrors how gqlgen picks an
	// unmarshaler: config.Models[<GraphQL type>].Model names a Go object, and
	// binder.TypeReference walks that list picking the model whose Go type is
	// compatible with the field's (codegen/config/binder.go). Resolving by Go
	// type alone cannot work: two GraphQL scalars bound to the same Go type
	// (Duration and Int64 both over time.Duration/int64) are indistinguishable.
	//
	// Seeded with exactly the bindings config.injectBuiltins installs
	// (codegen/config/config.go), which is the source of truth this table
	// shadows. Names outside it belong to the app's gqlgen.yml, so the app
	// registers them with RegisterScalar.
	scalars = map[string]map[reflect.Type]Coercer{
		// injectBuiltins: unconditional builtins.
		"Float":   {reflect.TypeFor[float64](): func(ctx context.Context, v any) (any, error) { return graphql.UnmarshalFloatContext(ctx, v) }},
		"String":  {reflect.TypeFor[string](): coercer(graphql.UnmarshalString)},
		"Boolean": {reflect.TypeFor[bool](): coercer(graphql.UnmarshalBoolean)},
		"Int": {
			reflect.TypeFor[int]():   coercer(graphql.UnmarshalInt),
			reflect.TypeFor[int32](): coercer(graphql.UnmarshalInt32),
			reflect.TypeFor[int64](): coercer(graphql.UnmarshalInt64),
		},
		"ID": {
			reflect.TypeFor[string](): coercer(graphql.UnmarshalID),
			reflect.TypeFor[int]():    coercer(graphql.UnmarshalIntID),
			// Not in injectBuiltins: an app with uint IDs must add
			// graphql.UintID to models.ID.model itself or gqlgen codegen
			// fails, and gqlgen then emits UnmarshalUintID -- which differs
			// from UnmarshalUint on -1 (silent wraparound vs error), on nil,
			// and on uint32/uint64 inputs.
			reflect.TypeFor[uint](): coercer(graphql.UnmarshalUintID),
		},
		// injectBuiltins: extraBuiltins, injected when the schema declares the
		// scalar. entgql emits Time and Map itself; the rest are here because
		// gqlgen binds them the moment an app declares them.
		"Int64": {
			reflect.TypeFor[int]():   coercer(graphql.UnmarshalInt),
			reflect.TypeFor[int64](): coercer(graphql.UnmarshalInt64),
		},
		"Time":   {reflect.TypeFor[time.Time](): coercer(graphql.UnmarshalTime)},
		"Map":    {reflect.TypeFor[map[string]any](): coercer(graphql.UnmarshalMap)},
		"Upload": {reflect.TypeFor[graphql.Upload](): coercer(graphql.UnmarshalUpload)},
		"Any":    {reflect.TypeFor[any](): coercer(graphql.UnmarshalAny)},
		// Not injected by gqlgen, but graphql.Uint/Uint32/Uint64 is the
		// conventional binding for an app-declared Uint* scalar (entgql's own
		// internal/todo fixture does exactly that for Uint64), and the binder
		// resolves Marshal<Name>/Unmarshal<Name> from that model string.
		"Uint":   {reflect.TypeFor[uint](): coercer(graphql.UnmarshalUint)},
		"Uint32": {reflect.TypeFor[uint32](): coercer(graphql.UnmarshalUint32)},
		"Uint64": {reflect.TypeFor[uint64](): coercer(graphql.UnmarshalUint64)},
	}
	// goCoercers resolves a struct field that carries no gqlscalar tag, i.e. a
	// hand-written input struct rather than one entgql generated. It is a
	// best-effort Go-type table: it cannot tell two scalars bound to the same
	// Go type apart, which is why generated structs carry the tag.
	goCoercers = map[reflect.Type]Coercer{
		reflect.TypeFor[string]():         coercer(graphql.UnmarshalString),
		reflect.TypeFor[bool]():           coercer(graphql.UnmarshalBoolean),
		reflect.TypeFor[int]():            coercer(graphql.UnmarshalInt),
		reflect.TypeFor[int32]():          coercer(graphql.UnmarshalInt32),
		reflect.TypeFor[int64]():          coercer(graphql.UnmarshalInt64),
		reflect.TypeFor[uint]():           coercer(graphql.UnmarshalUint),
		reflect.TypeFor[uint32]():         coercer(graphql.UnmarshalUint32),
		reflect.TypeFor[uint64]():         coercer(graphql.UnmarshalUint64),
		reflect.TypeFor[float64]():        func(ctx context.Context, v any) (any, error) { return graphql.UnmarshalFloatContext(ctx, v) },
		reflect.TypeFor[time.Time]():      coercer(graphql.UnmarshalTime),
		reflect.TypeFor[uuid.UUID]():      coercer(graphql.UnmarshalUUID),
		reflect.TypeFor[map[string]any](): coercer(graphql.UnmarshalMap),
	}
)

// coercer adapts a context-free gqlgen unmarshaler to a Coercer.
func coercer[T any](fn func(any) (T, error)) Coercer {
	return func(_ context.Context, v any) (any, error) { return fn(v) }
}

// RegisterScalar registers fn as the coercer for the GraphQL scalar named
// name, bound to Go type T. Use it for every scalar the app binds in
// gqlgen.yml to marshal/unmarshal functions rather than to a type with
// UnmarshalGQL methods -- gqlgen resolves those by GraphQL name, so the
// decoder must too:
//
//	models:
//	  Duration:
//	    model: [example.com/app/durationgql.Duration]
//
//	gqlwhere.RegisterScalar("Duration", func(_ context.Context, v any) (time.Duration, error) {
//		return durationgql.UnmarshalDuration(v)
//	})
//
// RegisterScalar must be called during package initialization, before the
// first Decode of any struct with a field of that scalar: decode plans are
// cached per struct type on first use.
func RegisterScalar[T any](name string, fn func(ctx context.Context, v any) (T, error)) {
	coercersMu.Lock()
	defer coercersMu.Unlock()
	byGo := scalars[name]
	if byGo == nil {
		byGo = make(map[reflect.Type]Coercer, 1)
		scalars[name] = byGo
	}
	byGo[reflect.TypeFor[T]()] = func(ctx context.Context, v any) (any, error) { return fn(ctx, v) }
}

var (
	ctxUnmarshalerT = reflect.TypeFor[graphql.ContextUnmarshaler]()
	unmarshalerT    = reflect.TypeFor[graphql.Unmarshaler]()
)

// decodeFn coerces v into a reflect.Value of a fixed Go type.
type decodeFn func(ctx context.Context, v any) (reflect.Value, error)

type decodeField struct {
	name  string
	index int
	fn    decodeFn
	err   error // set instead of fn when the field's Go type has no coercer; returned only if the field is present in the input.
}

type decodePlan struct {
	fields []decodeField
}

var plans sync.Map // reflect.Type → *decodePlan

func planFor(t reflect.Type) *decodePlan {
	if p, ok := plans.Load(t); ok {
		return p.(*decodePlan)
	}
	p := buildPlan(t)
	actual, _ := plans.LoadOrStore(t, p)
	return actual.(*decodePlan)
}

func buildPlan(t reflect.Type) *decodePlan {
	p := &decodePlan{}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		name := gqlFieldName(sf)
		if name == "" {
			continue
		}
		fn, err := coercerFor(sf.Type, sf.Tag.Get("gqlscalar"))
		if err != nil {
			p.fields = append(p.fields, decodeField{
				name:  name,
				index: i,
				err:   fmt.Errorf("gqlwhere: %w (field %q); register one with gqlwhere.RegisterScalar", err, name),
			})
			continue
		}
		p.fields = append(p.fields, decodeField{name: name, index: i, fn: fn})
	}
	return p
}

// gqlFieldName returns the GraphQL field name for a struct field: the `gql`
// tag, else the name part of the `json` tag. "" means "not a GraphQL field".
func gqlFieldName(sf reflect.StructField) string {
	if n := sf.Tag.Get("gql"); n != "" {
		return n
	}
	j, ok := sf.Tag.Lookup("json")
	if !ok {
		return ""
	}
	if i := strings.IndexByte(j, ','); i >= 0 {
		j = j[:i]
	}
	if j == "-" {
		return ""
	}
	return j
}

// coercerFor resolves the decodeFn for t under the GraphQL scalar named by the
// field's gqlscalar tag ("" when the field carries none), then — for any Go
// type gqlgen treats as nilable (map, slice, pointer, interface) — wraps it so an
// explicit GraphQL null short-circuits to the zero value instead of reaching
// the inner coercer. This mirrors gqlgen's generated `if v == nil { return
// nil, nil }` guard in type.gotpl, emitted for every nilable scalar/list.
func coercerFor(t reflect.Type, scalar string) (decodeFn, error) {
	fn, err := buildCoercer(t, scalar)
	if err != nil {
		return nil, err
	}
	switch t.Kind() {
	case reflect.Map, reflect.Slice, reflect.Pointer, reflect.Interface:
		inner := fn
		return func(ctx context.Context, v any) (reflect.Value, error) {
			if v == nil {
				return reflect.Zero(t), nil
			}
			return inner(ctx, v)
		}, nil
	}
	return fn, nil
}

func buildCoercer(t reflect.Type, scalar string) (decodeFn, error) {
	// Methods first: enums, custom scalars, nested inputs. gqlgen's type.gotpl
	// takes the IsMarshaler arm ahead of the scalar path too.
	if reflect.PointerTo(t).Implements(ctxUnmarshalerT) {
		return func(ctx context.Context, v any) (reflect.Value, error) {
			rv := reflect.New(t)
			err := rv.Interface().(graphql.ContextUnmarshaler).UnmarshalGQLContext(ctx, v)
			return rv.Elem(), graphql.ErrorOnPath(ctx, err)
		}, nil
	}
	if reflect.PointerTo(t).Implements(unmarshalerT) {
		return func(ctx context.Context, v any) (reflect.Value, error) {
			rv := reflect.New(t)
			err := rv.Interface().(graphql.Unmarshaler).UnmarshalGQL(v)
			return rv.Elem(), graphql.ErrorOnPath(ctx, err)
		}, nil
	}
	c, ok, bound := lookup(t, scalar)
	if ok {
		// scalarFn converts, which is the cast gqlgen applies for a named type
		// over the bound model's basic type (binder.go's CastType arm).
		return scalarFn(t, c), nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		// Wrappers carry the scalar down to the element: gqlgen's type.gotpl
		// resolves the unmarshaler from the leaf named type either way.
		elem, err := coercerFor(t.Elem(), scalar)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, v any) (reflect.Value, error) {
			if v == nil {
				return reflect.Zero(t), nil
			}
			ev, err := elem(ctx, v)
			if err != nil {
				return reflect.Value{}, err
			}
			pv := reflect.New(t.Elem())
			pv.Elem().Set(ev)
			return pv, nil
		}, nil
	case reflect.Slice:
		return sliceCoercer(t, scalar)
	case reflect.Struct:
		if !bound {
			// An input object, not a scalar: gqlgen generates unmarshalInput<X>.
			return func(ctx context.Context, v any) (reflect.Value, error) {
				rv := reflect.New(t)
				err := Decode(ctx, t.Name(), rv.Interface(), v)
				return rv.Elem(), graphql.ErrorOnPath(ctx, err)
			}, nil
		}
	}
	if bound {
		// The scalar is known but not bound to this Go type; gqlgen would have
		// failed codegen rather than silently picking another unmarshaler.
		return nil, fmt.Errorf("GraphQL scalar %s is not bound to Go type %s", scalar, t)
	}
	if scalar != "" {
		return nil, fmt.Errorf("no coercer for GraphQL scalar %s (Go type %s)", scalar, t)
	}
	return nil, fmt.Errorf("no coercer for Go type %s", t)
}

// lookup resolves the coercer for Go type t under GraphQL scalar name scalar,
// the way binder.TypeReference does: among the models bound to that scalar,
// the one whose Go type matches the field's -- exactly, or as the basic type a
// named type wraps. An empty scalar name (no gqlscalar tag, i.e. a
// hand-written struct) falls back to the Go-type table.
//
// bound reports that the scalar name is one the decoder knows, so a miss is a
// real binding mismatch rather than "this is an input object or an enum".
func lookup(t reflect.Type, scalar string) (c Coercer, ok, bound bool) {
	coercersMu.RLock()
	defer coercersMu.RUnlock()
	byGo := goCoercers
	if scalar != "" {
		if byGo, bound = scalars[scalar]; !bound {
			return nil, false, false
		}
	}
	if c, ok = byGo[t]; ok {
		return c, true, bound
	}
	if t.PkgPath() != "" {
		// Named type over a basic kind without methods: gqlgen casts the
		// underlying scalar.
		if c, ok = byGo[underlyingType(t)]; ok {
			return c, true, bound
		}
	}
	return nil, false, bound
}

func scalarFn(t reflect.Type, c Coercer) decodeFn {
	return func(ctx context.Context, v any) (reflect.Value, error) {
		out, err := c(ctx, v)
		if err != nil {
			return reflect.Value{}, graphql.ErrorOnPath(ctx, err)
		}
		return reflect.ValueOf(out).Convert(t), nil
	}
}

// underlyingType maps a named basic type (type Status string) to its kind's
// canonical type so it can be looked up in the coercer table.
func underlyingType(t reflect.Type) reflect.Type {
	switch t.Kind() {
	case reflect.String:
		return reflect.TypeFor[string]()
	case reflect.Bool:
		return reflect.TypeFor[bool]()
	case reflect.Int:
		return reflect.TypeFor[int]()
	case reflect.Int32:
		return reflect.TypeFor[int32]()
	case reflect.Int64:
		return reflect.TypeFor[int64]()
	case reflect.Uint:
		return reflect.TypeFor[uint]()
	case reflect.Uint32:
		return reflect.TypeFor[uint32]()
	case reflect.Uint64:
		return reflect.TypeFor[uint64]()
	case reflect.Float64:
		return reflect.TypeFor[float64]()
	}
	return t
}

// Decode fills the struct behind dst from v exactly the way gqlgen's generated
// unmarshalInput<gqlName> function would: nil leaves the zero value, an
// already-typed value is copied through, a non-map is an error, fields are
// visited in struct order under a per-field path context, and the first
// coercion error is returned wrapped with graphql.ErrorOnPath.
func Decode(ctx context.Context, gqlName string, dst any, v any) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("gqlwhere: Decode dst must be a non-nil pointer to a struct, got %T", dst)
	}
	target := rv.Elem()
	if v == nil {
		return nil
	}
	switch tv := reflect.ValueOf(v); {
	case tv.Type() == target.Type():
		target.Set(tv)
		return nil
	case tv.Type() == rv.Type() && !tv.IsNil():
		target.Set(tv.Elem())
		return nil
	}
	asMap, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("unmarshalInput%s: expected map[string]any, got %T", gqlName, v)
	}
	p := planFor(target.Type())
	for _, f := range p.fields {
		raw, present := asMap[f.name]
		if !present {
			continue
		}
		if f.err != nil {
			return f.err
		}
		fctx := graphql.WithPathContext(ctx, graphql.NewPathWithField(f.name))
		val, err := f.fn(fctx, raw)
		if err != nil {
			return graphql.ErrorOnPath(fctx, err)
		}
		target.Field(f.index).Set(val)
	}
	return nil
}

// sliceCoercer builds a decodeFn for a slice type t, matching gqlgen's
// generated list coercers exactly: they call graphql.CoerceList(v), and this
// decoder must produce byte-identical struct values, error text, and error
// paths to gqlgen's generated unmarshalers, so it delegates to the same
// function rather than reimplementing its rule.
//
// CoerceList's actual behavior, read from graphql/coercion.go in the gqlgen
// version this module resolves (v0.17.68) and confirmed against its one call
// site in codegen/type.gotpl: a []any passes through unchanged; nine named
// concrete types ([]string, []json.Number, []bool, []map[string]any,
// []float64, []float32, []int, []int32, []int64) collapse to a list holding
// only their first element (empty input yields an empty list) — silently
// dropping every other element, an apparent quirk of gqlgen's own, not a
// deliberate list-of-lists idiom; every other value, including any other
// pre-typed slice or array type outside those nine, is wrapped whole as a
// single list element. nil yields the zero value, handled below before
// CoerceList is reached.
//
// We reproduce this deliberately, quirk included: parity with gqlgen is the
// constraint this decoder exists to satisfy, validated by a differential
// corpus, not by tests written to a preferred output. Changing the quirk
// (e.g. fully iterating the nine named types instead of truncating them) is
// a spec-level decision that would need that differential corpus extended
// first, not a call for this decoder to make unilaterally.
func sliceCoercer(t reflect.Type, scalar string) (decodeFn, error) {
	elem, err := coercerFor(t.Elem(), scalar)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, v any) (reflect.Value, error) {
		if v == nil {
			return reflect.Zero(t), nil
		}
		items := graphql.CoerceList(v)
		out := reflect.MakeSlice(t, len(items), len(items))
		for i, item := range items {
			ictx := graphql.WithPathContext(ctx, graphql.NewPathWithIndex(i))
			ev, err := elem(ictx, item)
			if err != nil {
				return reflect.Value{}, graphql.ErrorOnPath(ictx, err)
			}
			out.Index(i).Set(ev)
		}
		return out, nil
	}, nil
}
