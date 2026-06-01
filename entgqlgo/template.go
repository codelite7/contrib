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
		"gqlgoFieldCollections":    fieldCollections,
		"gqlgoFieldMapping":        fieldMapping,
		"gqlgoFilterEdges":         filterEdges,
		"gqlgoFilterFields":        filterFields,
		"gqlgoFilterNodes":         filterNodes,
		"gqlgoIDType":              gqlIDType,
		"gqlgoHasWhereInput":       hasWhereInput,
		"gqlgoIsRelayConn":         isRelayConn,
		"gqlgoIsSkipMode":          isSkipMode,
		"gqlgoMutationInputs":      mutationInputs,
		"gqlgoNodeImplementors":    nodeImplementors,
		"gqlgoNodeImplementorsVar": nodeImplementorsVar,
		"gqlgoNodePaginationNames": nodePaginationNames,
		"gqlgoOrderFields":         orderFields,
		"gqlgoSkipMode":            skipModeFromString,
		"gqlgoTrimPrefix":          trimPrefix,
		"gqlgoType":                gqlgoType,
		"gqlgoScalar":              gqlgoScalar,
		"gqlgoHasFieldNamed":       hasFieldNamed,
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

func (m *MutationDescriptor) skip(immutable bool, skip SkipMode) bool {
	if m.IsCreate {
		return skip.Is(SkipMutationCreateInput)
	}
	return immutable || skip.Is(SkipMutationUpdateInput)
}

// mutationInputs returns the list of input types for the mutation.
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
			filteredNodes = append(filteredNodes, &MutationDescriptor{
				Type:     n,
				IsCreate: a.IsCreate,
			})
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
func gqlgoType(f *gen.Field) string {
	switch t := f.Type.Type; {
	case f.Name == "id":
		return "graphql.ID"
	case f.IsEdgeField():
		return "graphql.ID"
	case t.Float():
		return "graphql.Float"
	case t.Integer():
		return "graphql.Int"
	case t == field.TypeString:
		return "graphql.String"
	case t == field.TypeBool:
		return "graphql.Boolean"
	case t == field.TypeTime:
		return "TimeScalar"
	case t == field.TypeUUID:
		return "graphql.ID" // UUID maps to ID to match gqlgen convention
	case t == field.TypeBytes:
		return "graphql.String" // Bytes serialized as base64 string
	case t == field.TypeJSON:
		// Check for custom type annotation first.
		if ant, err := annotation(f.Annotations); err == nil && ant.Type != "" {
			return ant.Type
		}
		// Check if the underlying Go type is a slice (e.g. []string, []int).
		if inner, ok := sliceElementGraphQLType(f.Type.String()); ok {
			return "graphql.NewList(graphql.NewNonNull(" + inner + "))"
		}
		return "graphql.String" // JSON serialized as string; use entgqlgo.Type() annotation for custom scalars
	case t == field.TypeEnum:
		return "graphql.String" // Enums handled separately in templates
	case t == field.TypeOther:
		// Check for custom type annotation first.
		if ant, err := annotation(f.Annotations); err == nil && ant.Type != "" {
			return ant.Type
		}
		// Check if the underlying Go type is a slice (e.g. []string, []int).
		if inner, ok := sliceElementGraphQLType(f.Type.String()); ok {
			return "graphql.NewList(graphql.NewNonNull(" + inner + "))"
		}
		return "graphql.String" // Other types require entgqlgo.Type() annotation
	default:
		return "graphql.String"
	}
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
