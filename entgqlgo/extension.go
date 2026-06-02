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
		genWhereInput bool
		relaySpec     bool
		scalarFunc    func(*gen.Field, gen.Op) string
	}

	// ExtensionOption allows for managing the Extension configuration
	// using functional options.
	ExtensionOption func(*Extension) error
)

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

// WithSplitRuntime configures the generated code for consumers whose ent codegen
// uses the MatthewsREIS/ent fork's split runtime layout (per-entity subpackages +
// entbuilder generic mutations). In this mode, generated mutation-input code uses
// ent's generic mutation API (SetField/SetEdgeID/...) instead of typed setters.
//
// The flag is consulted by template FuncMap helpers (which are package-level
// functions and cannot reach the Extension instance), so it is stored solely in
// a package-level variable. Codegen is single-pass and single-threaded, so a
// package global is safe here and mirrors how ent itself threads global codegen
// config into template functions.
func WithSplitRuntime(enabled bool) ExtensionOption {
	return func(e *Extension) error {
		splitRuntime = enabled
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
//		entgqlgo.WithWhereInputs(true),
//		entgqlgo.WithRelaySpec(true),
//	)
func NewExtension(opts ...ExtensionOption) (*Extension, error) {
	ex := &Extension{
		templates:     AllTemplates,
		relaySpec:     true,
		genWhereInput: true,
	}
	for _, opt := range opts {
		if err := opt(ex); err != nil {
			return nil, err
		}
	}
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

// hasTemplate reports if the template exists in the template list and returns its index.
func (e *Extension) hasTemplate(tem *gen.Template) (int, bool) {
	tems := tem.Templates()
	if len(tems) < 2 {
		return -1, false
	}
	for i := range e.templates {
		if e.templates[i].Name() == tems[1].Name() {
			return i, true
		}
	}
	return -1, false
}

// splitRuntime mirrors the most recently configured WithSplitRuntime value so
// that package-level template FuncMap helpers (e.g. gqlgoSplitRuntime) can read
// it. See WithSplitRuntime for why a package global is used.
var splitRuntime bool

var (
	_ entc.Extension = (*Extension)(nil)

	camel    = gen.Funcs["camel"].(func(string) string)
	pascal   = gen.Funcs["pascal"].(func(string) string)
	plural   = gen.Funcs["plural"].(func(string) string)
	singular = gen.Funcs["singular"].(func(string) string)
	snake    = gen.Funcs["snake"].(func(string) string)
)
