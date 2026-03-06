package dag

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s-hammon/p"
	"github.com/stretchr/testify/require"
)

func TestNewDAG_Validation(t *testing.T) {
	t.Run("Valid DAG", func(t *testing.T) {
		n1 := Node("n1", timesTwo)
		n2 := Node("n2", timesTwo, "n1")
		_, err := NewDag(n1, n2)
		require.NoError(t, err)
	})

	t.Run("Missing Dependency", func(t *testing.T) {
		n1 := Node("n1", timesTwo, "nil")
		_, err := NewDag(n1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "non-existent node")
	})

	t.Run("Simple Cycle", func(t *testing.T) {
		n1 := Node("A", timesTwo, "B")
		n2 := Node("B", timesTwo, "A")
		_, err := NewDag(n1, n2)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrCyclicalGraph)
	})
}

func TestDAG_Run_Linear(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	nA := Node("A", func(ctx context.Context, in <-chan any, out chan<- any) error {
		out <- 2
		return nil
	})
	nB := Node("B", timesTwo, "A")
	nC := Node("C", timesTwo, "B")
	nD := Node("D", timesTwo, "C")

	dag, err := NewDag(nA, nB, nC, nD)
	require.NoError(t, err)

	var got []int
	for v := range dag.Run(ctx) {
		got = append(got, v.(int))
	}

	require.Equal(t, []int{16}, got)
}

func TestDAG_Run_Diamond(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sumOp := func(ctx context.Context, in <-chan any, out chan<- any) error {
		sum := 0
		for v := range in {
			sum += v.(int)
		}
		out <- sum

		return nil
	}

	nA := Node("A", func(ctx context.Context, in <-chan any, out chan<- any) error {
		out <- 2
		return nil
	})
	nB := Node("B", timesTwo, "A")
	nC := Node("C", timesTwo, "A")
	nD := Node("D", sumOp, "B", "C")

	dag, err := NewDag(nA, nB, nC, nD)
	require.NoError(t, err)

	out := dag.Run(ctx)

	var results []int
	for v := range out {
		results = append(results, v.(int))
	}

	require.Equal(t, []int{8}, results)
}

func TestDAG_Stress(t *testing.T) {
	skipIfShort(t)
	t.Parallel()

	ctx := context.Background()
	numVals := 1000

	nA := Node("A", func(ctx context.Context, in <-chan any, out chan<- any) error {
		for i := range numVals {
			out <- i
		}

		return nil
	})
	nB := Node("B", timesTwo, "A")

	dag, _ := NewDag(nA, nB)

	out := dag.Run(ctx)
	count := 0
	for range out {
		count++
	}

	require.Equal(t, numVals, count)
}

func TestHasCycle(t *testing.T) {
	tests := []struct {
		name string
		adj  [][]int
		want bool
	}{
		{
			"empty graph",
			[][]int{},
			false,
		},
		{
			"single node",
			// 0: "a"
			[][]int{
				nil,
			},
			false,
		},
		{
			"self loop",
			// 0: "a"
			[][]int{
				{0},
			},
			true,
		},
		{
			"simple cycle",
			// 0: "a", 1: "b", 2: "c"
			[][]int{
				{1}, // a -> b
				{2}, // b -> c
				{0}, // c -> a
			},
			true,
		},
		{
			"DAG",
			// 0: "a", 1: "b", 2: "c", 3: "d"
			[][]int{
				{1, 2}, // a -> b, c
				{3},    // b -> d
				{3},    // c -> d
				nil,    // d -> (nothing)
			},
			false,
		},
		{
			"two graphs, one cycle",
			// 0: "a", 1: "b", 2: "x", 3: "y"
			[][]int{
				{1}, // a -> b
				nil, // b -> (nothing)
				{3}, // x -> y
				{2}, // y -> x
			},
			true,
		},
		{
			"all roots",
			// 0: "a", 1: "b", 2: "c"
			[][]int{
				nil,
				nil,
				nil,
			},
			false,
		},
		{
			"single path",
			// 0: "a", 1: "b", 2: "c", 3: "d", 4: "e"
			[][]int{
				{1}, // a -> b
				{2}, // b -> c
				{3}, // c -> d
				{4}, // d -> e
				nil, // e -> (nothing)
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasCycle(tt.adj)
			// Using require.Equal gives much better error output on failure
			// than require.True(t, got == tt.want)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDAG_Error(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("critical node failure")

	errNode := Node("error", func(ctx context.Context, in <-chan any, out chan<- any) error {
		return wantErr
	})

	lockedNode := Node("locked", func(ctx context.Context, in <-chan any, out chan<- any) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return errors.New("lockedNode did not receive context cancellation")
		}
	})

	d, err := NewDag(errNode, lockedNode)
	require.NoError(t, err)

	start := time.Now()

	for range d.Run(ctx) {
		t.Fatal("expected no output due to early error")
	}

	duration := time.Since(start)
	require.Less(t, duration, 1*time.Second, "DAG did not fail fast, cancellation likely leaked")
	require.Error(t, d.Error())
	require.Equal(t, wantErr, d.Error())
}

var timesTwo = func(ctx context.Context, in <-chan any, out chan<- any) error {
	for v := range in {
		out <- v.(int) * 2
	}

	return nil
}

func skipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
}

var passThrough = func(ctx context.Context, in <-chan any, out chan<- any) error {
	for v := range in {
		out <- v
	}

	return nil
}

func buildLinearDAG(b *testing.B, size int) *DAG {
	b.Helper()

	nodes := make([]*node, size)
	nodes[0] = Node("node_0", func(ctx context.Context, in <-chan any, out chan<- any) error {
		out <- struct{}{}
		return nil
	})

	for i := 1; i < size; i++ {
		nodes[i] = Node(p.Format("node_%d", i), passThrough, p.Format("node_%d", i-1))
	}

	d, err := NewDag(nodes...)
	require.NoError(b, err)
	return d
}

func buildWideDAG(b *testing.B, size int) *DAG {
	b.Helper()

	nodes := make([]*node, size)
	nodes[0] = Node("root", func(ctx context.Context, in <-chan any, out chan<- any) error {
		out <- struct{}{}
		return nil
	})

	for i := 1; i < size; i++ {
		nodes[i] = Node(p.Format("leaf_%d", i), passThrough, "root")
	}

	d, err := NewDag(nodes...)
	require.NoError(b, err)
	return d
}

func benchmarkLinear(b *testing.B, size int) {
	d := buildLinearDAG(b, size)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		for range d.Run(ctx) {
		}
	}
}

func BenchmarkDAG_Linear_10(b *testing.B)   { benchmarkLinear(b, 10) }
func BenchmarkDAG_Linear_100(b *testing.B)  { benchmarkLinear(b, 100) }
func BenchmarkDAG_Linear_1000(b *testing.B) { benchmarkLinear(b, 1000) }

func benchmarkWide(b *testing.B, size int) {
	d := buildWideDAG(b, size)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		for range d.Run(ctx) {
		}
	}
}

func BenchmarkDAG_Wide_10(b *testing.B)   { benchmarkWide(b, 10) }
func BenchmarkDAG_Wide_100(b *testing.B)  { benchmarkWide(b, 100) }
func BenchmarkDAG_Wide_1000(b *testing.B) { benchmarkWide(b, 1000) }

var taskOp = func(ctx context.Context, in <-chan any, out chan<- any) error {
	for range in {
	}

	out <- struct{}{}
	return nil
}

func buildMeshDAG(b *testing.B, layers, width int) *DAG {
	var nodes []*node

	for i := range width {
		name := p.Format("L0_N%d", i)
		nodes = append(nodes, Node(name, func(ctx context.Context, in <-chan any, out chan<- any) error {
			out <- struct{}{}
			return nil
		}))
	}

	for l := 1; l < layers; l++ {
		for i := range width {
			name := p.Format("L%d_N%d", l, i)

			dep1 := p.Format("L%d_N%d", l-1, i)
			dep2 := p.Format("L%d_N%d", l-1, (i+1)%width)
			dep3 := p.Format("L%d_N%d", l-1, (i+width-1)%width)

			if l > 1 {
				dep4 := p.Format("L%d_N%d", l-2, i)
				nodes = append(nodes, Node(name, taskOp, dep1, dep2, dep3, dep4))
			} else {
				nodes = append(nodes, Node(name, taskOp, dep1, dep2, dep3))
			}
		}
	}

	var sinkDeps []string
	for i := range width {
		sinkDeps = append(sinkDeps, p.Format("L%d_N%d", layers-1, i))
	}

	nodes = append(nodes, Node("sink", taskOp, sinkDeps...))

	d, err := NewDag(nodes...)
	require.NoError(b, err)
	return d
}

func benchmarkMesh(b *testing.B, layers, width int) {
	d := buildMeshDAG(b, layers, width)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		for range d.Run(ctx) {
		}
	}
}

func BenchmarkDAG_Mesh_10x10(b *testing.B)  { benchmarkMesh(b, 10, 10) }
func BenchmarkDAG_Mesh_20x50(b *testing.B)  { benchmarkMesh(b, 20, 50) }
func BenchmarkDAG_Mesh_100x10(b *testing.B) { benchmarkMesh(b, 100, 10) }
