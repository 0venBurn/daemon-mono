package main

import (
	"fmt"
	"os"
)

func main() {
	d, err := newDaemon(os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	d.serve(os.Stdin)
}
