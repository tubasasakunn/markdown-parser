package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	markdownparser "github.com/tubasasakunn/markdown-parser"
)

func main() {
	root := flag.String("root", ".", "directory containing Markdown files")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: mdparse [-root directory] entry.md")
		os.Exit(2)
	}
	files, err := os.OpenRoot(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer files.Close()
	doc, err := markdownparser.Parse(context.Background(), files.FS(), flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
