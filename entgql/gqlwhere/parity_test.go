// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gqlwhere

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"entgo.io/contrib/entgql/internal/todo/ent/schema/durationgql"
	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// parityDurationInput mirrors, field for field and tag for tag, the Duration
// predicate that where_input.tmpl emits for entgql/internal/todo's Category
// entity (see entgql/internal/todo/ent/gql_where_input_category.go). The
// tags are asserted against the real generator by
// TestGeneratedInputDecoderHooks in entgql/internal/todo. The GraphQL type of
// that field is the custom scalar Duration, which
// entgql/internal/todo/gqlgen.yml binds to durationgql (a package of
// Marshal/Unmarshal *functions*), so gqlgen generates
// unmarshalODuration2timeᚐDuration -> durationgql.UnmarshalDuration for it.
type parityDurationInput struct {
	Duration *time.Duration `json:"duration,omitempty" gql:"duration" gqlscalar:"Duration"`
}

// The app owns the binding, so the app registers the coercer, exactly as an
// entgql user would in an init() next to their gqlgen.yml models: block.
func init() {
	RegisterScalar("Duration", func(_ context.Context, v any) (time.Duration, error) {
		return durationgql.UnmarshalDuration(v)
	})
}

// TestParity_FunctionBoundScalar_Duration pins gqlwhere.Decode against the
// unmarshaler gqlgen actually generates for a function-bound custom scalar.
func TestParity_FunctionBoundScalar_Duration(t *testing.T) {
	for _, raw := range []any{"1h", "3600", int64(5), float64(7), json.Number("42")} {
		raw := raw
		t.Run(fmt.Sprintf("%T(%v)", raw, raw), func(t *testing.T) {
			wantV, wantErr := durationgql.UnmarshalDuration(raw)

			var dst parityDurationInput
			err := Decode(context.Background(), "CategoryWhereInput", &dst, map[string]any{"duration": raw})

			if wantErr != nil {
				require.Error(t, err, "gqlgen rejects %#v, decoder accepted it as %v", raw, dst.Duration)
				// Same message, under the same path gqlgen's generated
				// unmarshaler puts it: "input: duration <msg>".
				require.EqualError(t, err, "input: duration "+wantErr.Error())
				return
			}
			require.NoError(t, err, "gqlgen accepts %#v as %v, decoder rejected it", raw, wantV)
			require.NotNil(t, dst.Duration)
			require.Equal(t, wantV, *dst.Duration)
		})
	}
}

// parityUintIDInput is the ID predicate entgql generates for a schema with
// uint IDs. gqlgen's builtin ID binding is graphql.ID/graphql.IntID only
// (config.injectBuiltins), so such an app must add graphql.UintID to
// models.ID.model or codegen fails -- and gqlgen then emits
// UnmarshalUintID, which is not UnmarshalUint.
type parityUintIDInput struct {
	ID uint `json:"id,omitempty" gql:"id" gqlscalar:"ID"`
}

// TestParity_UintID covers I1: resolving ID by Go type reached
// graphql.UnmarshalUint, which silently differs from UnmarshalUintID on a
// negative value (wraparound vs sign error) and on uint32/uint64 inputs.
func TestParity_UintID(t *testing.T) {
	for _, raw := range []any{int(-1), int64(3), uint32(7), uint64(9), "12", json.Number("13")} {
		raw := raw
		t.Run(fmt.Sprintf("%T(%v)", raw, raw), func(t *testing.T) {
			wantV, wantErr := graphql.UnmarshalUintID(raw)

			var dst parityUintIDInput
			err := Decode(context.Background(), "TodoWhereInput", &dst, map[string]any{"id": raw})

			if wantErr != nil {
				require.Error(t, err, "gqlgen rejects %#v, decoder accepted it as %v", raw, dst.ID)
				require.EqualError(t, err, "input: id "+wantErr.Error())
				return
			}
			require.NoError(t, err, "gqlgen accepts %#v as %v, decoder rejected it", raw, wantV)
			require.Equal(t, wantV, dst.ID)
		})
	}
}

// parityCastString is the shape a field.Enum("x").GoType(...) field generates:
// a named string whose Go type carries no UnmarshalGQL, because entgql's
// enum.tmpl gates the generated marshaler on `not $f.HasGoType`. mapScalar
// still gives it the prefixed enum name, so the struct carries a gqlscalar
// tag naming a type that is in nobody's coercer registry.
type parityCastString string

type parityCastInput struct {
	Role *parityCastString `json:"role,omitempty" gql:"role" gqlscalar:"TodoRole"`
}

// TestParity_NamedStringUnderUnknownScalar covers gqlgen's #595 arm
// (codegen/config/binder.go:457-467): for a leaf type whose bound Go model is
// a named string without Marshal/UnmarshalGQL, gqlgen sets CastType to the
// underlying string and emits UnmarshalString plus the cast.
func TestParity_NamedStringUnderUnknownScalar(t *testing.T) {
	for _, raw := range []any{"admin", 7, int64(8), json.Number("42"), true, 1.5, map[string]any{}} {
		raw := raw
		t.Run(fmt.Sprintf("%T(%v)", raw, raw), func(t *testing.T) {
			wantV, wantErr := graphql.UnmarshalString(raw)

			var dst parityCastInput
			err := Decode(context.Background(), "TodoWhereInput", &dst, map[string]any{"role": raw})

			if wantErr != nil {
				require.Error(t, err, "gqlgen rejects %#v, decoder accepted it as %v", raw, dst.Role)
				require.EqualError(t, err, "input: role "+wantErr.Error())
				return
			}
			require.NoError(t, err, "gqlgen accepts %#v as %q, decoder rejected it", raw, wantV)
			require.NotNil(t, dst.Role)
			require.Equal(t, parityCastString(wantV), *dst.Role)
		})
	}
}

// TestParity_UnknownScalarOverNonStringStillErrors pins the narrowness of the
// #595 fallback above: it mirrors that one binder arm, which is String-only.
// An unknown scalar over any other kind is still a loud error -- exactly C1's
// class, where guessing from the Go type produced silently wrong values.
func TestParity_UnknownScalarOverNonStringStillErrors(t *testing.T) {
	var dst struct {
		Dur *time.Duration `gql:"dur" gqlscalar:"UnregisteredDuration"`
	}
	err := Decode(context.Background(), "X", &dst, map[string]any{"dur": "1h"})
	require.ErrorContains(t, err, "no coercer for GraphQL scalar UnregisteredDuration")
}

// TestParity_AppBoundIDScalar covers `models: ID: model: graphql.UUID` in
// gqlgen.yml, which makes every ID field a uuid.UUID. gqlgen resolves that
// through config.Models and emits UnmarshalUUID; the decoder cannot see the
// config, so a known scalar whose Go type is not in gqlgen's default model
// list for that scalar falls back to the Go-type table.
func TestParity_AppBoundIDScalar(t *testing.T) {
	type in struct {
		ID   *uuid.UUID  `gql:"id" gqlscalar:"ID"`
		IDIn []uuid.UUID `gql:"idIn" gqlscalar:"ID"`
	}
	id := uuid.New()
	var dst in
	require.NoError(t, Decode(context.Background(), "In", &dst, map[string]any{
		"id":   id.String(),
		"idIn": []any{id.String()},
	}))
	require.NotNil(t, dst.ID)
	require.Equal(t, id, *dst.ID)
	require.Equal(t, []uuid.UUID{id}, dst.IDIn)

	// Same value through gqlgen's own unmarshaler, for parity.
	want, err := graphql.UnmarshalUUID(id.String())
	require.NoError(t, err)
	require.Equal(t, want, *dst.ID)

	// A bad value must still error, and with gqlgen's text.
	var bad in
	gotErr := Decode(context.Background(), "In", &bad, map[string]any{"id": 42})
	_, wantErr := graphql.UnmarshalUUID(42)
	require.Error(t, gotErr)
	require.Contains(t, gotErr.Error(), wantErr.Error())
}

// TestParity_UnknownScalarStillErrorsAfterFallback pins that the fallback is
// reachable only for a scalar gqlgen knows: an unrecognised scalar name over a
// non-string Go type remains a loud error rather than silently coercing.
func TestParity_UnknownScalarStillErrorsAfterFallback(t *testing.T) {
	type in struct {
		D *time.Duration `gql:"d" gqlscalar:"Furlongs"`
	}
	var dst in
	err := Decode(context.Background(), "In", &dst, map[string]any{"d": "3600"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no coercer for GraphQL scalar Furlongs")
}
