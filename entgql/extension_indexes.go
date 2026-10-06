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
	"fmt"
	"regexp"
)

// WithIndexOutput configures entgql to emit a deterministic SQL file containing
// CREATE INDEX statements that match the queries the generated GraphQL surface
// invites (ORDER BY, WhereInput predicates).
//
// path is the absolute or relative file path to write. Empty path (the default)
// disables emission and no writer hook is added to the extension.
//
// The generated file is regenerated on every entgql run and should be checked
// into version control alongside the schema. Sort order is deterministic
// (tables alphabetical, indexes alphabetical by field within table) so
// regenerations produce minimal diffs.
//
// Per-field opt-out via the entgql.SkipIndex annotation.
func WithIndexOutput(path string) ExtensionOption {
	return func(ex *Extension) error {
		ex.indexOutputPath = path
		return nil
	}
}

// WithIndexTableNameStrip configures a regular expression whose full-match
// substrings are deleted from node.Table() when computing the physical table
// name for index emission. Use this when ent entities are backed by views
// but indexes target the underlying base table.
//
//	entgql.WithIndexTableNameStrip("_view$")
//	// node.Table() = "escrows_view"   -> index target = "escrows"
//	// node.Table() = "sync_positions" -> index target = "sync_positions" (unchanged)
//
// Empty pattern (the default) disables stripping. An invalid regex returns
// an error at option-apply time, so misconfiguration fails fast during
// NewExtension rather than during codegen.
func WithIndexTableNameStrip(pattern string) ExtensionOption {
	return func(ex *Extension) error {
		if pattern == "" {
			ex.indexTableNameStrip = nil
			return nil
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("entgql: WithIndexTableNameStrip: invalid pattern %q: %w", pattern, err)
		}
		ex.indexTableNameStrip = re
		return nil
	}
}

// WithIndexSoftDeleteColumn names the column whose presence on an entity
// triggers partial-index emission ("WHERE <col> IS NULL"). Default:
// "deleted_at". Empty string disables partial-index emission entirely —
// there is no fallback to the default, so pass "deleted_at" explicitly
// to restore the default after disabling.
//
// Match is by StorageKey() on any field of the entity; the column does
// not need to be exposed in the GraphQL schema.
func WithIndexSoftDeleteColumn(col string) ExtensionOption {
	return func(ex *Extension) error {
		ex.indexSoftDeleteColumn = col
		return nil
	}
}
