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
