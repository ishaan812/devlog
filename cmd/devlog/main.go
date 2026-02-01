package main

import (
	"os"

	"github.com/ishaan812/devlog/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
