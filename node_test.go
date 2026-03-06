package dag

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNode_Run(t *testing.T) {
	ctx := context.Background()

	in := make(chan any, 1)
	out := make(chan any, 1)

	in <- 23
	close(in)

	n := &node{
		name: "test",
		op: func(ctx context.Context, in <-chan any, out chan<- any) {
			for v := range in {
				out <- v.(int) * 3
			}
		},
		ins:  []<-chan any{in},
		outs: []chan<- any{out},
	}

	guard := make(chan struct{}, 1)

	go n.run(ctx, guard)

	require.Equal(t, 69, <-out)
}

func TestNode_Broadcast(t *testing.T) {
	ctx := context.Background()

	in := make(chan any, 1)
	in <- 23
	close(in)

	out1 := make(chan any, 1)
	out2 := make(chan any, 1)

	n := &node{
		name: "test",
		op: func(ctx context.Context, in <-chan any, out chan<- any) {
			for v := range in {
				out <- v.(int) * 3
			}
		},
		ins:  []<-chan any{in},
		outs: []chan<- any{out1, out2},
	}

	guard := make(chan struct{}, 1)

	go n.run(ctx, guard)

	v1 := <-out1
	v2 := <-out2

	require.Equal(t, []int{69, 69}, []int{v1.(int), v2.(int)})
}

func TestNode_Guard(t *testing.T) {
	ctx := context.Background()

	block := make(chan struct{})

	n := &node{
		name: "test",
		op: func(ctx context.Context, in <-chan any, out chan<- any) {
			<-block
		},
	}

	guard := make(chan struct{}, 1)

	go n.run(ctx, guard)

	done := make(chan struct{})

	go func() {
		n.run(ctx, guard)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("run should have blocked")
	case <-time.After(50 * time.Millisecond):
	}

	close(block)
}

func TestJobs_Add(t *testing.T) {
	j := make(jobs)

	a := &node{name: "test"}
	require.NoError(t, j.add(a))
	require.Error(t, j.add(a))
	require.Error(t, j.add(&node{}))
}

func TestHasCycle(t *testing.T) {
	tests := []struct {
		name string
		j    jobs
		want bool
	}{
		{
			"single node DAG",
			jobs{
				"a": &node{name: "a"},
			},
			false,
		},
		{
			"self loop",
			jobs{
				"a": &node{name: "a", deps: []string{"a"}},
			},
			true,
		},
		{
			"single thread DAG",
			jobs{
				"a": &node{name: "a"},
				"b": &node{name: "b", deps: []string{"a"}},
				"c": &node{name: "c", deps: []string{"b"}},
			},
			false,
		},
		{
			"branching thread DAG",
			jobs{
				"a": &node{name: "a"},
				"b": &node{name: "b", deps: []string{"a"}},
				"c": &node{name: "c", deps: []string{"a"}},
			},
			false,
		},
		{
			"single thread cycle",
			jobs{
				"a": &node{name: "a", deps: []string{"c"}},
				"b": &node{name: "b", deps: []string{"a"}},
				"c": &node{name: "c", deps: []string{"b"}},
			},
			true,
		},
		{
			"converging threads DAG",
			jobs{
				"a": &node{name: "a"},
				"b": &node{name: "b"},
				"c": &node{name: "c", deps: []string{"a", "b"}},
			},
			false,
		},
		{
			"two cycles",
			jobs{
				"a": &node{name: "a", deps: []string{"c"}},
				"b": &node{name: "b", deps: []string{"a"}},
				"c": &node{name: "c", deps: []string{"b"}},
				"d": &node{name: "d", deps: []string{"f"}},
				"e": &node{name: "e", deps: []string{"d"}},
				"f": &node{name: "f", deps: []string{"e"}},
			},
			true,
		},
	}

	for _, tt := range tests {
		require.Equal(t, hasCycle(tt.j), tt.want)
	}
}
