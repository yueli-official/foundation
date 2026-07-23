package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yueli-official/foundation/go/search"
)

func main() {
	directory := flag.String("dir", "", "migration directory")
	name := flag.String("name", "", "migration base name")
	flag.Parse()
	if err := search.WritePostgresMigration(*directory, *name); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
