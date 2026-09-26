package markdownparser

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

var taskMarkerPattern = regexp.MustCompile(`^\s*(?:[-+*]|[0-9]+[.)])\s+(\[[ xX]\])`)
var taskMetadataPattern = regexp.MustCompile(`<!--\s*md:task\s+([^>]*?)\s*-->`)

func (e *engine) convertNode(original ast.Node, file string, source, masked []byte, hash string, stack []string) *Node {
	node := &Node{Kind: kindFor(original), Source: nodeSource(original, file, hash, stack)}
	switch value := original.(type) {
	case *ast.Heading:
		node.Attributes = map[string]string{"level": strconv.Itoa(value.Level)}
	case *ast.List:
		node.Attributes = map[string]string{"ordered": strconv.FormatBool(value.IsOrdered())}
	case *ast.Text:
		node.Text = string(value.Segment.Value(masked))
	case *ast.String:
		node.Text = string(value.Value)
	case *ast.Link:
		node.Attributes = map[string]string{"destination": string(value.Destination)}
	case *ast.Image:
		node.Attributes = map[string]string{"destination": string(value.Destination)}
	case *ast.AutoLink:
		node.Text = string(value.Label(masked))
		node.Attributes = map[string]string{"destination": string(value.URL(masked))}
	case *ast.FencedCodeBlock:
		node.Text = string(value.Text(masked))
		node.Attributes = map[string]string{"language": string(value.Language(masked))}
	case *ast.CodeBlock:
		node.Text = string(value.Text(masked))
	case *ast.RawHTML, *ast.HTMLBlock:
		node.Text = string(original.Text(masked))
	case *east.TaskCheckBox:
		node.Attributes = map[string]string{"checked": strconv.FormatBool(value.IsChecked)}
	}
	for child := original.FirstChild(); child != nil; child = child.NextSibling() {
		node.Children = append(node.Children, e.convertNode(child, file, source, masked, hash, stack))
	}
	if node.Source.End <= node.Source.Start && len(node.Children) > 0 {
		start, end := node.Children[0].Source.Start, node.Children[0].Source.End
		for _, child := range node.Children[1:] {
			if child.Source.Start < start {
				start = child.Source.Start
			}
			if child.Source.End > end {
				end = child.Source.End
			}
		}
		node.Source.Start, node.Source.End = start, end
	}
	if node.Kind == "list_item" && containsCheckbox(node) {
		e.makeTask(node, file, source, hash, stack)
	}
	return node
}

func kindFor(node ast.Node) string {
	switch value := node.(type) {
	case *ast.Document:
		return "document"
	case *ast.Heading:
		return "heading"
	case *ast.Paragraph:
		return "paragraph"
	case *ast.Text, *ast.String:
		return "text"
	case *ast.Emphasis:
		if value.Level == 2 {
			return "strong"
		}
		return "emphasis"
	case *ast.Link, *ast.AutoLink:
		return "link"
	case *ast.Image:
		return "image"
	case *ast.CodeSpan:
		return "code_span"
	case *ast.CodeBlock, *ast.FencedCodeBlock:
		return "code_block"
	case *ast.List:
		return "list"
	case *ast.ListItem:
		return "list_item"
	case *ast.Blockquote:
		return "blockquote"
	case *ast.ThematicBreak:
		return "thematic_break"
	case *ast.RawHTML, *ast.HTMLBlock:
		return "raw_html"
	case *east.TaskCheckBox:
		return "checkbox"
	case *east.Table:
		return "table"
	case *east.TableHeader:
		return "table_header"
	case *east.TableRow:
		return "table_row"
	case *east.TableCell:
		return "table_cell"
	default:
		return "unknown"
	}
}

func nodeSource(node ast.Node, file, hash string, stack []string) SourceRef {
	reference := SourceRef{Path: file, Hash: hash, IncludeStack: append([]string(nil), stack...)}
	if node.Pos() >= 0 {
		reference.Start = node.Pos()
		reference.End = node.Pos()
	}
	if node.Type() == ast.TypeBlock {
		if lines := node.Lines(); lines != nil && lines.Len() > 0 {
			reference.Start = lines.At(0).Start
			reference.End = lines.At(lines.Len() - 1).Stop
		}
	}
	if value, ok := node.(*ast.Text); ok {
		reference.Start = value.Segment.Start
		reference.End = value.Segment.Stop
	}
	return reference
}

func containsCheckbox(node *Node) bool {
	if node.Kind == "checkbox" {
		return true
	}
	for _, child := range node.Children {
		if containsCheckbox(child) {
			return true
		}
	}
	return false
}

func checkboxChecked(node *Node) bool {
	if node.Kind == "checkbox" {
		return node.Attributes["checked"] == "true"
	}
	for _, child := range node.Children {
		if containsCheckbox(child) {
			return checkboxChecked(child)
		}
	}
	return false
}

func (e *engine) makeTask(node *Node, file string, source []byte, hash string, stack []string) {
	position := node.Source.Start
	if position < 0 || position > len(source) {
		return
	}
	lineStart := bytes.LastIndexByte(source[:position], '\n') + 1
	lineEnd := len(source)
	if newline := bytes.IndexByte(source[lineStart:], '\n'); newline >= 0 {
		lineEnd = lineStart + newline
	}
	line := source[lineStart:lineEnd]
	match := taskMarkerPattern.FindSubmatchIndex(line)
	if len(match) < 4 {
		return
	}
	markerStart := lineStart + match[2]
	attrs := make(map[string]string)
	if metadata := taskMetadataPattern.FindSubmatch(line); len(metadata) > 1 {
		if parsed, err := parseAttributes(string(metadata[1])); err == nil {
			attrs = parsed
		}
	}
	checked := checkboxChecked(node)
	attrs["checked"] = strconv.FormatBool(checked)
	id := attrs["id"]
	if id == "" {
		id = fmt.Sprintf("%s:%d", file, markerStart)
	}
	key := file + "#" + id
	attrs["id"] = id
	node.Kind = "task"
	node.ID = key
	node.Text = strings.TrimSpace(visibleText(node))
	node.Attributes = attrs
	node.Source = SourceRef{Path: file, Start: markerStart, End: lineEnd, Hash: hash, IncludeStack: append([]string(nil), stack...)}
	e.tasks[key] = Task{ID: id, Text: node.Text, Checked: checked, Attributes: attrs,
		Source: node.Source, MarkerStart: markerStart}
}
