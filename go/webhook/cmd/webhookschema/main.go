package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yueli-official/foundation/go/webhook"
)

func main() {
	directory := flag.String("dir", "", "migration output directory")
	name := flag.String("name", "webhook_v1", "migration base name")
	flag.Parse()
	result, err := webhook.WriteMigration(*directory, *name, webhook.CurrentSchemaVersion)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(result.UpPath)
	fmt.Println(result.DownPath)
}
