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

package entgql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewExtension_DefaultIndexOptions(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension()
	require.NoError(t, err)
	require.Empty(t, ex.indexOutputPath)
	require.Nil(t, ex.indexTableNameStrip)
	require.Equal(t, "deleted_at", ex.indexSoftDeleteColumn)
}

func TestWithIndexOutput_SetsPath(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension(WithIndexOutput("out/indexes.sql"))
	require.NoError(t, err)
	require.Equal(t, "out/indexes.sql", ex.indexOutputPath)
}

func TestWithIndexOutput_EmptyDisables(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension(WithIndexOutput(""))
	require.NoError(t, err)
	require.Empty(t, ex.indexOutputPath)
}

func TestWithIndexTableNameStrip_ValidPattern(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension(WithIndexTableNameStrip("_view$"))
	require.NoError(t, err)
	require.NotNil(t, ex.indexTableNameStrip)
	require.Equal(t, "escrows", ex.indexTableNameStrip.ReplaceAllString("escrows_view", ""))
	require.Equal(t, "escrows_archived", ex.indexTableNameStrip.ReplaceAllString("escrows_archived", ""))
}

func TestWithIndexTableNameStrip_EmptyResetsToNil(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension(
		WithIndexTableNameStrip("_view$"),
		WithIndexTableNameStrip(""),
	)
	require.NoError(t, err)
	require.Nil(t, ex.indexTableNameStrip)
}

func TestWithIndexTableNameStrip_InvalidPattern_Errors(t *testing.T) {
	t.Parallel()
	_, err := NewExtension(WithIndexTableNameStrip("[invalid"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "WithIndexTableNameStrip")
}

func TestWithIndexSoftDeleteColumn_Override(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension(WithIndexSoftDeleteColumn("removed_at"))
	require.NoError(t, err)
	require.Equal(t, "removed_at", ex.indexSoftDeleteColumn)
}

func TestWithIndexSoftDeleteColumn_EmptyDisables(t *testing.T) {
	t.Parallel()
	ex, err := NewExtension(WithIndexSoftDeleteColumn(""))
	require.NoError(t, err)
	require.Empty(t, ex.indexSoftDeleteColumn)
}

func TestIndexOptions_HooksUnchanged(t *testing.T) {
	t.Parallel()
	// Baseline: no index options.
	baseline, err := NewExtension()
	require.NoError(t, err)
	baselineHooks := len(baseline.Hooks())

	// All three index options set, but no writer hook is wired in this PR.
	// Hook count must NOT change — that arrives in the next PR.
	ex, err := NewExtension(
		WithIndexOutput("out/indexes.sql"),
		WithIndexTableNameStrip("_view$"),
		WithIndexSoftDeleteColumn("removed_at"),
	)
	require.NoError(t, err)
	require.Equal(t, baselineHooks, len(ex.Hooks()),
		"PR 1 must not wire any hook; that arrives in PR 2")
}
