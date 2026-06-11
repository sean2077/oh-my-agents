package main

import (
	"os"

	"github.com/sean2077/oh-my-agents/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
