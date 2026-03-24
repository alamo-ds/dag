# dag

`dag` is a Go library for creating and executing Directed Acyclic Graphs (DAGs). It allows you to define a set of nodes with operations and dependencies, and then execute them concurrently. Data is passed between nodes using channels, and the library handles the synchronization and execution order based on the defined dependencies.

## Features

- **Simple API**: Define nodes with operations and dependencies easily.
- **Concurrent Execution**: Nodes are executed concurrently using goroutines.
- **Cycle Detection**: Automatically detects cyclical dependencies and returns an error.
- **Data Flow**: Pass data between nodes using channels.
- **Context Support**: Supports `context.Context` for cancellation and timeouts.
- **Error Handling**: Propagates errors from node operations and cancels the execution of the DAG.

## Installation

```bash
go get github.com/alamo-ds/dag
```

## Usage

Here is a simple example of how to use the `dag` library:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/alamo-ds/dag"
)

func main() {
	ctx := context.Background()

	// Define a node that generates some data
	nodeA := dag.Node("A", func(ctx context.Context, in <-chan any, out chan<- any) error {
		out <- 2
		out <- 3
		return nil
	})

	// Define a node that multiplies the input by 2 and depends on node A
	nodeB := dag.Node("B", func(ctx context.Context, in <-chan any, out chan<- any) error {
		for v := range in {
			out <- v.(int) * 2
		}
		return nil
	}, "A")

	// Define a node that adds 10 to the input and depends on node B
	nodeC := dag.Node("C", func(ctx context.Context, in <-chan any, out chan<- any) error {
		for v := range in {
			out <- v.(int) + 10
		}
		return nil
	}, "B")

	// Create the DAG
	d, err := dag.NewDag(nodeA, nodeB, nodeC)
	if err != nil {
		log.Fatalf("Failed to create DAG: %v", err)
	}

	// Run the DAG and collect the results
	for result := range d.Run(ctx) {
		fmt.Printf("Result: %v\n", result)
	}

	// Check for any errors that occurred during execution
	if err := d.Error(); err != nil {
		log.Fatalf("DAG execution failed: %v", err)
	}
}
```

### Output

```
Result: 14
Result: 16
```

## API

### `Node(name string, fn op, dependencies ...string) *node`

Creates a new node with the given name, operation function, and a list of dependency node names.

The operation function has the signature:

```go
type op func(ctx context.Context, in <-chan any, out chan<- any) error
```

- `in`: A channel to receive data from dependency nodes.
- `out`: A channel to send data to dependent nodes.

### `NewDag(nodes ...*node) (*DAG, error)`

Creates a new DAG from the given nodes. It validates the dependencies and checks for cycles. Returns an error if a dependency is missing or if a cycle is detected.

### `(*DAG) Run(ctx context.Context) <-chan any`

Executes the DAG concurrently. It returns a channel that receives the output from the leaf nodes (nodes with no dependents). The execution can be canceled using the provided context.

### `(*DAG) Error() error`

Returns any error that occurred during the execution of the DAG.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
