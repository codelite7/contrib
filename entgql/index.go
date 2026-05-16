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
	"encoding/json"
	"fmt"
	"strings"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/entc/gen"
)

// defaultTextPrefixLen is the character count used in left() for unbounded
// text columns. 256 chars (up to 1024 bytes in UTF-8) keeps index tuples well
// under the btree maximum of 2704 bytes while preserving meaningful sort order.
const defaultTextPrefixLen = 256

// IndexConfig is the data passed to the index SQL template.
type IndexConfig struct {
	Tables []IndexTable
}

// IndexTable groups a physical table's indexes for template rendering.
type IndexTable struct {
	Name    string
	Indexes []Index
}

// Index represents a single CREATE INDEX entry in the generated SQL file.
type Index struct {
	// Name is the generated index name, e.g. idx_order_escrows_created_at_id.
	Name string
	// Field is the physical column name.
	Field string
	// Expression, if non-empty, is rendered in place of the quoted Field —
	// used for expression indexes like left("col", 256).
	Expression string
	// Where, if non-empty, is rendered as a partial-index predicate,
	// e.g. "deleted_at IS NULL".
	Where string
}

// isUnboundedTextField reports whether f is a string field with an explicit
// Postgres "text" schema type, meaning it has no length bound and could exceed
// the btree maximum index row size.
func isUnboundedTextField(f *gen.Field) bool {
	if !f.IsString() {
		return false
	}
	schemaType := f.Column().SchemaType
	return strings.EqualFold(schemaType[dialect.Postgres], "text")
}

// isEntSQLSkipped reports whether ants contains entsql.Skip(). It returns an
// error if the annotation is present but cannot be decoded, so callers fail
// fast rather than silently treating an unreadable annotation as "not skipped".
func isEntSQLSkipped(ants gen.Annotations) (bool, error) {
	ant := &entsql.Annotation{}
	raw, ok := ants[ant.Name()]
	if !ok || raw == nil {
		return false, nil
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return false, fmt.Errorf("marshal entsql annotation: %w", err)
	}
	if err := json.Unmarshal(buf, ant); err != nil {
		return false, fmt.Errorf("unmarshal entsql annotation: %w", err)
	}
	return ant.Skip, nil
}
