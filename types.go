// Package markdownparser parses Markdown into a source-aware document tree.
// The tree is independent of a particular HTML or native renderer.
package markdownparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
)

const SchemaVersion = 1

// SourceRef identifies the original bytes that produced a rendered node.
// Start and End are UTF-8 byte offsets, with End exclusive.
type SourceRef struct {
	Path         string   `json:"path"`
	Start        int      `json:"start"`
	End          int      `json:"end"`
	Hash         string   `json:"hash"`
	IncludeStack []string `json:"includeStack,omitempty"`
}

// Node is a renderer-neutral document node. An include node retains its own
// source location while its children retain the locations of the included file.
type Node struct {
	Kind       string            `json:"kind"`
	ID         string            `json:"id,omitempty"`
	Text       string            `json:"text,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Source     SourceRef         `json:"source"`
	Children   []*Node           `json:"children,omitempty"`
}

// Task is an editable GFM task list item. MarkerStart points at the '[' in
// '[ ]' or '[x]' and is validated together with Source.Hash before editing.
type Task struct {
	ID          string            `json:"id"`
	Text        string            `json:"text"`
	Checked     bool              `json:"checked"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	Source      SourceRef         `json:"source"`
	MarkerStart int               `json:"markerStart"`
}

type Diagnostic struct {
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Source  SourceRef `json:"source"`
}

// Document is the stable JSON contract used by native apps and CLI clients.
type Document struct {
	SchemaVersion int            `json:"schemaVersion"`
	Entry         string         `json:"entry"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	Root          *Node          `json:"root"`
	Tasks         []Task         `json:"tasks"`
	Dependencies  []string       `json:"dependencies"`
	Diagnostics   []Diagnostic   `json:"diagnostics"`
}

var ErrStaleTask = errors.New("task source changed since parsing")
var ErrTaskMarker = errors.New("task marker is missing at its source location")
var ErrTaskStatus = errors.New("invalid task status")

var statusIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
var statusAttributePattern = regexp.MustCompile(`(^|\s)status="[^"]*"`)

func contentHash(source []byte) string {
	sum := sha256.Sum256(source)
	return hex.EncodeToString(sum[:])
}

// SetTaskChecked returns an edited copy of the original Markdown file.
// It never writes files, so the caller can commit or synchronize atomically.
func SetTaskChecked(source []byte, task Task, checked bool) ([]byte, error) {
	if contentHash(source) != task.Source.Hash {
		return nil, ErrStaleTask
	}
	start := task.MarkerStart
	if start < 0 || start+3 > len(source) || source[start] != '[' || source[start+2] != ']' ||
		(source[start+1] != ' ' && source[start+1] != 'x' && source[start+1] != 'X') {
		return nil, ErrTaskMarker
	}
	result := append([]byte(nil), source...)
	if checked {
		result[start+1] = 'x'
	} else {
		result[start+1] = ' '
	}
	return result, nil
}

// SetTaskStatus updates only the original task line. It keeps other metadata
// attributes and sets the checkbox when the destination is the final status.
// The caller determines that ordering from the board's front matter.
func SetTaskStatus(source []byte, task Task, status string, completed bool) ([]byte, error) {
	if !statusIDPattern.MatchString(status) {
		return nil, ErrTaskStatus
	}
	if contentHash(source) != task.Source.Hash {
		return nil, ErrStaleTask
	}
	start := task.MarkerStart
	if start < 0 || start+3 > len(source) || source[start] != '[' || source[start+2] != ']' ||
		(source[start+1] != ' ' && source[start+1] != 'x' && source[start+1] != 'X') {
		return nil, ErrTaskMarker
	}
	lineStart := bytes.LastIndexByte(source[:start], '\n') + 1
	lineEnd := len(source)
	if newline := bytes.IndexByte(source[start:], '\n'); newline >= 0 {
		lineEnd = start + newline
	}
	line := append([]byte(nil), source[lineStart:lineEnd]...)
	metadata := taskMetadataPattern.FindSubmatchIndex(line)
	if metadata != nil {
		attrs := line[metadata[2]:metadata[3]]
		if _, err := parseAttributes(string(attrs)); err != nil {
			return nil, err
		}
		if current := statusAttributePattern.FindIndex(attrs); current != nil {
			prefix := ""
			if attrs[current[0]] == ' ' || attrs[current[0]] == '\t' {
				prefix = " "
			}
			replacement := []byte(prefix + `status="` + status + `"`)
			line = append(append(append([]byte(nil), line[:metadata[2]+current[0]]...), replacement...),
				line[metadata[2]+current[1]:]...)
		} else {
			position := metadata[3]
			line = append(append(append([]byte(nil), line[:position]...), []byte(` status="`+status+`"`)...), line[position:]...)
		}
	} else {
		line = append(bytes.TrimRight(line, " \t"), []byte(` <!-- md:task status="`+status+`" -->`)...)
	}
	line[start-lineStart+1] = ' '
	if completed {
		line[start-lineStart+1] = 'x'
	}
	result := append([]byte(nil), source[:lineStart]...)
	result = append(result, line...)
	result = append(result, source[lineEnd:]...)
	return result, nil
}
