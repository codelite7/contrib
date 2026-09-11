package entgql

import (
	"fmt"
	"slices"

	"entgo.io/ent/entc/gen"
)

type unionDef struct {
	Type    string
	Members []string
}

func nodeUnions(n *gen.Type) ([]UnionFieldSpec, error) {
	ant, err := annotation(n.Annotations)
	if err != nil {
		return nil, err
	}
	return ant.Unions, nil
}

func unionMemberEdges(n *gen.Type, u UnionFieldSpec) ([]*gen.Edge, error) {
	edges := make([]*gen.Edge, 0, len(u.Edges))
	for _, name := range u.Edges {
		i := slices.IndexFunc(n.Edges, func(e *gen.Edge) bool { return e.Name == name })
		if i < 0 {
			return nil, fmt.Errorf("entgql: union %q on %s names edge %q which does not exist", u.Type, n.Name, name)
		}
		edges = append(edges, n.Edges[i])
	}
	return edges, nil
}

func isUnionMember(n *gen.Type, e *gen.Edge) (bool, error) {
	unions, err := nodeUnions(n)
	if err != nil {
		return false, err
	}
	for _, u := range unions {
		if slices.Contains(u.Edges, e.Name) {
			return true, nil
		}
	}
	return false, nil
}

func nonUnionEdges(n *gen.Type) ([]*gen.Edge, error) {
	visible, err := filterEdges(n.Edges, SkipType)
	if err != nil {
		return nil, err
	}
	out := visible[:0:0]
	for _, e := range visible {
		member, err := isUnionMember(n, e)
		if err != nil {
			return nil, err
		}
		if !member {
			out = append(out, e)
		}
	}
	return out, nil
}

func collectUnions(g *gen.Graph) (map[string]*unionDef, error) {
	defs := map[string]*unionDef{}
	for _, n := range g.Nodes {
		unions, err := nodeUnions(n)
		if err != nil {
			return nil, err
		}
		for _, u := range unions {
			if len(u.Edges) < 2 {
				return nil, fmt.Errorf("entgql: union %q on %s needs at least two member edges", u.Type, n.Name)
			}
			edges, err := unionMemberEdges(n, u)
			if err != nil {
				return nil, err
			}
			members := make([]string, 0, len(edges))
			for _, e := range edges {
				edgeAnt, err := annotation(e.Annotations)
				if err != nil {
					return nil, err
				}
				switch {
				case !e.Unique:
					return nil, fmt.Errorf("entgql: union %q on %s: edge %q must be unique", u.Type, n.Name, e.Name)
				case edgeAnt.Skip.Is(SkipType):
					return nil, fmt.Errorf("entgql: union %q on %s: edge %q is itself skipped", u.Type, n.Name, e.Name)
				case !e.Optional:
					// The union field resolves to null when no member is set, so a
					// required member edge promises something the field cannot express.
					return nil, fmt.Errorf("entgql: union %q on %s: edge %q is required, but a union field is always nullable", u.Type, n.Name, e.Name)
				case edgeAnt.RelayConnection:
					return nil, fmt.Errorf("entgql: union %q on %s: edge %q cannot be a relay connection", u.Type, n.Name, e.Name)
				}
				gqlType, targetAnt, err := gqlTypeFromNode(e.Type)
				if err != nil {
					return nil, err
				}
				if targetAnt.Skip.Is(SkipType) {
					return nil, fmt.Errorf("entgql: union %q on %s: edge %q targets %s which is skipped", u.Type, n.Name, e.Name, e.Type.Name)
				}
				members = append(members, gqlType)
			}
			for _, other := range g.Nodes {
				gqlType, _, err := gqlTypeFromNode(other)
				if err != nil {
					return nil, err
				}
				if gqlType == u.Type {
					return nil, fmt.Errorf("entgql: union %q on %s collides with type %s", u.Type, n.Name, other.Name)
				}
			}
			if existing, ok := defs[u.Type]; ok {
				if !sameSet(existing.Members, members) {
					return nil, fmt.Errorf("entgql: union %q declared with different members on %s: %v vs %v", u.Type, n.Name, existing.Members, members)
				}
				continue
			}
			defs[u.Type] = &unionDef{Type: u.Type, Members: members}
		}
	}
	return defs, nil
}

func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func stampUnionMembership(g *gen.Graph, defs map[string]*unionDef) error {
	for _, n := range g.Nodes {
		gqlType, ant, err := gqlTypeFromNode(n)
		if err != nil {
			return err
		}
		var memberOf []string
		for _, name := range sortedKeys(defs) {
			if slices.Contains(defs[name].Members, gqlType) {
				memberOf = append(memberOf, name)
			}
		}
		if len(memberOf) == 0 {
			continue
		}
		ant.UnionMemberOf = memberOf
		if n.Annotations == nil {
			n.Annotations = gen.Annotations{}
		}
		n.Annotations[ant.Name()] = *ant
	}
	return nil
}

func graphUnions(g *gen.Graph) ([]string, error) {
	defs, err := collectUnions(g)
	if err != nil {
		return nil, err
	}
	return sortedKeys(defs), nil
}

func sortedKeys(m map[string]*unionDef) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
