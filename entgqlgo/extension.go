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
	"os"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

type (
	// Extension implements the entc.Extension for providing graphql-go/graphql integration.
	Extension struct {
		entc.DefaultExtension
		hooks     []gen.Hook
		templates []*gen.Template

		// Configuration
		genWhereInput    bool
		genMutations     bool
		relaySpec        bool
		genSchema        bool
		path             string
		scalarFunc       func(*gen.Field, gen.Op) string
	}

	// ExtensionOption allows for managing the Extension configuration
	// using functional options.
	ExtensionOption func(*Extension) error
)

// WithSchemaPath sets the filepath to write the generated Go GraphQL schema code.
func WithSchemaPath(path string) ExtensionOption {
	return func(ex *Extension) error {
		ex.path = path
		return nil
	}
}

// WithTemplates overrides the default templates with specific templates.
func WithTemplates(templates ...*gen.Template) ExtensionOption {
	return func(ex *Extension) error {
		ex.templates = templates
		return nil
	}
}

// WithWhereInputs configures the extension to either add or
// remove the WhereTemplate from the code generation templates.
func WithWhereInputs(b bool) ExtensionOption {
	return func(ex *Extension) error {
		ex.genWhereInput = b
		i, exists := ex.hasTemplate(WhereTemplate)
		if b && !exists {
			ex.templates = append(ex.templates, WhereTemplate)
		} else if !b && exists && len(ex.templates) > 0 {
			ex.templates = append(ex.templates[:i], ex.templates[i+1:]...)
		}
		return nil
	}
}

// WithRelaySpec enables or disables generating the Relay Node interface.
func WithRelaySpec(enabled bool) ExtensionOption {
	return func(e *Extension) error {
		e.relaySpec = enabled
		return nil
	}
}

// WithSchemaGenerator add a hook for generate GQL schema
func WithSchemaGenerator() ExtensionOption {
	return func(e *Extension) error {
		e.genSchema = true
		return nil
	}
}

// WithMapScalarFunc allows users to provide a custom function that
// maps an ent.Field (*gen.Field) into its GraphQL scalar type.
func WithMapScalarFunc(scalarFunc func(*gen.Field, gen.Op) string) ExtensionOption {
	return func(ex *Extension) error {
		ex.scalarFunc = scalarFunc
		return nil
	}
}

// NewExtension creates a new extension with the given configuration.
//
//	ex, err := entgqlgo.NewExtension(
//		entgqlgo.WithSchemaGenerator(),
//		entgqlgo.WithSchemaPath("./gql_schema.go"),
//		entgqlgo.WithWhereInputs(true),
//	)
func NewExtension(opts ...ExtensionOption) (*Extension, error) {
	ex := &Extension{
		templates:    AllTemplates,
		relaySpec:    true,
		genMutations: true,
	}
	for _, opt := range opts {
		if err := opt(ex); err != nil {
			return nil, err
		}
	}
	ex.hooks = append(ex.hooks, ex.genSchemaHook())
	return ex, nil
}

// Templates of the extension.
func (e *Extension) Templates() []*gen.Template {
	return e.templates
}

// Hooks of the extension.
func (e *Extension) Hooks() []gen.Hook {
	return e.hooks
}

// Options of the extension.
func (e *Extension) Options() []entc.Option {
	return []entc.Option{
		entc.FeatureNames(gen.FeatureNamedEdges.Name),
	}
}

// genSchemaHook returns a new hook for generating the GraphQL schema from the graph.
func (e *Extension) genSchemaHook() gen.Hook {
	return func(next gen.Generator) gen.Generator {
		return gen.GenerateFunc(func(g *gen.Graph) (err error) {
			if err = next.Generate(g); err != nil {
				return err
			}
			if !(e.genSchema || e.genWhereInput || e.genMutations) {
				return nil
			}
			if e.path == "" {
				return nil
			}
			// Generate the graphql-go schema code
			schemaCode, err := e.buildSchemaCode(g)
			if err != nil {
				return err
			}
			return os.WriteFile(e.path, []byte(schemaCode), 0644)
		})
	}
}

// buildSchemaCode generates the graphql-go/graphql schema code.
func (e *Extension) buildSchemaCode(g *gen.Graph) (string, error) {
	// This is a placeholder - actual implementation will generate
	// graphql-go type definitions and schema builder code
	return "// Generated graphql-go schema\npackage gql\n", nil
}

// hasTemplate reports if the template exists in the template list and returns its index.
func (e *Extension) hasTemplate(tem *gen.Template) (int, bool) {
	for i := range e.templates {
		if e.templates[i].Name() == tem.Templates()[1].Name() {
			return i, true
		}
	}
	return -1, false
}

var (
	_ entc.Extension = (*Extension)(nil)

	camel    = gen.Funcs["camel"].(func(string) string)
	pascal   = gen.Funcs["pascal"].(func(string) string)
	plural   = gen.Funcs["plural"].(func(string) string)
	singular = gen.Funcs["singular"].(func(string) string)
	snake    = gen.Funcs["snake"].(func(string) string)
)
