package entgql

import (
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"github.com/stretchr/testify/require"
)

func loadUnionGraph(t *testing.T) *gen.Graph {
	t.Helper()
	s, err := gen.NewStorage("sql")
	require.NoError(t, err)
	graph, err := entc.LoadGraph("./internal/uniontest/ent/schema", &gen.Config{
		Storage: s,
		Package: "entgo.io/contrib/entgql/internal/uniontest/ent",
	})
	require.NoError(t, err)
	return graph
}

func TestUnionFixtureLoads(t *testing.T) {
	graph := loadUnionGraph(t)
	require.Len(t, graph.Nodes, 3)
}

func postNode(t *testing.T, g *gen.Graph) *gen.Type {
	t.Helper()
	for _, n := range g.Nodes {
		if n.Name == "Post" {
			return n
		}
	}
	t.Fatal("no Post node")
	return nil
}

func withUnionAnnotation(t *testing.T, n *gen.Type, ant Annotation) {
	t.Helper()
	if n.Annotations == nil {
		n.Annotations = gen.Annotations{}
	}
	n.Annotations[ant.Name()] = ant
}

func TestCollectUnions(t *testing.T) {
	graph := loadUnionGraph(t)
	defs, err := collectUnions(graph)
	require.NoError(t, err)
	require.Equal(t, map[string]*unionDef{"Author": {Type: "Author", Members: []string{"Person", "Bot"}}}, defs)

	post := postNode(t, graph)
	members, err := unionMemberEdges(post, Annotation{}.Merge(UnionField("author", "Author", "author_person", "author_bot")).(Annotation).Unions[0])
	require.NoError(t, err)
	require.Equal(t, []string{"author_person", "author_bot"}, []string{members[0].Name, members[1].Name})

	rest, err := nonUnionEdges(post)
	require.NoError(t, err)
	require.Len(t, rest, 1)
	require.Equal(t, "reviewer", rest[0].Name)
}

func TestCollectUnionsValidation(t *testing.T) {
	cases := []struct {
		name string
		ant  Annotation
		want string
	}{
		{"unknown edge", UnionField("author", "Author", "author_person", "missing"), `edge "missing"`},
		{"single member", UnionField("author", "Author", "author_person"), "at least two"},
		{"type collision", UnionField("author", "Post", "author_person", "author_bot"), `collides`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			graph := loadUnionGraph(t)
			withUnionAnnotation(t, postNode(t, graph), tc.ant)
			_, err := collectUnions(graph)
			require.ErrorContains(t, err, tc.want)
		})
	}
	t.Run("member sets must agree", func(t *testing.T) {
		graph := loadUnionGraph(t)
		for _, n := range graph.Nodes {
			if n.Name == "Person" {
				n.Edges = append(n.Edges, &gen.Edge{Name: "extra", Type: postNode(t, graph), Unique: true}, &gen.Edge{Name: "extra2", Type: postNode(t, graph), Unique: true})
				withUnionAnnotation(t, n, UnionField("x", "Author", "extra", "extra2"))
			}
		}
		_, err := collectUnions(graph)
		require.ErrorContains(t, err, "different members")
	})
	t.Run("non-unique member", func(t *testing.T) {
		graph := loadUnionGraph(t)
		for _, e := range postNode(t, graph).Edges {
			if e.Name == "author_bot" {
				e.Unique = false
			}
		}
		_, err := collectUnions(graph)
		require.ErrorContains(t, err, "must be unique")
	})
}

func TestStampUnionMembership(t *testing.T) {
	graph := loadUnionGraph(t)
	defs, err := collectUnions(graph)
	require.NoError(t, err)
	require.NoError(t, stampUnionMembership(graph, defs))
	for _, n := range graph.Nodes {
		ant, err := annotation(n.Annotations)
		require.NoError(t, err)
		switch n.Name {
		case "Person", "Bot":
			require.Equal(t, []string{"Author"}, ant.UnionMemberOf)
			impls, err := nodeImplementors(n)
			require.NoError(t, err)
			require.Contains(t, impls, "Author")
		default:
			require.Empty(t, ant.UnionMemberOf)
		}
	}
}
