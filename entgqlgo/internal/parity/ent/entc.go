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

//go:build ignore
// +build ignore

package main

import (
	"log"

	"entgo.io/contrib/entgql"
	"entgo.io/contrib/entgqlgo"
	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	// go run ./ent/entc.go is invoked from the package directory (internal/parity/),
	// so all paths below are relative to internal/parity/.

	// Run entgql first with no Go templates — SDL generation only.
	// WithTemplates() with no args disables Go template output; only the
	// schema hook (WithSchemaGenerator) runs, writing ent.graphql.
	// This avoids conflicts with entgqlgo's generated types (both would
	// otherwise produce CreateTodoInput etc. in the same package).
	gqlEx, err := entgql.NewExtension(
		entgql.WithSchemaGenerator(),
		entgql.WithSchemaPath("./ent.graphql"),
		entgql.WithWhereInputs(true),
		entgql.WithTemplates(), // SDL only — no gql_*.go files
	)
	if err != nil {
		log.Fatalf("creating entgql extension: %v", err)
	}
	if err = entc.Generate("./ent/schema", &gen.Config{}, entc.Extensions(gqlEx)); err != nil {
		log.Fatalf("running entgql codegen: %v", err)
	}

	// Run entgqlgo second to generate the full ent client + gqlgo/ subpackage.
	gqlgoEx, err := entgqlgo.NewExtension()
	if err != nil {
		log.Fatalf("creating entgqlgo extension: %v", err)
	}
	if err = entc.Generate("./ent/schema", &gen.Config{}, entc.Extensions(gqlgoEx)); err != nil {
		log.Fatalf("running entgqlgo codegen: %v", err)
	}
}
