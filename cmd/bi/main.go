// Command bi is the office-document conversion HTTP service.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		runServe(os.Args[1:])
		return
	}
	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "healthcheck":
		runHealthcheck(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %q\n", os.Args[1])
		os.Exit(2)
	}
}
