package dag

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var (
	ErrCyclicalGraph = errors.New("cyclical dependency detected")
)

type DAG struct {
	nodes   []*node
	adj     [][]int
	isLeaf  []bool
	deps    []uint32
	nodeMap map[string]int
	err     error
}

func NewDag(nodes ...*node) (*DAG, error) {
	n := len(nodes)
	d := &DAG{
		nodes:   nodes,
		adj:     make([][]int, n),
		isLeaf:  make([]bool, n),
		deps:    make([]uint32, n),
		nodeMap: make(map[string]int, n),
	}

	for i, node := range nodes {
		d.nodeMap[node.name] = i
	}

	for i, node := range nodes {
		d.deps[i] = uint32(len(node.deps))
		for _, dep := range node.deps {
			idx, ok := d.nodeMap[dep]
			if !ok {
				return nil, fmt.Errorf("node %s depends on non-existent node %s", node.name, dep)
			}

			d.adj[idx] = append(d.adj[idx], i)
		}
	}

	if hasCycle(d.adj) {
		return nil, ErrCyclicalGraph
	}

	for i := range nodes {
		if len(d.adj[i]) == 0 {
			d.isLeaf[i] = true
		}
	}

	return d, nil
}

func hasCycle(adj [][]int) bool {
	visited := make([]bool, len(adj))
	stack := make([]bool, len(adj))

	var dfs func(int) bool
	dfs = func(nodeIdx int) bool {
		visited[nodeIdx] = true
		stack[nodeIdx] = true

		for _, childIdx := range adj[nodeIdx] {
			if !visited[childIdx] {
				if dfs(childIdx) {
					return true
				}
			} else if stack[childIdx] {
				return true // Back-edge detected!
			}
		}

		stack[nodeIdx] = false
		return false
	}

	for i := range adj {
		if !visited[i] {
			if dfs(i) {
				return true
			}
		}
	}

	return false
}

const DefaultMaxProcs = 4

type executionState struct {
	in      chan any
	pending atomic.Uint32
}

// Run returns the out channel of the graph, through which all output
// data in the graph is sent. Each node is wrapped in a job with an
// in channel, and executed in a goroutine.
func (d *DAG) Run(ctx context.Context) <-chan any {
	ctx, cancel := context.WithCancelCause(ctx)
	states := make([]executionState, len(d.nodes))

	for i := range d.nodes {
		states[i].in = make(chan any)
		states[i].pending.Store(d.deps[i])
	}

	var (
		wg    sync.WaitGroup
		resCh = make(chan any)
	)

	// main execution loop
	for i := range d.nodes {
		wg.Go(func() {
			mux := make(chan any)
			done := make(chan struct{})
			isLeaf := d.isLeaf[i]

			go func() {
				defer close(done)

				for val := range mux {
					if isLeaf {
						select {
						case resCh <- val:
						case <-ctx.Done():
							return
						}
					} else {
						for _, childIdx := range d.adj[i] {
							select {
							case states[childIdx].in <- val:
							case <-ctx.Done():
								return
							}
						}
					}
				}

				// notify children of completion. If for any child this
				// is the final parent, close its in channel.
				for _, childIdx := range d.adj[i] {
					if states[childIdx].pending.Add(^uint32(0)) == 0 {
						close(states[childIdx].in)
					}
				}
			}()

			if err := d.nodes[i].op(ctx, states[i].in, mux); err != nil {
				cancel(err)
			}

			close(mux)
			<-done
		})
	}

	go func(cancelCtx context.CancelCauseFunc) {
		defer cancelCtx(nil)
		wg.Wait()

		if cause := context.Cause(ctx); cause != nil && cause != context.Canceled {
			d.err = cause
		}

		close(resCh)
	}(cancel)

	return resCh
}

func (d *DAG) Error() error {
	return d.err
}
