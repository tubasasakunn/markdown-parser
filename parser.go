package markdownparser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

type engine struct {
	ctx          context.Context
	files        fs.FS
	markdown     goldmark.Markdown
	dependencies []string
	seen         map[string]bool
	diagnostics  []Diagnostic
	tasks        map[string]Task
}

// Parse reads entry and all referenced Markdown files from a virtual file
// system. Paths always use slash separators and cannot leave the FS root.
func Parse(ctx context.Context, files fs.FS, entry string) (Document, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	entry, err := cleanPath("", entry)
	if err != nil {
		return Document{}, err
	}
	e := &engine{
		ctx:         ctx,
		files:       files,
		markdown:    goldmark.New(goldmark.WithExtensions(extension.GFM)),
		seen:        make(map[string]bool),
		tasks:       make(map[string]Task),
		diagnostics: make([]Diagnostic, 0),
	}
	root, metadata, err := e.parseFile(entry, nil)
	if err != nil {
		return Document{}, err
	}
	if root == nil {
		return Document{}, errors.New("entry document is empty")
	}
	visible := make(map[string]bool)
	tasks := make([]Task, 0)
	var walk func(*Node)
	walk = func(node *Node) {
		if node.Kind == "task" && node.ID != "" && !visible[node.ID] {
			if task, ok := e.tasks[node.ID]; ok {
				visible[node.ID] = true
				tasks = append(tasks, task)
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return Document{
		SchemaVersion: SchemaVersion,
		Entry:         entry,
		Metadata:      metadata,
		Root:          root,
		Tasks:         tasks,
		Dependencies:  e.dependencies,
		Diagnostics:   e.diagnostics,
	}, nil
}

func (e *engine) parseFile(file string, stack []string) (*Node, map[string]any, error) {
	if err := e.ctx.Err(); err != nil {
		return nil, nil, err
	}
	source, err := fs.ReadFile(e.files, file)
	if err != nil {
		return nil, nil, err
	}
	if !e.seen[file] {
		e.seen[file] = true
		e.dependencies = append(e.dependencies, file)
	}
	masked, metadata, metadataDiagnostic := parseFrontMatter(source)
	hash := contentHash(source)
	currentStack := append(append([]string(nil), stack...), file)
	if metadataDiagnostic != "" {
		e.diagnostics = append(e.diagnostics, Diagnostic{
			Code: "front_matter_invalid", Message: metadataDiagnostic,
			Source: SourceRef{Path: file, Hash: hash, IncludeStack: currentStack},
		})
	}
	parsed := e.markdown.Parser().Parse(text.NewReader(masked))
	root := e.convertNode(parsed, file, source, masked, hash, currentStack)
	root.Source = SourceRef{Path: file, Start: 0, End: len(source), Hash: hash, IncludeStack: currentStack}
	root.Children = e.resolveNodes(root.Children, file, currentStack)
	return root, metadata, nil
}

func parseFrontMatter(source []byte) ([]byte, map[string]any, string) {
	if !strings.HasPrefix(string(source), "---\n") {
		return source, nil, ""
	}
	end := strings.Index(string(source[4:]), "\n---\n")
	if end < 0 {
		return source, nil, ""
	}
	end += 4
	var metadata map[string]any
	if err := yaml.Unmarshal(source[4:end], &metadata); err != nil {
		return source, nil, err.Error()
	}
	masked := append([]byte(nil), source...)
	for i := 0; i < end+5; i++ {
		if masked[i] != '\n' {
			masked[i] = ' '
		}
	}
	return masked, metadata, ""
}

func cleanPath(base, target string) (string, error) {
	if target == "" || strings.Contains(target, "\\") || strings.HasPrefix(target, "/") {
		return "", fmt.Errorf("invalid path %q", target)
	}
	cleaned := path.Clean(path.Join(base, target))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path %q leaves document root", target)
	}
	return cleaned, nil
}

func (e *engine) diagnostic(code, message string, source SourceRef) {
	e.diagnostics = append(e.diagnostics, Diagnostic{Code: code, Message: message, Source: source})
}

func (e *engine) resolveNodes(nodes []*Node, file string, stack []string) []*Node {
	var resolved []*Node
	for i := 0; i < len(nodes); i++ {
		node := nodes[i]
		if node.Kind != "paragraph" {
			resolved = append(resolved, node)
			continue
		}
		line := strings.TrimSpace(visibleText(node))
		if strings.HasPrefix(line, ":::callout ") || strings.HasPrefix(line, ":::summary") {
			if container, ok := e.inlineContainer(node, file, stack); ok {
				resolved = append(resolved, container)
				continue
			}
		}
		switch {
		case strings.HasPrefix(line, "::include "):
			attrs, err := parseAttributes(strings.TrimPrefix(line, "::include "))
			if err != nil || attrs["path"] == "" {
				e.diagnostic("include_invalid", "include requires a quoted path", node.Source)
				resolved = append(resolved, node)
				continue
			}
			target, err := cleanPath(path.Dir(file), attrs["path"])
			if err != nil {
				e.diagnostic("include_outside_root", err.Error(), node.Source)
				resolved = append(resolved, node)
				continue
			}
			cycle := false
			for _, member := range stack {
				if member == target {
					cycle = true
					break
				}
			}
			if cycle {
				e.diagnostic("include_cycle", "include cycle: "+strings.Join(append(stack, target), " -> "), node.Source)
				resolved = append(resolved, node)
				continue
			}
			included, _, err := e.parseFile(target, stack)
			if err != nil {
				e.diagnostic("include_missing", err.Error(), node.Source)
				resolved = append(resolved, node)
				continue
			}
			children := included.Children
			if section := attrs["section"]; section != "" {
				if section != "summary" {
					e.diagnostic("include_section_invalid", "include section must be summary", node.Source)
					resolved = append(resolved, node)
					continue
				}
				children = nil
				for _, child := range included.Children {
					if child.Kind == "summary" {
						children = child.Children
						break
					}
				}
				if children == nil {
					e.diagnostic("include_section_missing", "included file has no summary block", node.Source)
					children = []*Node{}
				}
			}
			resolved = append(resolved, &Node{Kind: "include", Attributes: map[string]string{"path": target}, Source: node.Source, Children: children})
		case strings.HasPrefix(line, "::tasks "):
			attrs, err := parseAttributes(strings.TrimPrefix(line, "::tasks "))
			if err != nil || attrs["from"] == "" {
				e.diagnostic("task_query_invalid", "tasks requires a quoted from pattern", node.Source)
				resolved = append(resolved, node)
				continue
			}
			resolved = append(resolved, e.queryTasks(attrs, node.Source, stack))
		case strings.HasPrefix(line, ":::callout ") || strings.HasPrefix(line, ":::summary"):
			kind, attrs, err := parseContainerAttributes(line)
			if err != nil {
				e.diagnostic(kind+"_invalid", err.Error(), node.Source)
				resolved = append(resolved, node)
				continue
			}
			end := -1
			for j := i + 1; j < len(nodes); j++ {
				if nodes[j].Kind == "paragraph" && strings.TrimSpace(visibleText(nodes[j])) == ":::" {
					end = j
					break
				}
			}
			if end < 0 {
				e.diagnostic(kind+"_unclosed", kind+" needs a closing :::", node.Source)
				resolved = append(resolved, node)
				continue
			}
			resolved = append(resolved, &Node{Kind: kind, Attributes: attrs, Source: node.Source, Children: e.resolveNodes(nodes[i+1:end], file, stack)})
			i = end
		default:
			resolved = append(resolved, node)
		}
	}
	return resolved
}

// Goldmark treats a compact ::: container as one paragraph. Reparse its body
// against a same-length masked source so every child keeps original offsets.
func (e *engine) inlineContainer(node *Node, file string, stack []string) (*Node, bool) {
	source, err := fs.ReadFile(e.files, file)
	if err != nil || node.Source.Start < 0 || node.Source.End > len(source) || node.Source.Start >= node.Source.End {
		return nil, false
	}
	span := source[node.Source.Start:node.Source.End]
	firstBreak := bytes.IndexByte(span, '\n')
	lastBreak := bytes.LastIndexByte(span, '\n')
	if firstBreak < 0 || lastBreak <= firstBreak {
		return nil, false
	}
	opener := strings.TrimSpace(string(span[:firstBreak]))
	closer := strings.TrimSpace(string(span[lastBreak+1:]))
	kind, attrs, attrErr := parseContainerAttributes(opener)
	if attrErr != nil || closer != ":::" {
		return nil, false
	}
	start := node.Source.Start + firstBreak + 1
	end := node.Source.Start + lastBreak + 1
	masked := append([]byte(nil), source...)
	for i := range masked {
		if (i < start || i >= end) && masked[i] != '\n' {
			masked[i] = ' '
		}
	}
	parsed := e.markdown.Parser().Parse(text.NewReader(masked))
	body := e.convertNode(parsed, file, source, masked, node.Source.Hash, stack)
	return &Node{Kind: kind, Attributes: attrs, Source: node.Source,
		Children: e.resolveNodes(body.Children, file, stack)}, true
}

func parseContainerAttributes(opener string) (string, map[string]string, error) {
	if opener == ":::summary" {
		return "summary", nil, nil
	}
	if strings.HasPrefix(opener, ":::callout ") {
		attrs, err := parseAttributes(strings.TrimPrefix(opener, ":::callout "))
		return "callout", attrs, err
	}
	return "container", nil, fmt.Errorf("unsupported container")
}

func visibleText(node *Node) string {
	if node.Kind == "raw_html" || node.Kind == "checkbox" {
		return ""
	}
	var result strings.Builder
	result.WriteString(node.Text)
	for _, child := range node.Children {
		result.WriteString(visibleText(child))
	}
	return result.String()
}

func (e *engine) queryTasks(attrs map[string]string, source SourceRef, stack []string) *Node {
	query := &Node{Kind: "task_query", Attributes: attrs, Source: source}
	pattern := attrs["from"]
	var paths []string
	_ = fs.WalkDir(e.files, ".", func(file string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && matchGlob(pattern, file) {
			paths = append(paths, file)
		}
		return nil
	})
	sort.Strings(paths)
	for _, file := range paths {
		inStack := false
		for _, member := range stack {
			if member == file {
				inStack = true
			}
		}
		if inStack {
			continue
		}
		doc, _, err := e.parseFile(file, stack)
		if err != nil {
			e.diagnostic("task_query_read_failed", err.Error(), source)
			continue
		}
		var visit func(*Node)
		visit = func(node *Node) {
			if node.Kind == "task" && taskMatches(node, attrs["where"]) {
				query.Children = append(query.Children, node)
			}
			for _, child := range node.Children {
				visit(child)
			}
		}
		visit(doc)
	}
	seen := make(map[string]bool)
	unique := query.Children[:0]
	for _, task := range query.Children {
		if !seen[task.ID] {
			seen[task.ID] = true
			unique = append(unique, task)
		}
	}
	query.Children = unique
	if field := attrs["sort"]; field != "" {
		sort.SliceStable(query.Children, func(i, j int) bool {
			return query.Children[i].Attributes[field] < query.Children[j].Attributes[field]
		})
	}
	return query
}

func taskMatches(node *Node, where string) bool {
	if where == "" {
		return true
	}
	for _, condition := range strings.Split(where, ",") {
		parts := strings.SplitN(strings.TrimSpace(condition), "=", 2)
		if len(parts) != 2 || node.Attributes[strings.TrimSpace(parts[0])] != strings.TrimSpace(parts[1]) {
			return false
		}
	}
	return true
}
