package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yueli-official/foundation/go/traffic/postgres"
)

func main() {
	directory := flag.String("dir", "", "consumer migration directory")
	name := flag.String("name", "", "migration base name without .up.sql/.down.sql")
	version := flag.Uint("version", postgres.CurrentSchemaVersion, "traffic schema version")
	flag.Parse()

	result, err := postgres.WriteMigration(*directory, *name, *version)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(result.UpPath)
	fmt.Println(result.DownPath)
}
