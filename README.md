# markdown-parser

Go library that turns Markdown into a source-aware, renderer-neutral AST. The
same JSON contract is intended for a native Swift renderer on iOS and macOS.

## Parse

```go
files, err := os.OpenRoot("/path/to/notes")
if err != nil { /* handle error */ }
defer files.Close()
doc, err := markdownparser.Parse(context.Background(), files.FS(), "index.md")
```

`Document.SchemaVersion` is currently `1`. Each node has a `kind`, source
path, UTF-8 byte offsets, content hash, and include stack. `Tasks` is the
deduplicated index of visible tasks after includes and queries resolve.
`Dependencies` lists files read while constructing the document. Diagnostics
report recoverable errors such as missing includes and cycles.

## Extensions

Directives must occupy their own paragraph. Paths are relative to the file
containing the directive and stay inside the document root.

```md
::include path="notes/meeting.md"
::include path="notes/meeting.md" section="summary"

::tasks from="tasks/**/*.md" where="status=open" sort="due"

:::callout kind="note"
Remember this.
:::

:::summary
Short version for dashboards and other notes.
:::

- [ ] Pay invoice <!-- md:task id="invoice" status="open" due="2026-10-01" -->
```

Standard Markdown and GitHub Flavored Markdown tasks and tables use Goldmark.
An included file may expose a `:::summary` block; `section="summary"` includes
only that block's contents. An include without `section` keeps including the
whole file.
YAML front matter is exposed as document metadata. Directive-like text in a
fenced code block stays code. `WIDGET_TODO.md` and `WIDGET_MEMO.md` are app
entrypoint conventions, not special syntax in the parser: apps can parse them
and use the expanded AST to populate widgets.

`SetTaskChecked` returns edited Markdown source. It checks the source hash
before changing a task marker, so callers can resolve sync conflicts instead
of overwriting newer edits.
`SetTaskStatus(source, task, status, completed)` also checks the source hash,
then edits only the original task line's `status` attribute and checkbox marker.
The application supplies the ordered status definition from YAML front matter.

## Apple bridge

The `mobile` package exposes `ParseDirectory(root, entry)` and
`SetTaskChecked(source, taskJSON, checked)` and
`SetTaskStatus(source, taskJSON, status, completed)` as string-based functions suitable
for Go Mobile binding. Generate the Apple XCFramework with:

```sh
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init
gomobile bind -target=ios,iossimulator,macos -o MarkdownParser.xcframework ./mobile
```

Generated binaries are release artifacts, not source files. The module can be
used directly with `go get github.com/tubasasakunn/markdown-parser`.

## Test and inspect

```sh
go test ./...
go run ./cmd/mdparse -root testdata/tasks WIDGET_TODO.md
```

Reminders use editable task checkboxes with `md:reminder` metadata:

```md
- [ ] Take medicine <!-- md:reminder id="medicine" at="2026-10-04T09:00" -->
```

`at` accepts a local date and time (`YYYY-MM-DDTHH:mm`) or RFC3339 with a
UTC offset. The parser adds `reminder="true"` to the task attributes and reports
`reminder_invalid` for missing or invalid dates. Native clients interpret local
times in the device time zone and schedule notifications. Use
`::tasks from="**/*.md" where="reminder=true" sort="at"` to collect reminders.
Fenced examples are not reminders. Completion still uses `SetTaskChecked` and
preserves the reminder's metadata and source file.
