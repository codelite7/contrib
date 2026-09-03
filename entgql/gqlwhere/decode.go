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
	// coercers maps a Go type to the gqlgen primitive its generated coercer
	// would call. Keep this the exact list of primitives gqlgen's type.gotpl
	// emits for built-in scalars.
	coercers = map[reflect.Type]Coercer{
		reflect.TypeFor[string]():         func(_ context.Context, v any) (any, error) { return graphql.UnmarshalString(v) },
		reflect.TypeFor[bool]():           func(_ context.Context, v any) (any, error) { return graphql.UnmarshalBoolean(v) },
		reflect.TypeFor[int]():            func(_ context.Context, v any) (any, error) { return graphql.UnmarshalInt(v) },
		reflect.TypeFor[int32]():          func(_ context.Context, v any) (any, error) { return graphql.UnmarshalInt32(v) },
		reflect.TypeFor[int64]():          func(_ context.Context, v any) (any, error) { return graphql.UnmarshalInt64(v) },
		reflect.TypeFor[uint]():           func(_ context.Context, v any) (any, error) { return graphql.UnmarshalUint(v) },
		reflect.TypeFor[uint32]():         func(_ context.Context, v any) (any, error) { return graphql.UnmarshalUint32(v) },
		reflect.TypeFor[uint64]():         func(_ context.Context, v any) (any, error) { return graphql.UnmarshalUint64(v) },
		reflect.TypeFor[float64]():        func(ctx context.Context, v any) (any, error) { return graphql.UnmarshalFloatContext(ctx, v) },
		reflect.TypeFor[time.Time]():      func(_ context.Context, v any) (any, error) { return graphql.UnmarshalTime(v) },
		reflect.TypeFor[uuid.UUID]():      func(_ context.Context, v any) (any, error) { return graphql.UnmarshalUUID(v) },
		reflect.TypeFor[map[string]any](): func(_ context.Context, v any) (any, error) { return graphql.UnmarshalMap(v) },
	}
	// idCoercers are used instead of coercers when a field carries `gqlscalar:"ID"`.
	idCoercers = map[reflect.Type]Coercer{
		reflect.TypeFor[int]():    func(_ context.Context, v any) (any, error) { return graphql.UnmarshalIntID(v) },
		reflect.TypeFor[string](): func(_ context.Context, v any) (any, error) { return graphql.UnmarshalID(v) },
	}
)

// RegisterCoercer registers fn as the coercer for Go type T, for scalars bound
// to gqlgen via marshal/unmarshal functions rather than methods.
//
// RegisterCoercer must be called during package initialization, before the
// first Decode of any struct that has a field of type T: decode plans are
// cached per struct type on first use.
func RegisterCoercer[T any](fn func(ctx context.Context, v any) (T, error)) {
	coercersMu.Lock()
	defer coercersMu.Unlock()
	coercers[reflect.TypeFor[T]()] = func(ctx context.Context, v any) (any, error) { return fn(ctx, v) }
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
}

type decodePlan struct {
	fields []decodeField
	err    error // plan-build error, returned by every Decode call for this type
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
		fn, err := coercerFor(sf.Type, sf.Tag.Get("gqlscalar") == "ID")
		if err != nil {
			p.err = fmt.Errorf("gqlwhere: %w (field %q); register one with gqlwhere.RegisterCoercer", err, name)
			return p
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

// coercerFor resolves the decodeFn for t, then — for any Go type gqlgen
// treats as nilable (map, slice, pointer, interface) — wraps it so an
// explicit GraphQL null short-circuits to the zero value instead of reaching
// the inner coercer. This mirrors gqlgen's generated `if v == nil { return
// nil, nil }` guard in type.gotpl, emitted for every nilable scalar/list.
func coercerFor(t reflect.Type, isID bool) (decodeFn, error) {
	fn, err := buildCoercer(t, isID)
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

func buildCoercer(t reflect.Type, isID bool) (decodeFn, error) {
	// Methods first: enums, custom scalars, nested inputs.
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
	if isID {
		if c, ok := idCoercers[t]; ok {
			return scalarFn(t, c), nil
		}
	}
	coercersMu.RLock()
	c, ok := coercers[t]
	coercersMu.RUnlock()
	if ok {
		return scalarFn(t, c), nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		elem, err := coercerFor(t.Elem(), isID)
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
		return sliceCoercer(t, isID) // Task 3
	case reflect.Struct:
		return func(ctx context.Context, v any) (reflect.Value, error) {
			rv := reflect.New(t)
			err := Decode(ctx, t.Name(), rv.Interface(), v)
			return rv.Elem(), graphql.ErrorOnPath(ctx, err)
		}, nil
	}
	// Named type over a basic kind without methods: gqlgen casts the underlying scalar.
	if t.PkgPath() != "" {
		coercersMu.RLock()
		uc, ok := coercers[underlyingType(t)]
		coercersMu.RUnlock()
		if ok {
			return func(ctx context.Context, v any) (reflect.Value, error) {
				out, err := uc(ctx, v)
				if err != nil {
					return reflect.Value{}, graphql.ErrorOnPath(ctx, err)
				}
				return reflect.ValueOf(out).Convert(t), nil
			}, nil
		}
	}
	return nil, fmt.Errorf("no coercer for Go type %s", t)
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
	if p.err != nil {
		return p.err
	}
	for _, f := range p.fields {
		raw, present := asMap[f.name]
		if !present {
			continue
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
// generated list coercers: a non-[]any value is treated as a one-element
// list, and each element is coerced under a path context carrying its index.
func sliceCoercer(t reflect.Type, isID bool) (decodeFn, error) {
	elem, err := coercerFor(t.Elem(), isID)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, v any) (reflect.Value, error) {
		if v == nil {
			return reflect.Zero(t), nil
		}
		items, ok := v.([]any)
		if !ok {
			items = []any{v}
		}
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
