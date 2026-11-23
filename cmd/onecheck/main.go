// Command onecheck runs configured system checks.
package main

import (
	"os"

	"github.com/alekzonder/onecheck/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
