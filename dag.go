package dag

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

const DefaultTimeoutSeconds = 10

// DAG is a directed acyclic graph, implying the following invariants:
// 1. edges are directed (one-way)
// 2. graph contains no directed cycles or closed loops (it starts and ends)
type DAG struct {
	// adjacency list of nodes to dependecies
	jobs jobs
	adj  map[string][]string
	out  chan any

	errCh chan error
	guard chan struct{}
}

func NewDAG(nodes []*node) (*DAG, error) {
	d := &DAG{}

	if err := d.handleNodes(nodes); err != nil {
		return nil, err
	}

	d.out = make(chan any)

	for _, n := range d.jobs {
		for _, dep := range n.deps {
			parent := d.jobs[dep]

			ch := make(chan any)

			parent.outs = append(parent.outs, ch)
			n.ins = append(n.ins, ch)
		}

		if n.isLeaf {
			n.outs = append(n.outs, d.out)
		}
	}

	d.errCh = make(chan error, 1)
	return d, nil
}

func (d *DAG) Run(ctx context.Context) <-chan any {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeoutSeconds*time.Second)

	d.guard = make(chan struct{}, 10)

	var wg sync.WaitGroup
	fn := func(j *node) func() {
		return func() {
			j.run(ctx, d.guard)
		}
	}

	for _, job := range d.jobs {
		wg.Go(fn(job))
	}

	go func() {
		select {
		case err := <-d.errCh:
			log.Fatalln("encountered error:", err)
			cancel()
		case <-ctx.Done():
		}
	}()

	go func() {
		wg.Wait()
		cancel()
	}()

	return d.out
}

// ensures that we have a valid configuration for the DAG
func (d *DAG) handleNodes(nodes []*node) error {
	d.jobs = make(jobs, len(nodes))
	d.adj = make(map[string][]string)

	for _, n := range nodes {
		if err := d.jobs.add(n); err != nil {
			return fmt.Errorf("couldn't add job %q: %v", n.name, err)
		}

		for _, dep := range n.deps {
			d.adj[dep] = append(d.adj[dep], n.name)
		}
	}

	for dep := range d.adj {
		if _, ok := d.jobs[dep]; !ok {
			return fmt.Errorf("couldn't find dependency %q", dep)
		}
	}

	for name, n := range d.jobs {
		if len(d.adj[name]) == 0 {
			n.isLeaf = true
		}
	}

	return cycleErr(d.jobs)
}

func merge(ctx context.Context, chs ...<-chan any) <-chan any {
	var wg sync.WaitGroup
	out := make(chan any)

	fn := func(ch <-chan any) func() {
		return func() {
			for {
				select {
				case <-ctx.Done():
					return
				case v, ok := <-ch:
					if !ok {
						return
					}

					select {
					case <-ctx.Done():
						return
					case out <- v:
					}
				}
			}
			// for v := range ch {
			// 	out <- v
			// }
		}
	}

	for _, ch := range chs {
		wg.Go(fn(ch))
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

func broadcast(in <-chan any, outs ...chan<- any) {
	for v := range in {
		for _, out := range outs {
			out <- v
		}
	}

	for _, out := range outs {
		close(out)
	}
}
