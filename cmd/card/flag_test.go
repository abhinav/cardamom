//go:build script

package main

import "flag"

func init() {
	if flag.Lookup("update") == nil {
		flag.Bool("update", false, "update golden files")
	}
}

// updateFlag reports whether testscript should replace mismatched cmp fixtures.
func updateFlag() bool {
	return flag.Lookup("update").Value.(flag.Getter).Get().(bool)
}
