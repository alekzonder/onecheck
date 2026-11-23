// Command onecheck runs configured system checks.
package main

import (
	"fmt"

	"github.com/alekzonder/onecheck/internal/version"
)

func main() {
	fmt.Println("onecheck", version.Version)
}
