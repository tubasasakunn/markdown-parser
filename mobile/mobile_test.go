package mobile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	markdownparser "github.com/tubasasakunn/markdown-parser"
)

func TestParseDirectoryAndEditIncludedTask(t *testing.T) {
	root := filepath.Join("..", "testdata", "include")
	value, err := ParseDirectory(root, "index.md")
	if err != nil {
		t.Fatal(err)
	}
	var doc markdownparser.Document
	if err := json.Unmarshal([]byte(value), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Tasks) != 1 || doc.Tasks[0].Source.Path != "notes/child.md" {
		t.Fatalf("unexpected expanded task: %#v", doc.Tasks)
	}
	source, err := os.ReadFile(filepath.Join(root, doc.Tasks[0].Source.Path))
	if err != nil {
		t.Fatal(err)
	}
	taskJSON, _ := json.Marshal(doc.Tasks[0])
	updated, err := SetTaskChecked(string(source), string(taskJSON), true)
	if err != nil || updated == string(source) {
		t.Fatalf("task edit failed: %v", err)
	}
}

func TestParseDirectoryConfinesSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private.md")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseDirectory(root, "escape.md"); err == nil {
		t.Fatal("symlink escaped the document root")
	}
}
