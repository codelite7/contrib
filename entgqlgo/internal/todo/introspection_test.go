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

package todo

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"entgo.io/contrib/entgqlgo/internal/todo/ent/enttest"
	"entgo.io/contrib/entgqlgo/internal/todo/ent/gqlgo"

	"github.com/graphql-go/handler"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// introspectionQuery is the standard GraphQL introspection query that retrieves
// the full schema information.
const introspectionQuery = `
query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
}

fragment InputValue on __InputValue {
  name
  description
  type {
    ...TypeRef
  }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}
`

const goldenFile = "testdata/schema_introspection.json"

// TestSchemaIntrospection tests that the generated GraphQL schema matches
// the expected introspection result stored in a golden file.
func TestSchemaIntrospection(t *testing.T) {
	// Create ent client with in-memory SQLite
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	// Build GraphQL schema
	schema, err := gqlgo.NewSchema(client)
	require.NoError(t, err, "failed to create GraphQL schema")

	// Create HTTP test server
	h := handler.New(&handler.Config{
		Schema:   &schema,
		Pretty:   true,
		GraphiQL: false,
	})
	server := httptest.NewServer(h)
	defer server.Close()

	// Execute introspection query
	reqBody, err := json.Marshal(map[string]string{
		"query": introspectionQuery,
	})
	require.NoError(t, err)

	resp, err := http.Post(server.URL, "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Parse and normalize the response
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Check for GraphQL errors
	if errors, ok := result["errors"]; ok {
		t.Fatalf("GraphQL introspection returned errors: %v", errors)
	}

	// Normalize the response for deterministic comparison
	normalizeIntrospectionResult(result)

	// Format with indentation for readability
	normalizedJSON, err := json.MarshalIndent(result, "", "  ")
	require.NoError(t, err)

	// Load golden file or create it if it doesn't exist
	goldenPath := filepath.Join(".", goldenFile)

	goldenData, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		// Create testdata directory if needed
		err = os.MkdirAll(filepath.Dir(goldenPath), 0755)
		require.NoError(t, err)

		// Write the golden file
		err = os.WriteFile(goldenPath, normalizedJSON, 0644)
		require.NoError(t, err)
		t.Logf("Created golden file: %s", goldenPath)
		return
	}
	require.NoError(t, err)

	// Compare with golden file
	if !bytes.Equal(normalizedJSON, goldenData) {
		// Write actual output for debugging
		actualPath := filepath.Join(".", "testdata/schema_introspection_actual.json")
		err = os.WriteFile(actualPath, normalizedJSON, 0644)
		require.NoError(t, err)
		t.Errorf("Schema introspection mismatch.\nActual output written to: %s\nExpected: %s\nRun diff to see changes: diff %s %s",
			actualPath, goldenPath, goldenPath, actualPath)
	}
}

// normalizeIntrospectionResult sorts arrays in the introspection result
// for deterministic comparison.
func normalizeIntrospectionResult(result map[string]interface{}) {
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return
	}
	schema, ok := data["__schema"].(map[string]interface{})
	if !ok {
		return
	}

	// Sort types by name
	if types, ok := schema["types"].([]interface{}); ok {
		sortByName(types)
		for _, t := range types {
			if typeMap, ok := t.(map[string]interface{}); ok {
				normalizeType(typeMap)
			}
		}
	}

	// Sort directives by name
	if directives, ok := schema["directives"].([]interface{}); ok {
		sortByName(directives)
		for _, d := range directives {
			if directiveMap, ok := d.(map[string]interface{}); ok {
				// Sort directive args
				if args, ok := directiveMap["args"].([]interface{}); ok {
					sortByName(args)
				}
				// Sort locations
				if locations, ok := directiveMap["locations"].([]interface{}); ok {
					sortStrings(locations)
				}
			}
		}
	}
}

// normalizeType sorts all arrays within a type definition.
func normalizeType(typeMap map[string]interface{}) {
	// Sort fields by name
	if fields, ok := typeMap["fields"].([]interface{}); ok {
		sortByName(fields)
		for _, f := range fields {
			if fieldMap, ok := f.(map[string]interface{}); ok {
				// Sort field args
				if args, ok := fieldMap["args"].([]interface{}); ok {
					sortByName(args)
				}
			}
		}
	}

	// Sort inputFields by name
	if inputFields, ok := typeMap["inputFields"].([]interface{}); ok {
		sortByName(inputFields)
	}

	// Sort enumValues by name
	if enumValues, ok := typeMap["enumValues"].([]interface{}); ok {
		sortByName(enumValues)
	}

	// Sort interfaces by name
	if interfaces, ok := typeMap["interfaces"].([]interface{}); ok {
		sortByName(interfaces)
	}

	// Sort possibleTypes by name
	if possibleTypes, ok := typeMap["possibleTypes"].([]interface{}); ok {
		sortByName(possibleTypes)
	}
}

// sortByName sorts a slice of maps by their "name" field.
func sortByName(items []interface{}) {
	slices.SortFunc(items, func(a, b interface{}) int {
		aMap, aOk := a.(map[string]interface{})
		bMap, bOk := b.(map[string]interface{})
		if !aOk || !bOk {
			return 0
		}
		aName, _ := aMap["name"].(string)
		bName, _ := bMap["name"].(string)
		if aName < bName {
			return -1
		}
		if aName > bName {
			return 1
		}
		return 0
	})
}

// sortStrings sorts a slice of interface{} containing strings.
func sortStrings(items []interface{}) {
	slices.SortFunc(items, func(a, b interface{}) int {
		aStr, _ := a.(string)
		bStr, _ := b.(string)
		if aStr < bStr {
			return -1
		}
		if aStr > bStr {
			return 1
		}
		return 0
	})
}


// TestNoSingularByIDQueries verifies that the schema does not contain singular
// by-ID query fields (e.g., category(id: ID!), todo(id: ID!)). Single-entity
// lookups should use node(id: ID!) instead, matching gqlgen behavior.
func TestNoSingularByIDQueries(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	schema, err := gqlgo.NewSchema(client)
	require.NoError(t, err, "failed to create GraphQL schema")

	queryType := schema.QueryType()
	require.NotNil(t, queryType, "schema should have a Query type")

	fields := queryType.Fields()

	singularNames := []string{"category", "todo"}
	for _, name := range singularNames {
		_, exists := fields[name]
		require.False(t, exists, "singular by-ID query %%q should not exist; use node(id: ID!) instead", name)
	}

	connectionNames := []string{"categoriesConnection", "todosConnection"}
	for _, name := range connectionNames {
		_, exists := fields[name]
		require.False(t, exists, "Connection query %%q should not exist", name)
	}

	pluralNames := []string{"categories", "todos"}
	for _, name := range pluralNames {
		_, exists := fields[name]
		require.True(t, exists, "plural list query %%q should exist", name)
	}

	relayNames := []string{"node", "nodes"}
	for _, name := range relayNames {
		_, exists := fields[name]
		require.True(t, exists, "relay query %%q should exist", name)
	}
}
