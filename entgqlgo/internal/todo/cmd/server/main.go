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

package main

import (
	"context"
	"log"
	"net/http"

	"github.com/graphql-go/handler"
	_ "github.com/mattn/go-sqlite3"

	"entgo.io/contrib/entgqlgo/internal/todo/ent"
)

func main() {
	// Create ent client
	client, err := ent.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	if err != nil {
		log.Fatalf("failed to open ent client: %v", err)
	}
	defer client.Close()

	// Run migrations
	if err := client.Schema.Create(context.Background()); err != nil {
		log.Fatalf("failed to create schema: %v", err)
	}

	// Build GraphQL schema using generated code from entgqlgo
	schema, err := ent.NewSchema(client)
	if err != nil {
		log.Fatalf("failed to build schema: %v", err)
	}

	// Create GraphQL handler with GraphiQL playground
	h := handler.New(&handler.Config{
		Schema:   &schema,
		Pretty:   true,
		GraphiQL: true,
	})

	// Start server
	http.Handle("/graphql", h)
	log.Println("Server running at http://localhost:8080/graphql")
	log.Println("GraphiQL playground available at http://localhost:8080/graphql")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
