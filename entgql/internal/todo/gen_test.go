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

package todo_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"entgo.io/contrib/entgql"
	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestGeneratedSchema(t *testing.T) {
	tempDir := t.TempDir()
	gqlcfg, err := os.ReadFile("./gqlgen.yml")
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "gqlgen.yml"), gqlcfg, 0644)
	require.NoError(t, err)
	ex, err := entgql.NewExtension(
		entgql.WithConfigPath(filepath.Join(tempDir, "gqlgen.yml")),
		entgql.WithSchemaGenerator(),
		entgql.WithSchemaPath(filepath.Join(tempDir, "ent.graphql")),
		entgql.WithWhereInputs(true),
		entgql.WithNodeDescriptor(true),
	)
	require.NoError(t, err)
	err = entc.Generate("./ent/schema", &gen.Config{
		Target: tempDir,
		Features: []gen.Feature{
			gen.FeatureModifier,
		},
	}, entc.Extensions(ex))
	require.NoError(t, err)
	expected, err := os.ReadFile("./ent.graphql")
	require.NoError(t, err)
	actual, err := os.ReadFile(filepath.Join(tempDir, "ent.graphql"))
	require.NoError(t, err)
	require.Equal(t, string(expected), string(actual))
}

// TestGeneratedInputDecoderHooks asserts, without ever compiling the
// generated output, that where_input.tmpl/where_input_subpkg.tmpl and
// mutation_input.tmpl/mutation_input_sibling.tmpl emit the gqlwhere.Decode
// wiring (UnmarshalGQLContext + gql/gqlscalar struct tags) that Task 5 adds,
// in both the single-package and split-go-files ("subpkg") layouts.
func TestGeneratedInputDecoderHooks(t *testing.T) {
	tests := []struct {
		name string
		opts []entgql.ExtensionOption
	}{
		{name: "single-package"},
		{name: "split-files", opts: []entgql.ExtensionOption{entgql.WithSplitGoFiles(true)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			gqlcfg, err := os.ReadFile("./gqlgen.yml")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(tempDir, "gqlgen.yml"), gqlcfg, 0644))

			opts := append([]entgql.ExtensionOption{
				entgql.WithConfigPath(filepath.Join(tempDir, "gqlgen.yml")),
				entgql.WithSchemaGenerator(),
				entgql.WithSchemaPath(filepath.Join(tempDir, "ent.graphql")),
				entgql.WithWhereInputs(true),
				entgql.WithNodeDescriptor(true),
			}, tt.opts...)
			ex, err := entgql.NewExtension(opts...)
			require.NoError(t, err)
			require.NoError(t, entc.Generate("./ent/schema", &gen.Config{
				Target: tempDir,
				Features: []gen.Feature{
					gen.FeatureModifier,
				},
			}, entc.Extensions(ex)))

			// gqlgen.yml (see its "schema:" list) merges the hand-written
			// todo.graphql (custom scalars/inputs like CategoryConfig) with
			// the generated ent.graphql; load both the same way so
			// cross-file type references resolve.
			todoBytes, err := os.ReadFile("./todo.graphql")
			require.NoError(t, err)
			entBytes, err := os.ReadFile(filepath.Join(tempDir, "ent.graphql"))
			require.NoError(t, err)
			schema, err := gqlparser.LoadSchema(
				&ast.Source{Name: "todo.graphql", Input: string(todoBytes)},
				&ast.Source{Name: "ent.graphql", Input: string(entBytes)},
			)
			require.NoError(t, err)

			files := map[string]string{}
			require.NoError(t, filepath.WalkDir(tempDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() || !strings.HasSuffix(path, ".go") {
					return nil
				}
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				files[path] = string(b)
				return nil
			}))

			for _, def := range schema.Types {
				switch {
				case def.Kind == ast.InputObject && strings.HasSuffix(def.Name, "WhereInput"):
					checkWhereInput(t, files, def)
				case def.Kind == ast.InputObject && (strings.HasPrefix(def.Name, "Create") || strings.HasPrefix(def.Name, "Update")) && strings.HasSuffix(def.Name, "Input"):
					checkMutationInput(t, files, def)
				}
			}
		})
	}
}

// findStruct returns the file and body (between "struct {" and the matching
// top-level closing brace) of "type <name> struct" across all generated
// files, or ok=false if no file declares it.
func findStruct(files map[string]string, name string) (file, body string, ok bool) {
	re := regexp.MustCompile(`(?s)type ` + regexp.QuoteMeta(name) + ` struct \{(.*?)\n\}`)
	for path, content := range files {
		if m := re.FindStringSubmatch(content); m != nil {
			return path, m[1], true
		}
	}
	return "", "", false
}

// hasUnmarshalGQLContext reports whether some file declares the
// UnmarshalGQLContext method for name backed by gqlwhere.Decode, and returns
// that file's path for failure messages.
func hasUnmarshalGQLContext(files map[string]string, name string) (file string, ok bool) {
	sig := fmt.Sprintf("func (i *%s) UnmarshalGQLContext(ctx context.Context, v any) error", name)
	decode := fmt.Sprintf(`return gqlwhere.Decode(ctx, "%s", i, v)`, name)
	for path, content := range files {
		if strings.Contains(content, sig) && strings.Contains(content, decode) {
			return path, true
		}
	}
	return "", false
}

func checkWhereInput(t *testing.T, files map[string]string, def *ast.Definition) {
	t.Helper()
	file, body, ok := findStruct(files, def.Name)
	if !ok {
		t.Errorf("%s: no generated file declares \"type %s struct\"", def.Name, def.Name)
		return
	}
	if _, ok := hasUnmarshalGQLContext(files, def.Name); !ok {
		t.Errorf("%s: no generated file has UnmarshalGQLContext + gqlwhere.Decode (struct in %s)", def.Name, file)
	}

	hasID := false
	for _, f := range def.Fields {
		if f.Name == "id" {
			hasID = true
			break
		}
	}
	if !hasID {
		return
	}
	idTagRe := regexp.MustCompile(`(?m)^\s*ID\s+\S+\s+` + "`json:\"id,omitempty\" gqlscalar:\"ID\"`")
	if !idTagRe.MatchString(body) {
		t.Errorf("%s: field \"id\": expected an `ID ... json:\"id,omitempty\" gqlscalar:\"ID\"` struct field in %s, struct body:\n%s", def.Name, file, body)
	}
}

func checkMutationInput(t *testing.T, files map[string]string, def *ast.Definition) {
	t.Helper()
	file, body, ok := findStruct(files, def.Name)
	if !ok {
		// Not every Create/Update*Input in the schema is necessarily an
		// entgql-generated mutation input struct; skip ones with no struct.
		return
	}
	if _, ok := hasUnmarshalGQLContext(files, def.Name); !ok {
		t.Errorf("%s: no generated file has UnmarshalGQLContext + gqlwhere.Decode (struct in %s)", def.Name, file)
	}

	for _, f := range def.Fields {
		// todo.graphql hand-extends some entgql-generated inputs with extra
		// fields resolved outside the generated struct (e.g. "extend input
		// CreateCategoryInput { createTodos: ... }"); only fields entgql
		// itself put in ent.graphql are backed by a generated struct field.
		if f.Position == nil || f.Position.Src == nil || f.Position.Src.Name != "ent.graphql" {
			continue
		}
		tag := `gql:"` + f.Name + `"`
		idx := strings.Index(body, tag)
		if idx == -1 {
			t.Errorf("%s: field %q: missing %s tag in %s", def.Name, f.Name, tag, file)
			continue
		}
		wantIDScalar := f.Type.Name() == "ID"
		if !wantIDScalar {
			continue
		}
		lineStart := strings.LastIndex(body[:idx], "\n") + 1
		lineEnd := strings.Index(body[idx:], "\n")
		if lineEnd == -1 {
			lineEnd = len(body)
		} else {
			lineEnd += idx
		}
		line := body[lineStart:lineEnd]
		if !strings.Contains(line, `gqlscalar:"ID"`) {
			t.Errorf("%s: field %q: expected gqlscalar:\"ID\" alongside %s in %s, got line: %s", def.Name, f.Name, tag, file, line)
		}
	}
}
