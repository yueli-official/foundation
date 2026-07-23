// Command authzschema materializes a versioned authorization migration into a
// consumer repository.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yueli-official/foundation/go/authorization/postgres"
)

func main() {
	directory := flag.String("dir", "", "consumer migration directory")
	name := flag.String("name", "", "migration base name without .up.sql/.down.sql")
	version := flag.Uint("version", postgres.CurrentSchemaVersion, "authorization schema version")
	flag.Parse()
	written, err := postgres.WriteMigration(*directory, *name, *version)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(written.UpPath)
	fmt.Println(written.DownPath)
}
