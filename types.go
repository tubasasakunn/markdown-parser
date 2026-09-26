// Package markdownparser parses Markdown into a source-aware document tree.
// The tree is independent of a particular HTML or native renderer.
package markdownparser

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
