package dag

import (
	"context"
)

type op func(ctx context.Context, in <-chan any, out chan<- any) error

type node struct {
	name string
	op   op
	deps []string
}

func Node(name string, fn op, dependencies ...string) *node {
	return &node{
		name: name,
		op:   fn,
		deps: dependencies,
	}
}
