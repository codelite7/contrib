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

package entgqlgo

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderDirection(t *testing.T) {
	tests := []struct {
		name      string
		direction OrderDirection
		valid     bool
		reversed  OrderDirection
	}{
		{"ASC", OrderDirectionAsc, true, OrderDirectionDesc},
		{"DESC", OrderDirectionDesc, true, OrderDirectionAsc},
		{"Invalid", OrderDirection("INVALID"), false, OrderDirection("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.direction.Validate()
			if tt.valid {
				assert.NoError(t, err)
				assert.Equal(t, tt.reversed, tt.direction.Reverse())
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestOrderDirectionString(t *testing.T) {
	assert.Equal(t, "ASC", OrderDirectionAsc.String())
	assert.Equal(t, "DESC", OrderDirectionDesc.String())
}

func TestNullsDirection(t *testing.T) {
	tests := []struct {
		name      string
		direction NullsDirection
		valid     bool
		reversed  NullsDirection
	}{
		{"FIRST", NullsFirst, true, NullsLast},
		{"LAST", NullsLast, true, NullsFirst},
		{"Invalid", NullsDirection("INVALID"), false, NullsDirection("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.direction.Validate()
			if tt.valid {
				assert.NoError(t, err)
				assert.Equal(t, tt.reversed, tt.direction.Reverse())
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestCursorMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name   string
		cursor Cursor[int]
	}{
		{
			name: "simple id",
			cursor: Cursor[int]{
				ID: 123,
			},
		},
		{
			name: "id with value",
			cursor: Cursor[int]{
				ID:    456,
				Value: "test-value",
			},
		},
		{
			name: "zero id",
			cursor: Cursor[int]{
				ID: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to string
			str := tt.cursor.String()
			require.NotEmpty(t, str)

			// Verify the string is base64 encoded
			_, err := base64.RawStdEncoding.DecodeString(str)
			require.NoError(t, err, "String should be valid base64")
		})
	}
}

func TestCursorStringCursor(t *testing.T) {
	cursor := Cursor[string]{
		ID:    "abc-123",
		Value: nil,
	}

	str := cursor.String()
	require.NotEmpty(t, str)

	// Verify the string is base64 encoded
	_, err := base64.RawStdEncoding.DecodeString(str)
	require.NoError(t, err, "String should be valid base64")
}

func TestPageInfo(t *testing.T) {
	cursor1 := &Cursor[int]{ID: 1}
	cursor2 := &Cursor[int]{ID: 10}

	pageInfo := PageInfo[int]{
		HasNextPage:     true,
		HasPreviousPage: false,
		StartCursor:     cursor1,
		EndCursor:       cursor2,
	}

	assert.True(t, pageInfo.HasNextPage)
	assert.False(t, pageInfo.HasPreviousPage)
	assert.Equal(t, 1, pageInfo.StartCursor.ID)
	assert.Equal(t, 10, pageInfo.EndCursor.ID)
}
