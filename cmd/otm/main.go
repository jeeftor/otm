package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jeeftor/otm/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "otm: %v\n", err)
		os.Exit(1)
	}
}
