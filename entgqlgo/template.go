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
	"embed"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"text/template"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
	"github.com/samber/lo"
	"slices"
)

var (
	// CollectionTemplate adds fields collection support using auto eager-load ent edges.
	CollectionTemplate = parseT("template/collection.tmpl")

	// EnumTemplate adds a template for enum types for graphql-go.
	EnumTemplate = parseT("template/enum.tmpl")

	// NodeTemplate implements the Relay Node interface for all types.
	NodeTemplate = parseT("template/node.tmpl")

	// NodeDescriptorTemplate implements the Node descriptor API for all types.
	NodeDescriptorTemplate = parseT("template/node_descriptor.tmpl")

	// PaginationTemplate adds pagination support according to the GraphQL Cursor Connections Spec.
	PaginationTemplate = parseT("template/pagination.tmpl")

	// EdgeTemplate adds edge resolution using eager-loading with a query fallback.
	EdgeTemplate = parseT("template/edge.tmpl")

	// WhereTemplate adds a template for generating <T>WhereInput filters for each schema type.
	WhereTemplate = parseT("template/where_input.tmpl")

	// MutationInputTemplate adds a template for generating Create<T>Input and Update<T>Input.
	MutationInputTemplate = parseT("template/mutation_input.tmpl").SkipIf(skipMutationTemplate)

	// TypesTemplate generates graphql.NewObject definitions for each ent type.
	TypesTemplate = parseT("template/types.tmpl")

	// SchemaTemplate generates graphql.NewSchema builder.
	SchemaTemplate = parseT("template/schema.tmpl")

	// ScalarsTemplate generates Cursor, enum scalars for graphql-go.
	ScalarsTemplate = parseT("template/scalars.tmpl")

	// OrderingTemplate generates order field enums and input types for graphql-go.
	OrderingTemplate = parseT("template/ordering.tmpl")

	// AllTemplates holds all templates for extending ent to support graphql-go/graphql.
	// Note: Unlike entgql, WhereTemplate is included by default.
	AllTemplates = []*gen.Template{
		CollectionTemplate,
		EnumTemplate,
		NodeTemplate,
		NodeDescriptorTemplate,
		PaginationTemplate,
		EdgeTemplate,
		WhereTemplate,
		MutationInputTemplate,
		TypesTemplate,
		SchemaTemplate,
		ScalarsTemplate,
		OrderingTemplate,
	}

	// TemplateFuncs contains the extra template functions used by entgqlgo.
	// All functions are prefixed with "gqlgo" to avoid conflicts with entgql's template functions.
	TemplateFuncs = template.FuncMap{
		"gqlgoQueryFieldName":        queryFieldName,
		"gqlgoQueryFieldDescription": queryFieldDescription,
		"gqlgoIsRelayConnNode":       isRelayConnNode,
		"gqlgoSplitRuntime":          gqlgoSplitRuntime,
		"gqlgoPascalMutations":       gqlgoPascalMutations,
		"gqlgoNeedsEntbuilder":       gqlgoNeedsEntbuilder,
		"gqlgoDeref":                 gqlgoDeref,
		"gqlgoDecodeField":           gqlgoDecodeField,
		"gqlgoMutationSetField":      mutationSetField,
		"gqlgoMutationClearField":    mutationClearField,
		"gqlgoMutationAppendField":   mutationAppendField,
		"gqlgoMutationSetEdgeID":     mutationSetEdgeID,
		"gqlgoMutationAddEdgeIDs":    mutationAddEdgeIDs,
		"gqlgoMutationRemoveEdgeIDs": mutationRemoveEdgeIDs,
		"gqlgoMutationClearEdge":     mutationClearEdge,
		"gqlgoEdgeQueryExpr":         edgeQueryExpr,
		"gqlgoEdgeWithExpr":          edgeWithExpr,
		"gqlgoFieldCollections":      fieldCollections,
		"gqlgoFieldMapping":          fieldMapping,
		"gqlgoFilterEdges":           filterEdges,
		"gqlgoFilterFields":          filterFields,
		"gqlgoFilterNodes":           filterNodes,
		"gqlgoIDType":                gqlIDType,
		"gqlgoHasWhereInput":         hasWhereInput,
		"gqlgoIsRelayConn":           isRelayConn,
		"gqlgoIsSkipMode":            isSkipMode,
		"gqlgoMutationInputs":        mutationInputs,
		"gqlgoNodeImplementors":      nodeImplementors,
		"gqlgoNodeImplementorsVar":   nodeImplementorsVar,
		"gqlgoNodePaginationNames":   nodePaginationNames,
		"gqlgoOrderFields":           orderFields,
		"gqlgoSkipMode":              skipModeFromString,
		"gqlgoTrimPrefix":            trimPrefix,
		"gqlgoType":                  gqlgoType,
		"gqlgoScalar":                gqlgoScalar,
		"gqlgoHasFieldNamed":         hasFieldNamed,
		"gqlgoImplements":            gqlgoImplements,
		"gqlgoIsDeprecatedEnumValue": isDeprecatedEnumValue,
	}

	//go:embed template/*
	_templates embed.FS
)

func parseT(path string) *gen.Template {
	return gen.MustParse(gen.NewTemplate(path).
		Funcs(TemplateFuncs).
		ParseFS(_templates, path))
}

// idType is returned by the gqlIDType below to describe the
// Go scalar type of the GraphQL ID.
type idType struct {
	*field.TypeInfo
	Mixed bool
}

// gqlIDType returns the scalar (Go) type of the GraphQL ID.
func gqlIDType(nodes []*gen.Type, defaultType *field.TypeInfo) (*idType, error) {
	if len(nodes) == 0 {
		return &idType{TypeInfo: defaultType}, nil
	}
	var mixed bool
	for i := 1; i < len(nodes); i++ {
		id1, id2 := nodes[i-1].ID, nodes[i].ID
		if mixed = id1.Type.Type != id2.Type.Type; mixed {
			break
		}
		if mixed = id1.HasGoType() != id2.HasGoType() || (id1.HasGoType() && id1.Type.RType.Ident != id2.Type.RType.Ident); mixed {
			break
		}
	}
	if !mixed {
		return &idType{TypeInfo: nodes[0].ID.Type}, nil
	}
	for _, n := range nodes {
		if n.ID.IsString() && !n.ID.HasGoType() {
			continue
		}
		if !n.ID.HasGoType() {
			return nil, errors.New("entgqlgo: mixed id types must be type string")
		}
	}
	return &idType{
		Mixed: true,
		TypeInfo: &field.TypeInfo{
			Type: field.TypeString,
		},
	}, nil
}

type fieldCollection struct {
	Edge    *gen.Edge
	Mapping []string
}

func fieldCollections(edges []*gen.Edge) ([]*fieldCollection, error) {
	collect := make([]*fieldCollection, 0, len(edges))
	for _, e := range edges {
		ant, err := annotation(e.Annotations)
		if err != nil {
			return nil, err
		}
		switch {
		case len(ant.Mapping) > 0:
			if !ant.Unbind {
				return nil, errors.New("bind and mapping annotations are mutually exclusive")
			}
			collect = append(collect, &fieldCollection{Edge: e, Mapping: ant.Mapping})
		case !ant.Unbind:
			mapping := []string{camel(e.Name)}
			collect = append(collect, &fieldCollection{Edge: e, Mapping: mapping})
		}
	}
	return collect, nil
}

// MutationDescriptor holds information about a GraphQL mutation input.
type MutationDescriptor struct {
	*gen.Type
	IsCreate bool
}

// Input returns the input's name.
func (m *MutationDescriptor) Input() (string, error) {
	gqlType, _, err := gqlTypeFromNode(m.Type)
	if err != nil {
		return "", err
	}
	if m.IsCreate {
		return fmt.Sprintf("Create%sInput", gqlType), nil
	}
	return fmt.Sprintf("Update%sInput", gqlType), nil
}

// Builders return the builder's names to apply the input.
func (m *MutationDescriptor) Builders() []string {
	if m.IsCreate {
		return []string{m.Type.CreateName()}
	}
	return []string{m.Type.UpdateName(), m.Type.UpdateOneName()}
}

// InputFieldDescriptor holds the information about a field in the input type.
type InputFieldDescriptor struct {
	*gen.Field
	AppendOp bool
	ClearOp  bool
	Nullable bool
}

// IsPointer returns true if the Go type should be a pointer
func (f *InputFieldDescriptor) IsPointer() bool {
	if f.Type.Nillable || f.Type.RType.IsPtr() {
		return false
	}
	return f.Nullable
}

// InputFields returns the list of fields in the input type.
func (m *MutationDescriptor) InputFields() ([]*InputFieldDescriptor, error) {
	fields := make([]*InputFieldDescriptor, 0, len(m.Type.Fields))
	for _, f := range m.Type.Fields {
		ant, err := annotation(f.Annotations)
		if err != nil {
			return nil, err
		}
		if f.IsEdgeField() || m.skip(f.Immutable, ant.Skip) {
			continue
		}
		fields = append(fields, &InputFieldDescriptor{
			Field:    f,
			AppendOp: !m.IsCreate && f.SupportsMutationAppend(),
			ClearOp:  !m.IsCreate && f.Optional,
			Nullable: !m.IsCreate || f.Optional || f.Default || f.DefaultFunc(),
		})
	}
	return fields, nil
}

// InputEdges returns the list of fields in the input type.
func (m *MutationDescriptor) InputEdges() ([]*gen.Edge, error) {
	edges := make([]*gen.Edge, 0, len(m.Type.Edges))
	for _, e := range m.Type.Edges {
		ant, err := annotation(e.Annotations)
		if err != nil {
			return nil, err
		}
		if e.Type.IsEdgeSchema() || m.skip(e.Immutable, ant.Skip) {
			continue
		}
		edges = append(edges, e)
	}
	return edges, nil
}

// gqlgoSplitRuntime reports whether the extension was configured for the
// MatthewsREIS/ent fork's split runtime layout via WithSplitRuntime(true).
// Templates branch on it to emit generic-mutation calls instead of typed setters.
//
// It reads the flag from the Graph's annotations, where NewExtension's
// annotation hook injects an ExtensionAnnotation before rendering. Templates
// call it once as {{ $split := gqlgoSplitRuntime $ }} and thread the result
// through the mutation/edge helpers as their leading bool argument.
//
// The annotation value may arrive either as the original ExtensionAnnotation
// struct (in-process injection) or, if it round-trips through ent's JSON
// annotation encoding, as a map[string]any with a "SplitRuntime" key. Both
// shapes are handled here so callers never see the difference.
func gqlgoSplitRuntime(g *gen.Graph) bool {
	if g == nil || g.Config == nil || g.Annotations == nil {
		return false
	}
	switch ant := g.Annotations[ExtensionAnnotation{}.Name()].(type) {
	case ExtensionAnnotation:
		return ant.SplitRuntime
	case map[string]any:
		v, _ := ant["SplitRuntime"].(bool)
		return v
	default:
		return false
	}
}

// gqlgoPascalMutations reports whether root Mutation field names should be
// emitted in PascalCase (CreateTodo) instead of the default camelCase
// (createTodo), as configured by WithPascalMutationNames(true).
//
// It reads the flag from the Graph's annotations, where NewExtension's
// annotation hook injects an ExtensionAnnotation before rendering. The schema
// template calls it once as {{ $pascalMut := gqlgoPascalMutations $ }} and
// branches the root Mutation field name strings on it.
//
// As with gqlgoSplitRuntime, the annotation value may arrive either as the
// original ExtensionAnnotation struct (in-process injection) or, if it
// round-trips through ent's JSON annotation encoding, as a map[string]any with a
// "PascalMutationNames" key. Both shapes are handled here.
func gqlgoPascalMutations(g *gen.Graph) bool {
	if g == nil || g.Config == nil || g.Annotations == nil {
		return false
	}
	switch ant := g.Annotations[ExtensionAnnotation{}.Name()].(type) {
	case ExtensionAnnotation:
		return ant.PascalMutationNames
	case map[string]any:
		v, _ := ant["PascalMutationNames"].(bool)
		return v
	default:
		return false
	}
}

// gqlgoDeref returns "*" when ptr is true, else "". Used by the mutation-input
// template to prefix a pointer dereference onto a value expression.
func gqlgoDeref(ptr bool) string {
	if ptr {
		return "*"
	}
	return ""
}

// gqlgoNeedsEntbuilder reports whether the mutation-input template needs to
// import entgo.io/ent/runtime/entbuilder, i.e. split-runtime mode is on AND at
// least one mutation input has a non-unique edge (the only place that emits
// entbuilder.ToAny). Avoids an unused-import compile error when every edge is
// unique. Mirrors gqlgoMutationInputs/InputEdges filtering so it stays in sync.
// The split flag is supplied by the template (via gqlgoSplitRuntime $).
func gqlgoNeedsEntbuilder(split bool, nodes []*gen.Type) (bool, error) {
	if !split {
		return false, nil
	}
	inputs, err := mutationInputs(nodes)
	if err != nil {
		return false, err
	}
	for _, n := range inputs {
		edges, err := n.InputEdges()
		if err != nil {
			return false, err
		}
		for _, e := range edges {
			if !e.Unique {
				return true, nil
			}
		}
	}
	return false, nil
}

// The mutation* helpers below return a complete Go statement applying one
// field/edge operation to a mutation builder. In classic mode they emit the
// typed setter (e.g. m.SetStatus(*v)); in split-runtime mode they emit the
// generic entbuilder.Mutation API (e.g. _ = m.SetField("status", *v)) keyed by
// the schema (snake_case) field/edge name. They take the already-formed Go value
// expression so the template controls *v vs v vs i.Field. The leading split bool
// is supplied by the template (via gqlgoSplitRuntime $), keeping the behaviour
// table-testable without any global state.

func mutationSetField(split bool, f *InputFieldDescriptor, valueExpr string) string {
	if split {
		return fmt.Sprintf("_ = m.SetField(%q, %s)", f.Name, valueExpr)
	}
	return fmt.Sprintf("m.%s(%s)", f.MutationSet(), valueExpr)
}

func mutationClearField(split bool, f *InputFieldDescriptor) string {
	if split {
		return fmt.Sprintf("_ = m.ClearField(%q)", f.Name)
	}
	return fmt.Sprintf("m.%s()", "Clear"+f.StructField())
}

func mutationAppendField(split bool, f *InputFieldDescriptor, valueExpr string) string {
	if split {
		return fmt.Sprintf("_ = m.AppendField(%q, %s)", f.Name, valueExpr)
	}
	return fmt.Sprintf("m.%s(%s)", f.MutationAppend(), valueExpr)
}

func mutationSetEdgeID(split bool, e *gen.Edge, valueExpr string) string {
	if split {
		return fmt.Sprintf("_ = m.SetEdgeID(%q, %s)", e.Name, valueExpr)
	}
	return fmt.Sprintf("m.%s(%s)", e.MutationSet(), valueExpr)
}

func mutationAddEdgeIDs(split bool, e *gen.Edge, valueExpr string) string {
	if split {
		return fmt.Sprintf("_ = m.AddEdgeIDs(%q, entbuilder.ToAny(%s)...)", e.Name, valueExpr)
	}
	return fmt.Sprintf("m.%s(%s...)", e.MutationAdd(), valueExpr)
}

func mutationRemoveEdgeIDs(split bool, e *gen.Edge, valueExpr string) string {
	if split {
		return fmt.Sprintf("_ = m.RemoveEdgeIDs(%q, entbuilder.ToAny(%s)...)", e.Name, valueExpr)
	}
	return fmt.Sprintf("m.%s(%s...)", e.MutationRemove(), valueExpr)
}

func mutationClearEdge(split bool, e *gen.Edge) string {
	if split {
		return fmt.Sprintf("_ = m.ClearEdge(%q)", e.Name)
	}
	return fmt.Sprintf("m.%s()", e.MutationClear())
}

// The edge*Expr helpers below return the Go expression to traverse or
// eager-load edge e of node n. In classic mode they emit the per-entity method
// the entity/query type carries (source.QueryParent(), query.WithParent()). The
// MatthewsREIS/ent fork's split layout has no such methods; instead the edge
// query/eager-load bodies are hoisted to package-level functions named
// Query<TypeName><EdgeStructField> / With<TypeName><EdgeStructField> (aliased in
// the gen root). The query form takes the typed per-entity client plus the
// entity; the eager-load form takes the query plus optional sub-query option
// closures. The leading split bool is supplied by the template (via
// gqlgoSplitRuntime $), keeping the behaviour table-testable without any global
// state.

func edgeQueryExpr(split bool, n *gen.Type, e *gen.Edge, entPkg, entityExpr, typedClientExpr string) string {
	if split {
		return fmt.Sprintf("%s.Query%s%s(%s, %s)", entPkg, n.Name, e.StructField(), typedClientExpr, entityExpr)
	}
	return fmt.Sprintf("%s.Query%s()", entityExpr, e.StructField())
}

func edgeWithExpr(split bool, n *gen.Type, e *gen.Edge, entPkg, queryExpr string, args ...string) string {
	if split {
		callArgs := append([]string{queryExpr}, args...)
		return fmt.Sprintf("%s.With%s%s(%s)", entPkg, n.Name, e.StructField(), strings.Join(callArgs, ", "))
	}
	return fmt.Sprintf("%s.With%s(%s)", queryExpr, e.StructField(), strings.Join(args, ", "))
}

func (m *MutationDescriptor) skip(immutable bool, skip SkipMode) bool {
	if m.IsCreate {
		return skip.Is(SkipMutationCreateInput)
	}
	return immutable || skip.Is(SkipMutationUpdateInput)
}

// HasInput reports whether the mutation input would expose at least one
// GraphQL input field (a scalar/enum field or an edge ID field). graphql-go
// rejects an InputObject with zero fields at schema-construction time, so an
// input that filters down to nothing must be omitted entirely — along with its
// Parse function, Go struct, and create/update mutation field. This mirrors the
// set of fields the schema/mutation_input templates actually emit.
func (m *MutationDescriptor) HasInput() (bool, error) {
	fields, err := m.InputFields()
	if err != nil {
		return false, err
	}
	if len(fields) > 0 {
		return true, nil
	}
	edges, err := m.InputEdges()
	if err != nil {
		return false, err
	}
	return len(edges) > 0, nil
}

// mutationInputs returns the list of input types for the mutation.
//
// Inputs that would render with zero GraphQL fields (e.g. a pure join entity
// whose only fields are edge-bound foreign keys that are themselves skipped,
// leaving no non-edge fields, combined with no eligible edges) are omitted:
// emitting an empty graphql.InputObject makes graphql.NewSchema fail. Dropping
// the descriptor here keeps the input type, its Parse function, the Go struct,
// and the create/update mutation field consistent (all four iterate this list).
func mutationInputs(nodes []*gen.Type) ([]*MutationDescriptor, error) {
	filteredNodes := make([]*MutationDescriptor, 0, len(nodes))
	for _, n := range nodes {
		ant, err := annotation(n.Annotations)
		if err != nil {
			return nil, err
		}
		for _, a := range ant.MutationInputs {
			if (a.IsCreate && ant.Skip.Is(SkipMutationCreateInput)) ||
				(!a.IsCreate && ant.Skip.Is(SkipMutationUpdateInput)) {
				continue
			}
			desc := &MutationDescriptor{
				Type:     n,
				IsCreate: a.IsCreate,
			}
			hasInput, err := desc.HasInput()
			if err != nil {
				return nil, err
			}
			if !hasInput {
				continue
			}
			filteredNodes = append(filteredNodes, desc)
		}
	}
	return filteredNodes, nil
}

// filterNodes filters out nodes that should not be included in the GraphQL schema.
func filterNodes(nodes []*gen.Type, skip SkipMode) ([]*gen.Type, error) {
	filteredNodes := make([]*gen.Type, 0, len(nodes))
	for _, n := range nodes {
		if n.HasCompositeID() {
			continue
		}
		ant, err := annotation(n.Annotations)
		if err != nil {
			return nil, err
		}
		if !ant.Skip.Is(skip) {
			filteredNodes = append(filteredNodes, n)
		}
	}
	return filteredNodes, nil
}

// filterEdges filters out edges that should not be included in the GraphQL schema.
func filterEdges(edges []*gen.Edge, skip SkipMode) ([]*gen.Edge, error) {
	filteredEdges := make([]*gen.Edge, 0, len(edges))
	for _, e := range edges {
		if e.Type.HasCompositeID() {
			continue
		}
		antE, err := annotation(e.Annotations)
		if err != nil {
			return nil, err
		}
		antT, err := annotation(e.Type.Annotations)
		if err != nil {
			return nil, err
		}
		if !antE.Skip.Is(skip) && !antT.Skip.Is(skip) {
			filteredEdges = append(filteredEdges, e)
		}
	}
	return filteredEdges, nil
}

// filterFields filters out fields that should not be included in the GraphQL schema.
func filterFields(fields []*gen.Field, skip SkipMode) ([]*gen.Field, error) {
	filteredFields := make([]*gen.Field, 0, len(fields))
	for _, f := range fields {
		ant, err := annotation(f.Annotations)
		if err != nil {
			return nil, err
		}
		if !ant.Skip.Is(skip) {
			filteredFields = append(filteredFields, f)
		}
	}
	return filteredFields, nil
}

// hasFieldNamed checks if a type has a field with the given name (case-insensitive camelCase comparison).
// This is used to avoid generating duplicate edge ID fields in mutation inputs.
func hasFieldNamed(t *gen.Type, name string) bool {
	name = strings.ToLower(name)
	for _, f := range t.Fields {
		if strings.ToLower(camel(f.Name)) == name {
			return true
		}
	}
	return false
}

// OrderTerm is a struct that represents a single GraphQL order term.
type OrderTerm struct {
	Owner *gen.Type
	GQL   string
	Type  *gen.Type
	Field *gen.Field
	Edge  *gen.Edge
	Count bool
}

// IsFieldTerm returns true if the order term is a type field term.
func (o *OrderTerm) IsFieldTerm() bool {
	return o.Field != nil && o.Edge == nil
}

// IsEdgeFieldTerm returns true if the order term is an edge field term.
func (o *OrderTerm) IsEdgeFieldTerm() bool {
	return o.Field != nil && o.Edge != nil
}

// IsEdgeCountTerm returns true if the order term is an edge count term.
func (o *OrderTerm) IsEdgeCountTerm() bool {
	return o.Field == nil && o.Edge != nil && o.Count
}

// IsNonUniqueEdgeFieldTerm returns true if the order term is a non-unique edge field term.
func (o *OrderTerm) IsNonUniqueEdgeFieldTerm() bool {
	return o.Field != nil && o.Edge != nil && !o.Edge.Unique
}

// VarName returns the name of the variable holding the order term.
func (o *OrderTerm) VarName() (string, error) {
	switch prefix := paginationNames(o.Owner.Name).OrderField; {
	case o.IsFieldTerm():
		return prefix + o.Field.StructField(), nil
	case o.IsEdgeFieldTerm() && o.Edge.Unique:
		return prefix + o.Edge.StructField() + o.Field.StructField(), nil
	case o.IsNonUniqueEdgeFieldTerm():
		return prefix + o.Edge.StructField() + o.Field.StructField(), nil
	case o.IsEdgeCountTerm():
		return prefix + o.Edge.StructField() + "Count", nil
	default:
		return "", fmt.Errorf("entgqlgo: invalid order term %v", o)
	}
}

// VarField returns the field name inside the variable holding the order term.
func (o *OrderTerm) VarField() (string, error) {
	switch {
	case o.IsFieldTerm():
		return fmt.Sprintf("%s.%s", o.Type.Package(), o.Field.Constant()), nil
	case o.IsEdgeFieldTerm(), o.IsEdgeCountTerm():
		return strconv.Quote(strings.ToLower(o.GQL)), nil
	case o.IsNonUniqueEdgeFieldTerm():
		return fmt.Sprintf("%s.%s", o.Type.Package(), o.Field.Constant()), nil
	default:
		return "", fmt.Errorf("entgqlgo: invalid order term %v", o)
	}
}

// orderFields returns the GraphQL fields of the given node with the `OrderField` annotation.
func orderFields(n *gen.Type) ([]*OrderTerm, error) {
	var (
		terms  []*OrderTerm
		fields = n.Fields
	)
	if n.HasOneFieldID() {
		fields = append([]*gen.Field{n.ID}, fields...)
	}
	for _, f := range fields {
		switch ant, err := annotation(f.Annotations); {
		case err != nil:
			return nil, err
		case ant.Skip.Is(SkipOrderField), len(ant.OrderField) == 0:
		case !f.Type.Comparable():
			return nil, fmt.Errorf("entgqlgo: ordered field %s.%s must be comparable", n.Name, f.Name)
		default:
			if len(ant.OrderField) > 1 {
				return nil, fmt.Errorf("entgqlgo: ordered field %s.%s has more than one order field", n.Name, f.Name)
			}
			if len(ant.OrderField) == 1 {
				terms = append(terms, &OrderTerm{
					Owner: n,
					GQL:   ant.OrderField[0],
					Type:  n,
					Field: f,
				})
			}
		}
	}
	for _, e := range n.Edges {
		name := strings.ToUpper(e.Name)
		switch ant, err := annotation(e.Annotations); {
		case err != nil:
			return nil, err
		case ant.Skip.Is(SkipOrderField), len(ant.OrderField) == 0:
		case len(lo.Filter(ant.OrderField, func(item string, index int) bool {
			return item == fmt.Sprintf("%s_COUNT", name)
		})) > 0:
			for _, field := range ant.OrderField {
				if field == fmt.Sprintf("%s_COUNT", name) {
					if _, err := e.OrderCountName(); err != nil {
						return nil, fmt.Errorf("entgqlgo: invalid order field %s defined on edge %s.%s: %w", ant.OrderField, n.Name, e.Name, err)
					}
					terms = append(terms, &OrderTerm{
						Owner: n,
						GQL:   field,
						Type:  n,
						Edge:  e,
						Count: true,
					})
				}
			}
		case len(lo.Filter(ant.OrderField, func(item string, index int) bool {
			return strings.HasPrefix(item, name+"_")
		})) > 0:
			for _, field := range ant.OrderField {
				if e.Unique {
					if _, err := e.OrderFieldName(); err != nil {
						return nil, fmt.Errorf("entgqlgo: invalid order field %s defined on edge %s.%s: %w", ant.OrderField, n.Name, e.Name, err)
					}
				} else {
					if _, err := e.OrderTermsName(); err != nil {
						return nil, fmt.Errorf("entgqlgo: invalid order field %s defined on edge %s.%s: %w", ant.OrderField, n.Name, e.Name, err)
					}
				}
				ef := strings.TrimPrefix(field, name+"_")
				idx := slices.IndexFunc(e.Type.Fields, func(f *gen.Field) bool {
					ant, err := annotation(f.Annotations)
					return err == nil && lo.Contains(ant.OrderField, ef)
				})
				if idx == -1 {
					return nil, fmt.Errorf("entgqlgo: order field %s defined on edge %s.%s was not found on its reference", ant.OrderField, n.Name, e.Name)
				}
				terms = append(terms, &OrderTerm{
					Owner: n,
					GQL:   field,
					Edge:  e,
					Type:  e.Type,
					Field: e.Type.Fields[idx],
				})
			}
		default:
			return nil, fmt.Errorf("entgqlgo: invalid order field defined on edge %s.%s", n.Name, e.Name)
		}
	}
	return terms, nil
}

// hasWhereInput returns true if neither the edge nor its
// node type has the SkipWhereInput annotation
func hasWhereInput(n *gen.Edge) (v bool, err error) {
	antEdge, err := annotation(n.Annotations)
	if err != nil || antEdge.Skip.Is(SkipWhereInput) {
		return false, err
	}
	ant, err := annotation(n.Type.Annotations)
	if err != nil || ant.Skip.Is(SkipWhereInput) {
		return false, err
	}
	return true, nil
}

// skipModeFromString returns SkipFlag from a string
func skipModeFromString(modes ...string) (SkipMode, error) {
	var m SkipMode
	for _, s := range modes {
		switch s {
		case "type":
			m |= SkipType
		case "enum_field":
			m |= SkipEnumField
		case "order_field":
			m |= SkipOrderField
		case "where_input":
			m |= SkipWhereInput
		case "mutation_create_input":
			m |= SkipMutationCreateInput
		case "mutation_update_input":
			m |= SkipMutationUpdateInput
		default:
			return 0, fmt.Errorf("invalid skip mode: %s", s)
		}
	}
	return m, nil
}

func trimPrefix(source, prefix string) string {
	return strings.TrimPrefix(source, prefix)
}

func isSkipMode(antSkip interface{}, m string) (bool, error) {
	skip, err := skipModeFromString(m)
	if err != nil || antSkip == nil {
		return false, err
	}
	if raw, ok := antSkip.(float64); ok {
		return SkipMode(raw).Is(skip), nil
	}
	return false, fmt.Errorf("invalid annotation skip: %v", antSkip)
}

func isRelayConn(e *gen.Edge) (bool, error) {
	ant, err := annotation(e.Annotations)
	if err != nil {
		return false, err
	}
	return ant.RelayConnection, nil
}

// PaginationNames holds the names of the pagination fields.
type PaginationNames struct {
	Connection string
	Edge       string
	Node       string
	Order      string
	OrderField string
	WhereInput string
}

func gqlTypeFromNode(t *gen.Type) (gqlType string, ant *Annotation, err error) {
	if ant, err = annotation(t.Annotations); err != nil {
		return
	}
	gqlType = t.Name
	if ant.Type != "" {
		gqlType = ant.Type
	}
	return
}

// nodePaginationNames returns the names of the pagination types for the node.
func nodePaginationNames(t *gen.Type) (*PaginationNames, error) {
	node, _, err := gqlTypeFromNode(t)
	if err != nil {
		return nil, err
	}
	return paginationNames(node), nil
}

func paginationNames(node string) *PaginationNames {
	return &PaginationNames{
		Connection: fmt.Sprintf("%sConnection", node),
		Edge:       fmt.Sprintf("%sEdge", node),
		Node:       node,
		Order:      fmt.Sprintf("%sOrder", node),
		OrderField: fmt.Sprintf("%sOrderField", node),
		WhereInput: fmt.Sprintf("%sWhereInput", node),
	}
}

func skipMutationTemplate(g *gen.Graph) bool {
	for _, n := range g.Nodes {
		ant, err := annotation(n.Annotations)
		if err != nil {
			continue
		}
		for _, i := range ant.MutationInputs {
			if (i.IsCreate && !ant.Skip.Is(SkipMutationCreateInput)) ||
				(!i.IsCreate && !ant.Skip.Is(SkipMutationUpdateInput)) {
				return false
			}
		}
	}
	return true
}

// queryFieldName returns the root Query field name for the node, or "" if the
// node has no QueryField annotation.
func queryFieldName(t *gen.Type) (string, error) {
	gqlType, ant, err := gqlTypeFromNode(t)
	if err != nil {
		return "", err
	}
	if ant.QueryField == nil {
		return "", nil
	}
	return ant.QueryField.fieldName(gqlType), nil
}

// queryFieldDescription returns the description of the node's root Query field.
func queryFieldDescription(t *gen.Type) (string, error) {
	_, ant, err := gqlTypeFromNode(t)
	if err != nil || ant.QueryField == nil {
		return "", err
	}
	return ant.QueryField.Description, nil
}

// isRelayConnNode reports whether the node itself (not an edge) has the
// RelayConnection annotation.
func isRelayConnNode(t *gen.Type) (bool, error) {
	ant, err := annotation(t.Annotations)
	if err != nil {
		return false, err
	}
	return ant.RelayConnection, nil
}

func nodeImplementors(n *gen.Type) (ifaces []string, err error) {
	ant, err := annotation(n.Annotations)
	if err != nil {
		return nil, err
	}
	if !ant.Skip.Is(SkipType) {
		ifaces = append(ifaces, "Node")
	}
	return ifaces, nil
}

func nodeImplementorsVar(n *gen.Type) string {
	return strings.ToLower(n.Name) + "Implementors"
}

// fieldMapping returns the GraphQL names mapping of a field.
func fieldMapping(f *gen.Field) ([]string, error) {
	ant, err := annotation(f.Annotations)
	if err != nil || ant.Skip.Is(SkipType) || f.Sensitive() {
		return nil, err
	}
	if len(ant.Mapping) > 0 {
		return ant.Mapping, nil
	}
	return []string{camel(f.Name)}, nil
}

// gqlgoType maps an ent field type to graphql-go type.
// For enum fields, templates should use the generated enum type directly.
//
// It returns an error when a field carries a malformed entgqlgo.Type
// annotation so that code generation fails loudly rather than emitting a
// silently-wrong schema. An unknown but well-formed named type (e.g. "Upload")
// is NOT an error: it resolves via the generated CustomTypes registry.
func gqlgoType(f *gen.Field) (string, error) {
	// A custom Type annotation takes precedence over the field's Go type for
	// every field kind (mirrors entgql's mapScalar, which returns ant.Type
	// before any built-in mapping). This is what lets a Bytes field annotated
	// with entgqlgo.Type("Upload") resolve to the Upload scalar instead of the
	// Bytes default. The annotation value is a GraphQL SDL type expression
	// (e.g. "Upload", "[String!]") translated into a graphql-go Go expression.
	if ant, err := annotation(f.Annotations); err == nil && ant.Type != "" {
		expr, err := sdlTypeToGo(ant.Type)
		if err != nil {
			return "", fmt.Errorf("entgqlgo: field %q has invalid Type annotation %q: %w", f.Name, ant.Type, err)
		}
		return expr, nil
	}
	switch t := f.Type.Type; {
	case f.Name == "id":
		return "graphql.ID", nil
	case f.IsEdgeField():
		return "graphql.ID", nil
	case t.Float():
		return "graphql.Float", nil
	case t.Integer():
		return "graphql.Int", nil
	case t == field.TypeString:
		return "graphql.String", nil
	case t == field.TypeBool:
		return "graphql.Boolean", nil
	case t == field.TypeTime:
		return "TimeScalar", nil
	case t == field.TypeUUID:
		// entgql maps a UUID field to the "UUID" scalar (its Go type carries a
		// package path, so mapScalar falls through to the bare type name). Route
		// through the CustomTypes registry so consumers can register a real UUID
		// scalar; fall back to graphql.ID to preserve prior behaviour when
		// unregistered.
		return `customTypeOr("UUID", graphql.ID)`, nil
	case t == field.TypeBytes:
		return "graphql.String", nil // Bytes serialized as base64 string (override with entgqlgo.Type())
	case t == field.TypeJSON:
		// A map[string]interface{} JSON field maps to entgql's "Map" scalar.
		// Route through the CustomTypes registry so consumers can register a Map
		// scalar; fall back to graphql.String when unregistered.
		if isJSONMap(f) {
			return `customTypeOr("Map", graphql.String)`, nil
		}
		// Check if the underlying Go type is a slice (e.g. []string, []int).
		if inner, ok := sliceElementGraphQLType(f.Type.String()); ok {
			return "graphql.NewList(graphql.NewNonNull(" + inner + "))", nil
		}
		return "graphql.String", nil // JSON serialized as string; use entgqlgo.Type() annotation for custom scalars
	case t == field.TypeEnum:
		return "graphql.String", nil // Enums handled separately in templates
	case t == field.TypeOther:
		// The Type annotation was already handled at the top of the function. An
		// Other field without one falls back to a slice element type or String.
		if inner, ok := sliceElementGraphQLType(f.Type.String()); ok {
			return "graphql.NewList(graphql.NewNonNull(" + inner + "))", nil
		}
		return "graphql.String", nil // Other types require entgqlgo.Type() annotation
	default:
		return "graphql.String", nil
	}
}

// isJSONMap reports whether a JSON field's underlying Go type is
// map[string]interface{} — the shape entgql maps to its "Map" scalar.
// It matches on both the reflect kind/ident (when RType is populated) and the
// field's Go type string, so it works whether or not the field carries RType.
func isJSONMap(f *gen.Field) bool {
	if rt := f.Type.RType; rt != nil && rt.Kind == reflect.Map && rt.Ident == "map[string]interface {}" {
		return true
	}
	switch f.Type.String() {
	case "map[string]interface{}", "map[string]interface {}":
		return true
	default:
		return false
	}
}

// sdlBuiltinScalars maps GraphQL built-in scalar names to their graphql-go
// expression. Names not present here are treated as custom types and resolved
// at runtime via the generated CustomTypes registry.
var sdlBuiltinScalars = map[string]string{
	"String":  "graphql.String",
	"Int":     "graphql.Int",
	"Float":   "graphql.Float",
	"Boolean": "graphql.Boolean",
	"ID":      "graphql.ID",
	"Time":    "TimeScalar",
}

// sdlTypeToGo translates a GraphQL SDL type expression (as supplied via an
// entgqlgo.Type annotation) into a graphql-go Go expression suitable for
// emitting directly into generated source.
//
// Supported grammar (named type with optional list/non-null wrappers):
//
//	Type    := List | NonNull | Named
//	List    := "[" Type "]"
//	NonNull := (List | Named) "!"
//	Named   := identifier
//
// Built-in scalars (String, Int, Float, Boolean, ID, Time) map to the
// corresponding graphql-go value. Any other named type is emitted as a lookup
// into the generated CustomTypes registry with a graphql.String fallback, e.g.
// customTypeOr("AppAuthMethod", graphql.String).
func sdlTypeToGo(sdl string) (string, error) {
	expr := strings.TrimSpace(sdl)
	if expr == "" {
		return "", fmt.Errorf("entgqlgo: empty SDL type expression")
	}
	// Trailing "!" makes the (list or named) type non-null.
	if strings.HasSuffix(expr, "!") {
		inner, err := sdlTypeToGo(expr[:len(expr)-1])
		if err != nil {
			return "", err
		}
		return "graphql.NewNonNull(" + inner + ")", nil
	}
	// "[X]" is a list of X.
	if strings.HasPrefix(expr, "[") {
		if !strings.HasSuffix(expr, "]") {
			return "", fmt.Errorf("entgqlgo: unbalanced list brackets in SDL type %q", sdl)
		}
		inner, err := sdlTypeToGo(expr[1 : len(expr)-1])
		if err != nil {
			return "", err
		}
		return "graphql.NewList(" + inner + ")", nil
	}
	if strings.ContainsAny(expr, "[]!") {
		return "", fmt.Errorf("entgqlgo: malformed SDL type %q", sdl)
	}
	// Bare named type.
	if !isSDLName(expr) {
		return "", fmt.Errorf("entgqlgo: invalid SDL type name %q", sdl)
	}
	if builtin, ok := sdlBuiltinScalars[expr]; ok {
		return builtin, nil
	}
	// Unknown named type: resolve via the generated CustomTypes registry with a
	// graphql.String fallback so generation never produces invalid Go.
	return "customTypeOr(\"" + expr + "\", graphql.String)", nil
}

// isSDLName reports whether s is a valid GraphQL name: /[_A-Za-z][_0-9A-Za-z]*/.
func isSDLName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// isDeprecatedEnumValue reports whether the given enum value is listed in the
// field's DeprecatedEnumValues annotation.
func isDeprecatedEnumValue(f *gen.Field, value string) (bool, error) {
	ant, err := annotation(f.Annotations)
	if err != nil {
		return false, err
	}
	return slices.Contains(ant.DeprecatedEnumValues, value), nil
}

// gqlgoImplements returns the list of custom GraphQL interface names declared via
// the Implements annotation on the given node. Returns nil (empty slice) if the
// node has no annotation or no Implements entries, so range loops are safe.
func gqlgoImplements(n *gen.Type) ([]string, error) {
	ant, err := annotation(n.Annotations)
	if err != nil {
		return nil, err
	}
	return ant.Implements, nil
}

// sliceElementGraphQLType checks if a Go type string represents a slice and returns
// the corresponding graphql-go scalar type for the element type.
// For example, "[]string" returns ("graphql.String", true).
func sliceElementGraphQLType(goType string) (string, bool) {
	if !strings.HasPrefix(goType, "[]") {
		return "", false
	}
	elem := goType[2:]
	switch elem {
	case "string":
		return "graphql.String", true
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return "graphql.Int", true
	case "float32", "float64":
		return "graphql.Float", true
	case "bool":
		return "graphql.Boolean", true
	default:
		return "graphql.String", true // Default to String for unknown element types
	}
}

// gqlgoDecodeField emits the Go statements that decode a single mutation-input
// field value into the parser's result struct. It is the heart of the
// silent-data-loss fix: previously the schema template only knew how to decode
// int/string/bool fields and emitted an empty "// Handle <type>" stub for every
// other Go type, silently discarding the value. This helper decodes every Go
// type an ent field can carry (float, sized ints, time.Time, uuid.UUID, JSON
// maps/slices, []byte, named slice types such as pq.StringArray, ...).
//
// The value arrives from graphql-go's input coercion: each InputObject field is
// run through its GraphQL type's ParseValue (variables path) or ParseLiteral
// (inline-literal path). The concrete Go shape therefore depends on the GraphQL
// type entgqlgo assigned the field (see gqlgoType): graphql.Int -> int,
// graphql.Float -> float64, graphql.String -> string, TimeScalar -> time.Time,
// a list type -> []interface{}, and a custom scalar (UUID/Map/...) -> whatever
// that scalar's ParseValue returns, or the graphql.String/graphql.ID fallback
// (string) when the consumer never registered it. Because the same field can
// arrive in more than one shape (e.g. a Map field is map[string]interface{}
// when a Map scalar is registered but a JSON string otherwise), the emitted
// code accepts every plausible arrival shape defensively and only errors on a
// value it genuinely cannot interpret — never a silent skip, never a panic.
//
//   - f          the ent field being decoded (its Go type drives the cases).
//   - isPointer  whether the result struct field is a pointer (then we decode
//     into a temporary and assign its address).
//   - valueVar   the already-non-nil interface{} value variable name (e.g. "v").
//   - target     the assignment target (e.g. "result.Score").
//
// Enum, plain int, plain string and plain bool fields are handled inline by the
// template and never reach this helper.
func gqlgoDecodeField(f *gen.Field, isPointer bool, valueVar, target string) (string, error) {
	goType := f.Type.String()
	jsonTag := f.Name
	// assign emits the code that stores a value expression of the field's base
	// Go type into target, taking its address first when the struct field is a
	// pointer. tmpVar is a fresh local used when an address must be taken.
	assign := func(valueExpr string) string {
		if isPointer {
			return fmt.Sprintf("dv := %s\n\t\t\t%s = &dv", valueExpr, target)
		}
		return fmt.Sprintf("%s = %s", target, valueExpr)
	}
	errLine := func() string {
		return fmt.Sprintf("return nil, fmt.Errorf(\"field %s: invalid value %%v (%%T)\", %s, %s)", jsonTag, valueVar, valueVar)
	}

	switch {
	// time.Time: TimeScalar.ParseValue/ParseLiteral already returns time.Time;
	// fall back to RFC3339 parsing if a raw string slips through.
	case goType == "time.Time":
		return fmt.Sprintf(`switch tv := %s.(type) {
		case time.Time:
			%s
		case string:
			tt, err := time.Parse(time.RFC3339, tv)
			if err != nil {
				return nil, fmt.Errorf("field %s: invalid time %%q: %%w", tv, err)
			}
			%s
		default:
			%s
		}`, valueVar, assign("tv"), jsonTag, assign("tt"), errLine()), nil

	// uuid.UUID: arrives as a string via the UUID/ID scalar (parse it), or as a
	// uuid.UUID directly if a typed UUID scalar is registered.
	case goType == "uuid.UUID":
		return fmt.Sprintf(`switch uv := %s.(type) {
		case uuid.UUID:
			%s
		case string:
			uu, err := uuid.Parse(uv)
			if err != nil {
				return nil, fmt.Errorf("field %s: invalid uuid %%q: %%w", uv, err)
			}
			%s
		case [16]byte:
			uu := uuid.UUID(uv)
			%s
		default:
			%s
		}`, valueVar, assign("uv"), jsonTag, assign("uu"), assign("uu"), errLine()), nil

	// []byte: arrives as a base64 string (graphql.String fallback) or as raw
	// bytes / a string if a custom Bytes scalar is registered.
	case goType == "[]byte":
		return fmt.Sprintf(`switch bv := %s.(type) {
		case []byte:
			%s
		case string:
			if decoded, err := base64.StdEncoding.DecodeString(bv); err == nil {
				%s
			} else {
				%s
			}
		default:
			%s
		}`, valueVar, assign("bv"), assignBytesDecoded(isPointer, target), assignBytesRaw(isPointer, target), errLine()), nil

	// JSON map (map[string]interface{} and friends), or any other JSON-backed Go
	// type (map[string]string, []int, custom structs, ...). The value is either
	// already the decoded Go value (Map scalar registered) or a JSON string
	// (graphql.String fallback) that we json.Unmarshal into the field's Go type.
	case f.Type.Type == field.TypeJSON && !isJSONSliceField(f):
		return jsonDecode(f, isPointer, valueVar, target, goType), nil

	// Slice-backed fields (Strings/Ints/Floats and named slice types such as
	// pq.StringArray). When annotated/typed as a GraphQL list the value arrives
	// as []interface{}; it may also arrive as a JSON string fallback.
	case isSliceGoType(goType):
		return sliceDecode(f, valueVar, target, goType), nil

	// Numeric: graphql.Float -> float64, graphql.Int -> int. Accept int, int64,
	// float64, json.Number and numeric strings, converting to the field's type.
	case f.Type.Numeric():
		return fmt.Sprintf(`switch nv := %s.(type) {
		case int:
			cv := %s(nv)
			%s
		case int64:
			cv := %s(nv)
			%s
		case float64:
			cv := %s(nv)
			%s
		case json.Number:
			fv, err := nv.Float64()
			if err != nil {
				return nil, fmt.Errorf("field %s: invalid number %%q: %%w", nv.String(), err)
			}
			cv := %s(fv)
			%s
		case string:
			fv, err := strconv.ParseFloat(nv, 64)
			if err != nil {
				return nil, fmt.Errorf("field %s: invalid number %%q: %%w", nv, err)
			}
			cv := %s(fv)
			%s
		default:
			%s
		}`,
			valueVar,
			goType, assign("cv"),
			goType, assign("cv"),
			goType, assign("cv"),
			jsonTag, goType, assign("cv"),
			jsonTag, goType, assign("cv"),
			errLine()), nil

	// Fallback for any remaining Go type (e.g. field.TypeOther without a more
	// specific case, or a custom scalar Go type): accept a string verbatim,
	// otherwise error rather than silently dropping the value.
	default:
		return fmt.Sprintf(`if sv, ok := %s.(string); ok {
			%s
		} else {
			%s
		}`, valueVar, assign(fmt.Sprintf("%s(sv)", goType)), errLine()), nil
	}
}

// assignBytesDecoded/assignBytesRaw emit the []byte assignment for the
// base64-decoded and raw-string fallbacks respectively.
func assignBytesDecoded(isPointer bool, target string) string {
	if isPointer {
		return fmt.Sprintf("dv := decoded\n\t\t\t\t%s = &dv", target)
	}
	return fmt.Sprintf("%s = decoded", target)
}

func assignBytesRaw(isPointer bool, target string) string {
	if isPointer {
		return fmt.Sprintf("dv := []byte(bv)\n\t\t\t\t%s = &dv", target)
	}
	return fmt.Sprintf("%s = []byte(bv)", target)
}

// jsonDecode emits decode code for a JSON-backed (non-slice) field: an already
// decoded Go value is asserted directly; a JSON string is json.Unmarshal'd into
// the field's Go type; a map[string]interface{} is re-marshalled then
// unmarshalled into the concrete Go type to coerce (e.g. into map[string]string
// or a struct) when the registered scalar handed us a generic map.
func jsonDecode(f *gen.Field, isPointer bool, valueVar, target, goType string) string {
	jsonTag := f.Name
	assignAddr := "" // how to store a value held in the variable "dec"
	if isPointer {
		assignAddr = fmt.Sprintf("dv := dec\n\t\t\t%s = &dv", target)
	} else {
		assignAddr = fmt.Sprintf("%s = dec", target)
	}
	return fmt.Sprintf(`switch jv := %s.(type) {
		case %s:
			dec := jv
			%s
		case string:
			var dec %s
			if err := json.Unmarshal([]byte(jv), &dec); err != nil {
				return nil, fmt.Errorf("field %s: invalid JSON: %%w", err)
			}
			%s
		default:
			raw, err := json.Marshal(jv)
			if err != nil {
				return nil, fmt.Errorf("field %s: cannot marshal value: %%w", err)
			}
			var dec %s
			if err := json.Unmarshal(raw, &dec); err != nil {
				return nil, fmt.Errorf("field %s: invalid JSON: %%w", err)
			}
			%s
		}`,
		valueVar,
		goType, assignAddr,
		goType, jsonTag, assignAddr,
		jsonTag, goType, jsonTag, assignAddr)
}

// sliceDecode emits decode code for slice-backed fields. A GraphQL list arrives
// as []interface{} whose elements are decoded per the slice's element type; a
// JSON-string fallback is json.Unmarshal'd into the whole slice type. The result
// is always assigned by value (slice fields are never pointers in the structs).
func sliceDecode(f *gen.Field, valueVar, target, goType string) string {
	jsonTag := f.Name
	elem := sliceElementGoType(goType)
	elemDecode := elementDecodeExpr(elem)
	return fmt.Sprintf(`switch sv := %s.(type) {
		case %s:
			%s = sv
		case []interface{}:
			out := make(%s, 0, len(sv))
			for _, item := range sv {
				%s
			}
			%s = out
		case string:
			var dec %s
			if err := json.Unmarshal([]byte(sv), &dec); err != nil {
				return nil, fmt.Errorf("field %s: invalid JSON list: %%w", err)
			}
			%s = dec
		default:
			return nil, fmt.Errorf("field %s: invalid value %%v (%%T)", %s, %s)
		}`,
		valueVar,
		goType, target,
		goType, fmt.Sprintf(elemDecode, "item"), target,
		goType, jsonTag, target,
		jsonTag, valueVar, valueVar)
}

// elementDecodeExpr returns a format string (with a single %s for the source
// element expression) that appends one decoded element of type elem to "out".
func elementDecodeExpr(elem string) string {
	switch elem {
	case "string":
		return `if s, ok := %[1]s.(string); ok {
					out = append(out, s)
				} else {
					out = append(out, fmt.Sprint(%[1]s))
				}`
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "rune", "byte":
		return `switch n := %[1]s.(type) {
				case int:
					out = append(out, ` + elem + `(n))
				case int64:
					out = append(out, ` + elem + `(n))
				case float64:
					out = append(out, ` + elem + `(n))
				}`
	case "float32", "float64":
		return `switch n := %[1]s.(type) {
				case float64:
					out = append(out, ` + elem + `(n))
				case int:
					out = append(out, ` + elem + `(n))
				case int64:
					out = append(out, ` + elem + `(n))
				}`
	case "bool":
		return `if b, ok := %[1]s.(bool); ok {
					out = append(out, b)
				}`
	default:
		// Unknown element type: best-effort assert to the element type directly.
		return `if e, ok := %[1]s.(` + elem + `); ok {
					out = append(out, e)
				}`
	}
}

// sliceElementGoType returns the element Go type of a slice type string, looking
// through a named slice alias if necessary. For "[]string" it returns "string";
// for a named type like "pq.StringArray" it returns "string" (its underlying
// element), inferred from the well-known aliases entgql/ent emit. For an unknown
// named slice type it returns "interface{}" so the decoded []interface{} can be
// asserted element-wise.
func sliceElementGoType(goType string) string {
	if strings.HasPrefix(goType, "[]") {
		return goType[2:]
	}
	switch goType {
	case "pq.StringArray":
		return "string"
	case "pq.Int64Array":
		return "int64"
	case "pq.Float64Array":
		return "float64"
	case "pq.BoolArray":
		return "bool"
	default:
		return "interface{}"
	}
}

// isSliceGoType reports whether the Go type string denotes a slice value,
// including named slice aliases (pq.StringArray and friends).
func isSliceGoType(goType string) bool {
	if strings.HasPrefix(goType, "[]") {
		return true
	}
	switch goType {
	case "pq.StringArray", "pq.Int64Array", "pq.Float64Array", "pq.BoolArray":
		return true
	default:
		return false
	}
}

// isJSONSliceField reports whether a JSON-backed field's Go type is a slice
// (e.g. field.Strings -> []string, or a named slice alias). Such fields decode
// through sliceDecode (list shape) rather than jsonDecode (object shape).
func isJSONSliceField(f *gen.Field) bool {
	return isSliceGoType(f.Type.String())
}

// gqlgoScalar maps an ent field type to graphql-go scalar string.
// For enum fields, templates should use the generated enum type directly.
func gqlgoScalar(f *gen.Field) string {
	switch t := f.Type.Type; {
	case f.Name == "id":
		return "ID"
	case f.IsEdgeField():
		return "ID"
	case t.Float():
		return "Float"
	case t.Integer():
		return "Int"
	case t == field.TypeString:
		return "String"
	case t == field.TypeBool:
		return "Boolean"
	case t == field.TypeTime:
		return "Time"
	case t == field.TypeUUID:
		return "String" // UUID serialized as string
	case t == field.TypeBytes:
		return "String" // Bytes serialized as base64 string
	case t == field.TypeJSON:
		return "String" // JSON serialized as string; use entgqlgo.Type() annotation for custom scalars
	case t == field.TypeEnum:
		return "String" // Enums handled separately in templates
	case t == field.TypeOther:
		return "String" // Other types require entgqlgo.Type() annotation
	default:
		return "String"
	}
}
