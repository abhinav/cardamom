package main

import "os"

func main() {
	os.Exit(Run(DefaultEnv(), os.Args[1:]))
}
