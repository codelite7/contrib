package gqlwhere

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

type decStatus string

func (s *decStatus) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return &gqlerror.Error{Message: "enums must be strings"}
	}
	*s = decStatus(str)
	if *s != "OK" && *s != "BAD" {
		return &gqlerror.Error{Message: str + " is not a valid decStatus"}
	}
	return nil
}
func (s decStatus) MarshalGQL(w io.Writer) { _, _ = io.WriteString(w, `"`+string(s)+`"`) }

type decInner struct {
	Name *string `json:"name,omitempty"`
}

func (i *decInner) UnmarshalGQLContext(ctx context.Context, v any) error {
	return Decode(ctx, "DecInner", i, v)
}

type decInput struct {
	unexported *string        // skipped
	Predicates []int          `json:"-"` // skipped
	Text       *string        `json:"text,omitempty"`
	Count      *int           `json:"count,omitempty"`
	Big        *int64         `json:"big,omitempty"`
	Ratio      *float64       `json:"ratio,omitempty"`
	Flag       bool           `json:"flag,omitempty"`
	At         *time.Time     `json:"at,omitempty"`
	ID         *uuid.UUID     `json:"id,omitempty"`
	IntID      *int           `json:"intId,omitempty" gqlscalar:"ID"`
	Meta       map[string]any `json:"meta,omitempty"`
	Status     *decStatus     `json:"status,omitempty"`
	Inner      *decInner      `json:"inner,omitempty"`
	Tagged     *string        `gql:"taggedName" json:"ignoredName,omitempty"`
}

func TestDecode_NilAndTypedPassthrough(t *testing.T) {
	var dst decInput
	require.NoError(t, Decode(context.Background(), "DecInput", &dst, nil))
	require.Equal(t, decInput{}, dst)

	s := "x"
	src := decInput{Text: &s}
	var dst2 decInput
	require.NoError(t, Decode(context.Background(), "DecInput", &dst2, src))
	require.Equal(t, src, dst2)
	var dst3 decInput
	require.NoError(t, Decode(context.Background(), "DecInput", &dst3, &src))
	require.Equal(t, src, dst3)
}

func TestDecode_NotAMap(t *testing.T) {
	var dst decInput
	err := Decode(context.Background(), "DecInput", &dst, 42)
	require.EqualError(t, err, "unmarshalInputDecInput: expected map[string]any, got int")
}

func TestDecode_Scalars(t *testing.T) {
	id := uuid.MustParse("0d2b1a0e-6d7e-4b6f-9c1a-2f3e4d5c6b7a")
	in := map[string]any{
		"text": "hi", "count": 3, "big": int64(7), "ratio": 1.5, "flag": true,
		"at": "2024-01-02T03:04:05Z", "id": id.String(), "intId": "12",
		"meta": map[string]any{"k": "v"}, "status": "OK", "taggedName": "tagged",
		"inner":       map[string]any{"name": "n"},
		"ignoredName": "must-not-bind",
	}
	var dst decInput
	require.NoError(t, Decode(context.Background(), "DecInput", &dst, in))
	require.Equal(t, "hi", *dst.Text)
	require.Equal(t, 3, *dst.Count)
	require.Equal(t, int64(7), *dst.Big)
	require.Equal(t, 1.5, *dst.Ratio)
	require.True(t, dst.Flag)
	require.Equal(t, time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC), dst.At.UTC())
	require.Equal(t, id, *dst.ID)
	require.Equal(t, 12, *dst.IntID)
	require.Equal(t, map[string]any{"k": "v"}, dst.Meta)
	require.Equal(t, decStatus("OK"), *dst.Status)
	require.Equal(t, "n", *dst.Inner.Name)
	require.Equal(t, "tagged", *dst.Tagged)
	require.Nil(t, dst.unexported)
	require.Nil(t, dst.Predicates)
}

func TestDecode_NullLeavesNilPointer(t *testing.T) {
	var dst decInput
	require.NoError(t, Decode(context.Background(), "DecInput", &dst, map[string]any{"text": nil, "inner": nil}))
	require.Nil(t, dst.Text)
	require.Nil(t, dst.Inner)
}

func TestDecode_NullLeavesNilMap(t *testing.T) {
	var dst decInput
	require.NoError(t, Decode(context.Background(), "DecInput", &dst, map[string]any{"meta": nil}))
	require.Nil(t, dst.Meta)
}

func TestDecode_ErrorCarriesFieldPath(t *testing.T) {
	var dst decInput
	err := Decode(context.Background(), "DecInput", &dst, map[string]any{"count": "not-a-number"})
	var gqlErr *gqlerror.Error
	require.ErrorAs(t, err, &gqlErr)
	require.Equal(t, "count", gqlErr.Path.String())

	err = Decode(context.Background(), "DecInput", &dst, map[string]any{"inner": map[string]any{"name": map[string]any{}}})
	require.ErrorAs(t, err, &gqlErr)
	require.Equal(t, "inner.name", gqlErr.Path.String())

	err = Decode(context.Background(), "DecInput", &dst, map[string]any{"status": "NOPE"})
	require.ErrorAs(t, err, &gqlErr)
	require.Equal(t, "NOPE is not a valid decStatus", gqlErr.Message)
	require.Equal(t, "status", gqlErr.Path.String())
}

func TestDecode_FirstErrorInStructOrderWins(t *testing.T) {
	var dst decInput
	err := Decode(context.Background(), "DecInput", &dst, map[string]any{"status": "NOPE", "count": "bad"})
	var gqlErr *gqlerror.Error
	require.ErrorAs(t, err, &gqlErr)
	require.Equal(t, "count", gqlErr.Path.String()) // Count is declared before Status
}

func TestDecode_PathNestsUnderOuterContext(t *testing.T) {
	ctx := graphql.WithPathContext(context.Background(), graphql.NewPathWithField("where"))
	var dst decInput
	err := Decode(ctx, "DecInput", &dst, map[string]any{"count": "bad"})
	var gqlErr *gqlerror.Error
	require.ErrorAs(t, err, &gqlErr)
	require.Equal(t, "where.count", gqlErr.Path.String())
}

func TestDecode_UnsupportedTypeIsAPlanError(t *testing.T) {
	type bad struct {
		Ch chan int `json:"ch"`
	}
	var dst bad
	err := Decode(context.Background(), "Bad", &dst, map[string]any{})
	require.ErrorContains(t, err, `gqlwhere: no coercer for Go type chan int (field "ch")`)
}

type decStrings []string // like pq.StringArray

type decListInput struct {
	Names    []string        `json:"names,omitempty"`
	Nums     []int           `json:"nums,omitempty"`
	IDs      []int           `json:"ids,omitempty" gqlscalar:"ID"`
	Statuses []decStatus     `json:"statuses,omitempty"`
	Inners   []*decInner     `json:"inners,omitempty"`
	Named    *decStrings     `json:"named,omitempty"`
	Durs     []time.Duration `json:"durs,omitempty"`
}

func TestDecode_Lists(t *testing.T) {
	in := map[string]any{
		"names":    []any{"a", "b"},
		"nums":     "7", // single value coerced to a one-element list
		"ids":      []any{"1", 2},
		"statuses": []any{"OK", "BAD"},
		"inners":   []any{map[string]any{"name": "x"}, nil},
		"named":    []any{"p", "q"},
	}
	var dst decListInput
	require.NoError(t, Decode(context.Background(), "DecListInput", &dst, in))
	require.Equal(t, []string{"a", "b"}, dst.Names)
	require.Equal(t, []int{7}, dst.Nums)
	require.Equal(t, []int{1, 2}, dst.IDs)
	require.Equal(t, []decStatus{"OK", "BAD"}, dst.Statuses)
	require.Len(t, dst.Inners, 2)
	require.Equal(t, "x", *dst.Inners[0].Name)
	require.Nil(t, dst.Inners[1])
	require.Equal(t, decStrings{"p", "q"}, *dst.Named)
}

func TestDecode_ListNullIsNilSlice(t *testing.T) {
	var dst decListInput
	require.NoError(t, Decode(context.Background(), "DecListInput", &dst, map[string]any{"names": nil}))
	require.Nil(t, dst.Names)
}

func TestDecode_ListErrorCarriesIndexPath(t *testing.T) {
	var dst decListInput
	err := Decode(context.Background(), "DecListInput", &dst, map[string]any{"nums": []any{1, "x"}})
	var gqlErr *gqlerror.Error
	require.ErrorAs(t, err, &gqlErr)
	require.Equal(t, "nums[1]", gqlErr.Path.String())

	err = Decode(context.Background(), "DecListInput", &dst, map[string]any{"inners": []any{map[string]any{"name": map[string]any{}}}})
	require.ErrorAs(t, err, &gqlErr)
	require.Equal(t, "inners[0].name", gqlErr.Path.String())
}

func TestDecode_RegisteredCoercer(t *testing.T) {
	RegisterCoercer[time.Duration](func(_ context.Context, v any) (time.Duration, error) {
		s, ok := v.(string)
		if !ok {
			return 0, fmt.Errorf("%T is not a duration string", v)
		}
		return time.ParseDuration(s)
	})
	plans.Delete(reflect.TypeFor[decListInput]()) // plan was cached before registration
	var dst decListInput
	require.NoError(t, Decode(context.Background(), "DecListInput", &dst, map[string]any{"durs": []any{"1s", "2m"}}))
	require.Equal(t, []time.Duration{time.Second, 2 * time.Minute}, dst.Durs)
}
