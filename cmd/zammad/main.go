package main

import (
	"fmt"
	"os"

	"github.com/lukeisontheroad/zammad-cli/internal/cmd"
)

// Set via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	if err := cmd.Execute(version); err != nil {
		fmt.Fprintln(os.Stderr, "zammad:", err)
		os.Exit(1)
	}
}
