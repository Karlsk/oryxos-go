package main

import (
	"fmt"
	"os"
)

func main() {
	if err := NewRootCommand(CommandDependencies{}).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
