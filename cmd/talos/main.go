// SPDX-License-Identifier: AGPL-3.0-only

// Command talos starts the TALOS monitoring service.
package main

import "fmt"

var version = "dev"

func main() {
	fmt.Printf("talos %s\n", version)
}
