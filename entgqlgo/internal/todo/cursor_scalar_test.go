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

package todo

import (
	"testing"

	"entgo.io/contrib/entgqlgo"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/gqlgo"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestCursorScalarSerializeIsIDTypeAgnostic guards the generated Cursor scalar's
// Serialize against the regression where it only handled Cursor[int] /
// Cursor[string] and returned null for every other node ID type — most notably
// Cursor[uuid.UUID], which produced "Cannot return null for non-nullable field
// <Type>Edge.cursor" on any UUID-keyed schema.
//
// Go generics make each Cursor[T] a distinct type, so the scalar must serialize
// via the fmt.Stringer interface (Cursor[T] has a value-receiver String()) to
// stay ID-type-agnostic. This test exercises representative T's directly so the
// guarantee holds even though the bundled test schemas are all int-keyed.
func TestCursorScalarSerializeIsIDTypeAgnostic(t *testing.T) {
	id := uuid.New()

	cases := []struct {
		name  string
		value interface{}
	}{
		{"Cursor[int] value", entgqlgo.Cursor[int]{ID: 7}},
		{"Cursor[int] pointer", &entgqlgo.Cursor[int]{ID: 7}},
		{"Cursor[string] value", entgqlgo.Cursor[string]{ID: "k"}},
		{"Cursor[string] pointer", &entgqlgo.Cursor[string]{ID: "k"}},
		{"Cursor[uuid.UUID] value", entgqlgo.Cursor[uuid.UUID]{ID: id}},
		{"Cursor[uuid.UUID] pointer", &entgqlgo.Cursor[uuid.UUID]{ID: id}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := gqlgo.CursorScalar.Serialize(tc.value)
			s, ok := out.(string)
			require.True(t, ok, "Serialize(%s) must return a string, got %T", tc.name, out)
			require.NotEmpty(t, s, "Serialize(%s) must return a non-empty cursor", tc.name)
		})
	}
}

// TestCursorScalarSerializeStringPassthrough confirms an already-encoded cursor
// string serializes unchanged, and a nil/unknown value serializes to null.
func TestCursorScalarSerializeStringPassthrough(t *testing.T) {
	require.Equal(t, "abc", gqlgo.CursorScalar.Serialize("abc"))
	require.Nil(t, gqlgo.CursorScalar.Serialize(nil))
	require.Nil(t, gqlgo.CursorScalar.Serialize(42))
}

// TestCursorScalarParseValuePassthrough confirms ParseValue returns the raw
// encoded string (not a fixed-type Cursor[int]), so the generated
// ParsePaginationArgs can decode it into the node's actual ID type. A UUID
// cursor round-trips: serialize to a string, then ParseValue yields that same
// string, and decoding it as Cursor[uuid.UUID] recovers the ID.
func TestCursorScalarParseValuePassthrough(t *testing.T) {
	id := uuid.New()
	encoded := entgqlgo.Cursor[uuid.UUID]{ID: id}.String()

	parsed := gqlgo.CursorScalar.ParseValue(encoded)
	s, ok := parsed.(string)
	require.True(t, ok, "ParseValue must return the raw string, got %T", parsed)
	require.Equal(t, encoded, s)

	var c entgqlgo.Cursor[uuid.UUID]
	require.NoError(t, c.UnmarshalText([]byte(s)))
	require.Equal(t, id, c.ID)

	require.Nil(t, gqlgo.CursorScalar.ParseValue(123))
}
