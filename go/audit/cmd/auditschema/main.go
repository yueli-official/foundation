package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yueli-official/foundation/go/audit"
)

func main() {
	directory := flag.String("dir", "", "consumer migration directory")
	name := flag.String("name", "", "migration base name without .up.sql/.down.sql")
	version := flag.Int("version", audit.PostgresSchemaVersion, "audit schema version")
	flag.Parse()

	result, err := audit.WritePostgresMigration(*directory, *name, *version)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(result.UpPath)
	fmt.Println(result.DownPath)
}
