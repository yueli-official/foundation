package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yueli-official/foundation/go/privacy"
)

func main() {
	directory := flag.String("dir", "", "migration output directory")
	name := flag.String("name", "privacy_v1", "migration base name")
	flag.Parse()
	result, err := privacy.WriteMigration(*directory, *name, privacy.CurrentSchemaVersion)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(result.UpPath)
	fmt.Println(result.DownPath)
}
