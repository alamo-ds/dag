package dag

import (
	"context"
	"errors"
	"slices"
)

// An op is a function which takes input from one channel and sends
// output to another channel.
type op func(ctx context.Context, in <-chan any, out chan<- any)

type node struct {
	name   string
	op     op
	ins    []<-chan any
	outs   []chan<- any
	deps   []string
	isLeaf bool
}

// Create a Node with a name, op, and dependencies (if any).
func Node(name string, fn op, dependencies ...string) *node {
	return &node{
		name: name,
		op:   fn,
		deps: dependencies,
	}
}

func (j *node) run(ctx context.Context, guard chan struct{}) {
	guard <- struct{}{}
	defer func() { <-guard }()

	in := merge(ctx, j.ins...)
	out := make(chan any)

	go broadcast(out, j.outs...)

	j.op(ctx, in, out)
	close(out)
}

type jobs map[string]*node

func (j jobs) add(n *node) error {
	if n.name == "" {
		return errors.New("node has empty name")
	}

	if _, ok := j[n.name]; ok {
		return errors.New("found duplicate node")
	}

	j[n.name] = n
	return nil
}

var cycleErr = func(nodes map[string]*node) error {
	if hasCycle(nodes) {
		return errors.New("cyclical graph detected")
	}

	return nil
}

// since each node has a list of dependencies, we just simply need to iterate
// over each node in DAG.jobs ;and recursively call dfs on its dependencies
// until we reach a root node.
func hasCycle(nodes map[string]*node) bool {
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var dfs func(string) bool
	dfs = func(name string) bool {
		if inStack[name] {
			return true
		}
		if visited[name] {
			return false
		}

		visited[name] = true
		inStack[name] = true

		if slices.ContainsFunc(nodes[name].deps, dfs) {
			return true
		}

		inStack[name] = false
		return false
	}

	for name := range nodes {
		if !visited[name] {
			if dfs(name) {
				return true
			}
		}
	}

	return false
}
