// Package mobile exposes the parser through Go Mobile bindings for Swift.
// Only strings cross the language boundary; schemaVersion identifies the JSON
// contract in the returned document.
package mobile

import (
	"context"
	"encoding/json"
	"os"

	markdownparser "github.com/tubasasakunn/markdown-parser"
)

// ParseDirectory resolves entry and its includes within root and returns a
// renderer-neutral JSON document. root must be a directory the app can read.
func ParseDirectory(root, entry string) (string, error) {
	files, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer files.Close()
	doc, err := markdownparser.Parse(context.Background(), files.FS(), entry)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// SetTaskChecked receives original source and a Task object from ParseDirectory
// and returns updated source. The source hash prevents stale edits.
func SetTaskChecked(source, taskJSON string, checked bool) (string, error) {
	var task markdownparser.Task
	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		return "", err
	}
	updated, err := markdownparser.SetTaskChecked([]byte(source), task, checked)
	if err != nil {
		return "", err
	}
	return string(updated), nil
}

// SetTaskStatus changes the status metadata and completed marker of a task.
func SetTaskStatus(source, taskJSON, status string, completed bool) (string, error) {
	var task markdownparser.Task
	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		return "", err
	}
	updated, err := markdownparser.SetTaskStatus([]byte(source), task, status, completed)
	if err != nil {
		return "", err
	}
	return string(updated), nil
}
