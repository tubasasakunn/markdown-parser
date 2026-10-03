package markdownparser

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func fixture(t *testing.T, group, entry string) Document {
	t.Helper()
	doc, err := Parse(context.Background(), os.DirFS(filepath.Join("testdata", group)), entry)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func nodesOfKind(node *Node, kind string) []*Node {
	if node == nil {
		return nil
	}
	var found []*Node
	if node.Kind == kind {
		found = append(found, node)
	}
	for _, child := range node.Children {
		found = append(found, nodesOfKind(child, kind)...)
	}
	return found
}

func hasDiagnostic(doc Document, code string) bool {
	for _, diagnostic := range doc.Diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func TestBasicGFMAndCodeFence(t *testing.T) {
	doc := fixture(t, "basic", "index.md")
	for _, kind := range []string{"heading", "emphasis", "strong", "link", "code_block", "table"} {
		if len(nodesOfKind(doc.Root, kind)) == 0 {
			t.Errorf("missing %s node", kind)
		}
	}
	if len(doc.Tasks) != 2 || doc.Tasks[0].Checked || !doc.Tasks[1].Checked {
		t.Fatalf("unexpected tasks: %#v", doc.Tasks)
	}
	if len(doc.Dependencies) != 1 || doc.Dependencies[0] != "index.md" {
		t.Fatalf("code fence was interpreted as include: %#v", doc.Dependencies)
	}
	if len(doc.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", doc.Diagnostics)
	}
}

func TestNestedIncludeKeepsSource(t *testing.T) {
	doc := fixture(t, "include", "index.md")
	if len(nodesOfKind(doc.Root, "include")) != 2 {
		t.Fatalf("expected two include nodes")
	}
	if strings.Join(doc.Dependencies, ",") != "index.md,notes/child.md,notes/grand.md" {
		t.Fatalf("unexpected dependencies: %#v", doc.Dependencies)
	}
	if len(doc.Tasks) != 1 || doc.Tasks[0].Source.Path != "notes/child.md" || doc.Tasks[0].ID != "child-task" {
		t.Fatalf("lost task source: %#v", doc.Tasks)
	}
	grandchild := nodesOfKind(doc.Root, "heading")
	if len(grandchild) != 3 || grandchild[2].Source.Path != "notes/grand.md" {
		t.Fatalf("lost grandchild source: %#v", grandchild)
	}
}

func TestIncludeSummarySectionSelectsOnlySummaryContents(t *testing.T) {
	files := fstest.MapFS{
		"index.md": {Data: []byte("::include path=\"notes.md\" section=\"summary\"\n")},
		"notes.md": {Data: []byte(":::summary\nShort **summary**.\n:::\n\n# Full notes\nLong details stay in the source.\n")},
	}
	doc, err := Parse(context.Background(), files, "index.md")
	if err != nil {
		t.Fatal(err)
	}
	included := nodesOfKind(doc.Root, "include")
	if len(included) != 1 || len(included[0].Children) != 1 {
		t.Fatalf("unexpected summary include: %#v", included)
	}
	if got := visibleText(included[0].Children[0]); got != "Short summary." {
		t.Fatalf("included the wrong summary content: %q", got)
	}
	if len(nodesOfKind(included[0], "heading")) != 0 || strings.Contains(visibleText(included[0]), "Long details") {
		t.Fatalf("summary include leaked the rest of the document: %#v", included[0].Children)
	}
	if len(doc.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", doc.Diagnostics)
	}
}

func TestIncludeSummarySectionWithoutSummaryReportsDiagnostic(t *testing.T) {
	files := fstest.MapFS{
		"index.md": {Data: []byte("::include path=\"notes.md\" section=\"summary\"\n")},
		"notes.md": {Data: []byte("Just the full note.\n")},
	}
	doc, err := Parse(context.Background(), files, "index.md")
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiagnostic(doc, "include_section_missing") {
		t.Fatalf("missing summary diagnostic: %#v", doc.Diagnostics)
	}
}

func TestIncludeCycleAndInvalidPathsAreDiagnostics(t *testing.T) {
	cycle := fixture(t, "cycle", "a.md")
	if !hasDiagnostic(cycle, "include_cycle") {
		t.Fatalf("missing cycle diagnostic: %#v", cycle.Diagnostics)
	}
	errors := fixture(t, "errors", "index.md")
	for _, code := range []string{"include_missing", "include_outside_root"} {
		if !hasDiagnostic(errors, code) {
			t.Errorf("missing %s: %#v", code, errors.Diagnostics)
		}
	}
}

func TestTaskQueryFiltersSortsAndDeduplicates(t *testing.T) {
	doc := fixture(t, "tasks", "WIDGET_TODO.md")
	queries := nodesOfKind(doc.Root, "task_query")
	if len(queries) != 1 || len(queries[0].Children) != 2 {
		t.Fatalf("unexpected task query: %#v", queries)
	}
	if queries[0].Children[0].Attributes["id"] != "invoice" || queries[0].Children[1].Attributes["id"] != "rice" {
		t.Fatalf("query did not filter and sort tasks: %#v", queries[0].Children)
	}
	if len(doc.Tasks) != 3 { // invoice appears in both include and query, but is one source task.
		t.Fatalf("unexpected visible task index: %#v", doc.Tasks)
	}
	if queries[0].Children[1].Source.Path != "tasks/home.md" {
		t.Fatalf("query lost source: %#v", queries[0].Children[1].Source)
	}
}

func TestFrontMatterCalloutAndJSONContract(t *testing.T) {
	doc := fixture(t, "memo", "WIDGET_MEMO.md")
	if doc.Metadata["title"] != "Today" || doc.Metadata["status"] != "active" {
		t.Fatalf("front matter lost: %#v", doc.Metadata)
	}
	callouts := nodesOfKind(doc.Root, "callout")
	if len(callouts) != 1 || callouts[0].Attributes["kind"] != "note" {
		t.Fatalf("callout lost: %#v", callouts)
	}
	if len(doc.Dependencies) != 2 {
		t.Fatalf("memo include lost: %#v", doc.Dependencies)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"schemaVersion":1`) || !strings.Contains(string(data), `"notes/meeting.md"`) {
		t.Fatalf("JSON bridge contract lost: %s", data)
	}
}

func TestJSONUsesEmptyArraysForSwiftDecoder(t *testing.T) {
	doc := fixture(t, "memo", "WIDGET_MEMO.md")
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"diagnostics":[]`) || !strings.Contains(string(data), `"tasks":[]`) {
		t.Fatalf("JSON array contract lost: %s", data)
	}
}

func TestSetTaskCheckedEditsOriginalSourceAndRejectsStaleData(t *testing.T) {
	doc := fixture(t, "include", "index.md")
	path := filepath.Join("testdata", "include", "notes", "child.md")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := SetTaskChecked(source, doc.Tasks[0], true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "- [x] Task from child") {
		t.Fatalf("wrong edit: %s", updated)
	}
	if _, err := SetTaskChecked(append([]byte("new line\n"), source...), doc.Tasks[0], true); err == nil {
		t.Fatal("stale task edit was accepted")
	}
}

func TestSetTaskStatusPreservesMetadataAndChecksFinalStatus(t *testing.T) {
	source := []byte("# Board\n- [ ] 企画を書く <!-- md:task id=\"plan\" due=\"2026-10-02\" status=\"todo\" -->\n")
	task := Task{MarkerStart: len("# Board\n- "), Source: SourceRef{Hash: contentHash(source)}}
	updated, err := SetTaskStatus(source, task, "doing", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), `id="plan" due="2026-10-02" status="doing"`) {
		t.Fatalf("metadata changed unexpectedly: %s", updated)
	}
	if !strings.Contains(string(updated), "- [ ] 企画を書く") {
		t.Fatalf("marker changed: %s", updated)
	}
	if _, err := SetTaskStatus(source, task, "bad status", false); err == nil {
		t.Fatal("invalid status accepted")
	}
	if _, err := SetTaskStatus(append([]byte("new\n"), source...), task, "doing", false); err == nil {
		t.Fatal("stale edit accepted")
	}
	parsedTask := Task{MarkerStart: task.MarkerStart, Source: SourceRef{Hash: contentHash(updated)}}
	completed, err := SetTaskStatus(updated, parsedTask, "done", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(completed), "- [x] 企画を書く") || !strings.Contains(string(completed), `status="done"`) {
		t.Fatalf("completion not applied: %s", completed)
	}
}

func TestSetTaskStatusAddsMetadataWhenMissing(t *testing.T) {
	source := []byte("- [ ] New task\n")
	task := Task{MarkerStart: 2, Source: SourceRef{Hash: contentHash(source)}}
	updated, err := SetTaskStatus(source, task, "doing", false)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated) != "- [ ] New task <!-- md:task status=\"doing\" -->\n" {
		t.Fatalf("unexpected edit: %s", updated)
	}
}
