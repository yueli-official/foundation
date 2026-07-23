package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yueli-official/foundation/go/siteprofile"
)

func main() {
	directory := flag.String("directory", "", "consumer migration directory")
	name := flag.String("name", "", "migration base name")
	prefix := flag.String("prefix", siteprofile.DefaultPostgresPrefix, "PostgreSQL table prefix")
	flag.Parse()

	result, err := siteprofile.WritePostgresMigration(
		*directory,
		*name,
		siteprofile.CurrentPostgresSchemaVersion,
		*prefix,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(result.UpPath)
	fmt.Println(result.DownPath)
}
