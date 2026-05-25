package main

import "os"

func main() {
	os.Exit(Run(os.Stdout, os.Stderr, os.Args[1:]))
}
