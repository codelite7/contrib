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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

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

// buildIndexConfig walks g and produces the deterministic IndexConfig the
// writer renders. In this PR it emits only composite (col, id) order
// indexes — equality and GIN indexes arrive in later PRs.
//
// As a side effect, for any OrderField-annotated field whose Postgres
// SchemaType is "text" and which has no existing OrderFieldExpr, this
// function injects entgql.OrderFieldExpr(left("col", 256)) into the
// field's annotation map so the pagination templates (PR #18) emit
// ORDER BY expressions that match the generated indexes. A
// consumer-provided OrderFieldExpr is never overwritten.
func buildIndexConfig(g *gen.Graph, ex *Extension) (IndexConfig, error) {
	var cfg IndexConfig

	for _, node := range g.Nodes {
		entSkipped, err := isEntSQLSkipped(node.Annotations)
		if err != nil {
			return cfg, fmt.Errorf("entgql: decode entsql annotation on node %s: %w", node.Name, err)
		}
		if entSkipped {
			continue
		}

		tableName := node.Table()
		if ex.indexTableNameStrip != nil {
			tableName = ex.indexTableNameStrip.ReplaceAllString(tableName, "")
		}

		var where string
		if ex.indexSoftDeleteColumn != "" && nodeHasColumn(node, ex.indexSoftDeleteColumn) {
			where = fmt.Sprintf("%s IS NULL", ex.indexSoftDeleteColumn)
		}

		var indexes []Index
		for _, f := range node.Fields {
			if f.Name == "id" {
				continue
			}
			if ex.indexSoftDeleteColumn != "" && f.StorageKey() == ex.indexSoftDeleteColumn {
				continue
			}

			gqlAnt, err := annotation(f.Annotations)
			if err != nil {
				return cfg, fmt.Errorf("entgql: decode entgql annotation on %s.%s: %w", node.Name, f.Name, err)
			}
			if len(gqlAnt.OrderField) == 0 {
				continue
			}
			if gqlAnt.SkipIndex.Is(SkipIndexOrder) {
				continue
			}

			fieldSkipped, err := isEntSQLSkipped(f.Annotations)
			if err != nil {
				return cfg, fmt.Errorf("entgql: decode entsql annotation on %s.%s: %w", node.Name, f.Name, err)
			}
			if fieldSkipped {
				continue
			}

			col := f.StorageKey()
			idx := Index{
				Name:  fmt.Sprintf("idx_order_%s_%s_id", tableName, col),
				Field: col,
				Where: where,
			}

			if isUnboundedTextField(f) {
				idx.Expression = fmt.Sprintf(`left("%s", %d)`, col, defaultTextPrefixLen)
				// Auto-inject OrderFieldExpr so ORDER BY matches the index.
				// Never overwrite a consumer-provided expression.
				if gqlAnt.OrderFieldExpr == "" {
					gqlAnt.OrderFieldExpr = idx.Expression
					if f.Annotations == nil {
						f.Annotations = gen.Annotations{}
					}
					f.Annotations[gqlAnt.Name()] = gqlAnt
				}
			}

			indexes = append(indexes, idx)
		}

		if len(indexes) > 0 {
			cfg.Tables = append(cfg.Tables, IndexTable{
				Name:    tableName,
				Indexes: indexes,
			})
		}
	}

	// Deterministic sort: tables alphabetical; indexes within a table by Field.
	sort.Slice(cfg.Tables, func(i, j int) bool {
		return cfg.Tables[i].Name < cfg.Tables[j].Name
	})
	for i := range cfg.Tables {
		sort.Slice(cfg.Tables[i].Indexes, func(a, b int) bool {
			return cfg.Tables[i].Indexes[a].Field < cfg.Tables[i].Indexes[b].Field
		})
	}

	return cfg, nil
}

// nodeHasColumn reports whether any field on node has the given StorageKey.
func nodeHasColumn(node *gen.Type, col string) bool {
	for _, f := range node.Fields {
		if f.StorageKey() == col {
			return true
		}
	}
	return false
}

// emitIndexFile renders the index template with cfg and writes the result to
// the given path. Creates parent directories as needed. The template lives at
// template/indexes.tmpl and is embedded via the package-wide _templates FS in
// template.go. The write is atomic (temp-file + rename), matching the
// writeFileAtomic helper used for the GraphQL schema output.
func emitIndexFile(path string, cfg IndexConfig) error {
	tmpl, err := template.New("indexes.tmpl").ParseFS(_templates, "template/indexes.tmpl")
	if err != nil {
		return fmt.Errorf("entgql: parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return fmt.Errorf("entgql: execute template: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("entgql: create dir %s: %w", filepath.Dir(path), err)
	}

	return writeFileAtomic(path, buf.Bytes(), 0o644)
}

// emitIndexFileHook returns a gen.Hook that builds the IndexConfig
// (mutating the graph by injecting OrderFieldExpr where appropriate),
// writes the SQL file, then chains next.Generate(g).
//
// It is a pre-hook: own work first, then next.Generate. Must be appended
// LAST in Extension.Hooks() so that next.Generate inside it invokes ent's
// base template generator — that way the OrderFieldExpr injection is
// visible to the pagination templates.
func (e *Extension) emitIndexFileHook() gen.Hook {
	return func(next gen.Generator) gen.Generator {
		return gen.GenerateFunc(func(g *gen.Graph) error {
			cfg, err := buildIndexConfig(g, e)
			if err != nil {
				return fmt.Errorf("entgql: build config: %w", err)
			}
			if err := emitIndexFile(e.indexOutputPath, cfg); err != nil {
				return fmt.Errorf("entgql: emit file: %w", err)
			}
			return next.Generate(g)
		})
	}
}
