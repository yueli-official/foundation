package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yueli-official/foundation/go/abuse"
)

func main() {
	directory := flag.String("dir", "", "consumer migration directory")
	name := flag.String("name", "", "migration base name without .up.sql/.down.sql")
	version := flag.Int("version", abuse.PostgresSchemaVersion, "abuse schema version")
	flag.Parse()
	result, err := abuse.WritePostgresMigration(*directory, *name, *version)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(result.UpPath)
	fmt.Println(result.DownPath)
}
