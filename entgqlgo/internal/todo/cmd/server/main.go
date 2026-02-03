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

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
	_ "github.com/mattn/go-sqlite3"

	"entgo.io/contrib/entgqlgo/internal/todo/ent"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/todo"
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

	// Build GraphQL schema using graphql-go
	schema, err := buildSchema(client)
	if err != nil {
		log.Fatalf("failed to build schema: %v", err)
	}

	// Create GraphQL handler
	h := handler.New(&handler.Config{
		Schema:   &schema,
		Pretty:   true,
		GraphiQL: true,
	})

	// Start server
	http.Handle("/graphql", h)
	log.Println("Server running at http://localhost:8080/graphql")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// buildSchema creates the GraphQL schema using graphql-go.
// This is a simplified example - the generated code would normally provide this.
func buildSchema(client *ent.Client) (graphql.Schema, error) {
	// Define Todo type
	todoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Todo",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
			},
			"text": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
			},
			"status": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
			},
			"priority": &graphql.Field{
				Type: graphql.Int,
			},
			"createdAt": &graphql.Field{
				Type: graphql.String,
			},
		},
	})

	// Define Category type
	categoryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Category",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
			},
			"text": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
			},
			"status": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
			},
		},
	})

	// Define Query type
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"todos": &graphql.Field{
				Type: graphql.NewNonNull(graphql.NewList(todoType)),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return client.Todo.Query().All(p.Context)
				},
			},
			"todo": &graphql.Field{
				Type: todoType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.Int),
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					id := p.Args["id"].(int)
					return client.Todo.Get(p.Context, id)
				},
			},
			"categories": &graphql.Field{
				Type: graphql.NewNonNull(graphql.NewList(categoryType)),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return client.Category.Query().All(p.Context)
				},
			},
		},
	})

	// Define Mutation type
	mutationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"createTodo": &graphql.Field{
				Type: todoType,
				Args: graphql.FieldConfigArgument{
					"text": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.String),
					},
					"status": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
					"priority": &graphql.ArgumentConfig{
						Type: graphql.Int,
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					create := client.Todo.Create().
						SetText(p.Args["text"].(string))

					if status, ok := p.Args["status"].(string); ok {
						create = create.SetStatus(todo.Status(status))
					} else {
						create = create.SetStatus(todo.StatusPending)
					}

					if priority, ok := p.Args["priority"].(int); ok {
						create = create.SetPriority(priority)
					}

					return create.Save(p.Context)
				},
			},
			"createCategory": &graphql.Field{
				Type: categoryType,
				Args: graphql.FieldConfigArgument{
					"text": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.String),
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return client.Category.Create().
						SetText(p.Args["text"].(string)).
						Save(p.Context)
				},
			},
		},
	})

	return graphql.NewSchema(graphql.SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
	})
}
