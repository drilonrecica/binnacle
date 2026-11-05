// SPDX-License-Identifier: AGPL-3.0-only

// Command errdiscard fails when a package assigns an error to the blank
// identifier (RUN-7). `make go-vet` runs it over ./internal/... after go vet.
package main

import (
	"fmt"
	"os"

	"github.com/drilonrecica/binnacle/internal/lint"
)

func main() {
	patterns := os.Args[1:]
	if len(patterns) == 0 {
		patterns = []string{"./internal/..."}
	}
	if err := lint.Run(os.Stdout, ".", patterns...); err != nil {
		fmt.Fprintln(os.Stderr, "errdiscard:", err)
		os.Exit(1)
	}
}
