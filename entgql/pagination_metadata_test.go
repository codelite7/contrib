package entgql

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCursorMetadataRoundTrip(t *testing.T) {
	type Meta struct {
		Rank  int64  `msgpack:"rank"`
		Index string `msgpack:"index"`
	}

	original := Cursor[int]{ID: 42}
	meta := Meta{Rank: 101, Index: "companies"}
	require.NoError(t, original.SetMetadata(meta))
	require.True(t, original.HasMetadata())

	var buf bytes.Buffer
	original.MarshalGQL(&buf)

	var decoded Cursor[int]
	require.NoError(t, decoded.UnmarshalGQL(buf.String()))
	require.True(t, decoded.HasMetadata())

	var out Meta
	require.NoError(t, decoded.Metadata(&out))
	require.Equal(t, meta, out)

	decoded.ClearMetadata()
	require.False(t, decoded.HasMetadata())
	require.ErrorIs(t, decoded.Metadata(&out), ErrNoCursorMetadata)
}
func TestCursorMetadataSetNil(t *testing.T) {
	c := Cursor[string]{ID: "abc"}
	require.NoError(t, c.SetMetadata(map[string]int{"x": 1}))
	require.True(t, c.HasMetadata())
	require.NoError(t, c.SetMetadata(nil))
	require.False(t, c.HasMetadata())
}

func TestCursorMetadataUnmarshalError(t *testing.T) {
	c := Cursor[int]{ID: 7}
	require.Error(t, c.Metadata(nil))

	// Set metadata that can't be marshaled.
	type invalid struct {
		Ch chan int
	}
	err := c.SetMetadata(invalid{Ch: make(chan int)})
	require.Error(t, err)
	require.False(t, c.HasMetadata())
}
