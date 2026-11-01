# goman

A local-first terminal HTTP client and webhook inspector, written in Go. No account or hosted service is required.

## Features

- Compose GET, POST, PUT, PATCH, and DELETE requests with editable JSON headers and payloads.
- Inspect response headers, status, latency, size, and syntax-highlighted JSON, XML/HTML, or YAML.
- Save and load local collections in YAML and JSON.
- Receive webhooks locally, browse payload previews, and load captured requests into the editor.
- Use keyboard navigation, field-local mouse selection, clipboard copying, and configurable shortcuts.
- See transient action notifications and confirm before quitting.

## Build and Run

Requires Go 1.26 or newer and a terminal at least 80 columns by 22 rows. The customized terminal UI library is included in `internal/gocui`; no lazygit checkout, `_ext` directory, or local module replacement is needed.

```text
goman/
  go.mod
  main.go
  internal/
    gocui/
    goman/
    tui/
```

From the repository root:

```sh
go run .
```

Build a standalone executable:

```sh
go build -o goman .
./goman
```

On Windows, use `go build -o goman.exe .` and run `goman.exe`. The compiled executable needs neither Go nor source checkouts at runtime. The Python implementation and its tooling are no longer included.

Go resolves the remaining library dependencies through `go.mod` and `go.sum`. The internal gocui copy retains the customized mouse, editing, rendering, and simulation-screen behavior; see [its provenance and license notes](internal/gocui/UPSTREAM.md).

## Options

```sh
./goman --workspace /path/to/project
./goman --webhook-address 127.0.0.1:9098 --timeout 30s
./goman --insecure
./goman --version
```

| Option | Default | Meaning |
| --- | --- | --- |
| `--workspace` | Current directory | Directory containing local collections |
| `--webhook-address` | `127.0.0.1:9090` | Receiver address; starts offline |
| `--timeout` | `15s` | Positive HTTP request timeout |
| `--insecure` | Off | Disable TLS certificate verification |
| `--version` | Off | Print the application version |

TLS certificates are verified by default. Use `--insecure` only for trusted development endpoints. The webhook receiver has no authentication or signature verification; binding a non-loopback address exposes it to other hosts.

## Controls

| Key | Action |
| --- | --- |
| `Ctrl+S` | Send request and show the response |
| `Ctrl+K` | Save the current request |
| `Ctrl+N` | New request |
| `Ctrl+W` | Start or stop the webhook receiver |
| `Escape` / `Ctrl+Q` | Open quit confirmation |
| `F2` | Configure application shortcuts |
| `Tab` / `Shift+Tab` | Move focus |
| `Enter` | Activate a control or load a selected request |
| `Ctrl+[` / `Ctrl+]` | Previous / next tab when reported as distinct Ctrl events |
| `Alt+1` / `Alt+2` / `Alt+3` | Body / Response / Live Webhooks |
| `Ctrl+G` | Select all in the focused field |
| `Ctrl+Shift+A` | Select all when the terminal reports Shift separately |
| `Ctrl+C` | Copy selected field text |
| `Ctrl+A` | Beginning of line in an editor |

Click buttons and tabs to activate them. Open the method menu with a click, Enter, or Down; use arrows or the wheel to choose, then Enter or a click to commit. Clicking outside dismisses it without activating the control underneath.

Scroll over a field to scroll only that field. Drag to select text within one field; typing replaces selected text in editors, and Backspace/Delete removes it. Selection excludes neighboring panes, borders, and labels. Read-only Select All copies the displayed preview, not undisplayed data.

Escape opens quit confirmation from the main interface, including editors and the method menu. A second Escape cancels. Enter always accepts the positive quit action while confirmation is open; clicking Cancel also cancels. Background actions are blocked during dialogs.

### Terminal Compatibility

Some macOS keyboards require Fn+F2. Alt-number shortcuts depend on terminal settings. Legacy terminals report Ctrl+[ as Escape, so it opens quit confirmation; use F2 to remap Previous tab if needed.

Some terminals report Ctrl+Shift+A as Ctrl+A; use Ctrl+G for Select All instead. macOS Cmd+A is usually handled by the terminal application. An optional terminal key mapping can send Ctrl+G (`0x07`) to select the focused field in goman.

### Shortcut Configuration

Press F2, select an action with arrows, the wheel, or a click, and press Enter to record a replacement key. Modified character keys and F1-F12 are supported; conflicts are rejected. Escape, Tab, Shift+Tab, Enter, and Ctrl+A are protected.

Tab cycles through the list, Reset, Apply, and Cancel. Apply updates shortcuts immediately; Cancel discards the draft. Reset restores defaults in the draft and still requires Apply. Escape stops recording or closes the window. Changes are session-only, and replacing an action removes its previous aliases. Native navigation and plain typing are not remapped.

## Collections and Migration

Collections live in `.goman/collections.yaml` and `.goman/collections.json` beneath `--workspace`. YAML takes precedence; invalid YAML falls back to valid JSON. Empty collections are respected, and unreadable or corrupt data produces an error rather than silently restoring examples.

If neither goman collection file exists, startup looks for legacy `.ghostman` collections in the same workspace and imports them into `.goman`. The original files remain untouched. Existing `.goman` files always take precedence, even if they are empty or invalid. With neither format present in either location, goman creates three example requests.

Import preserves names, payloads, URLs, and headers exactly as stored, including any old product names in user data. New saves go only to `.goman`; later changes to the legacy files are not synchronized.

Save appends to the first collection. Both formats use private file permissions and atomic replacement per file, but the pair is not a single transaction. Do not run multiple instances writing to the same workspace. Collection files may contain credentials: keep them out of version control unless reviewed.

## Webhook Inspection

Start the receiver with Ctrl+W, then send a local request:

```sh
curl -X POST http://127.0.0.1:9090/webhook \
  -H 'Content-Type: application/json' \
  -d '{"event":"example","value":42}'
```

Each webhook row includes a single-line payload preview. Keyboard navigation or scrolling selects a capture for inspection. Clicking a row or pressing Enter loads its method and raw body into the editor, preserving the destination URL; only Content-Type and Accept headers are copied. Loading does not send the request. Borders and empty rows do not load a capture.

The receiver responds with `{"status":"ok","source":"goman"}`. Captures stay in memory and disappear on exit. Clients relying on the former `source` value should be updated.

## Behavior and Limits

- GET and HEAD omit the editor body; other methods send it. Methods loaded from collections or webhooks, including OPTIONS and custom methods, are preserved exactly until another method is selected. Valid JSON receives Content-Type automatically unless already set.
- Redirects follow Go's standard HTTP behavior. HTTP error responses remain inspectable; network failures show their error.
- An explicitly configured Host header overrides the HTTP host without changing the connection destination.
- The default outgoing User-Agent is `goman/0.1.0`; an explicitly stored User-Agent is preserved.
- Responses and incoming webhook bodies are limited to 4 MiB. History retains at most 200 captures and 8 MiB of raw plus formatted payload text.
- JSON pretty-printing uses a conservative 4 MiB output budget checked before indentation. When formatting would exceed that budget, the original JSON is displayed instead.
- Body previews are capped at 128 KiB with a truncation marker. Syntax highlighting processes at most the first 64 KiB without changing source values.
- Webhook list snippets inspect at most 256 bytes and display up to 40 grapheme clusters.
- Green/red HTTP status labels distinguish responses below 400 from errors. Notifications expire after two seconds or can be dismissed by clicking; they do not take keyboard focus.
- Notifications cannot replace in-flight request telemetry and remain hidden behind dialogs or the method menu.
- Bracketed paste inserts text without activating Send or other command bindings. Dialogs hide below the minimum terminal size and restore when space is available; quit confirmation remains keyboard-accessible.

Fonts, cell sizes, clipboard access, and modifier-key reporting depend on the terminal and operating system.

## Development

```sh
go test -race ./...
go vet ./...
go build ./...
```

Tests include the internal gocui suite and local HTTP/simulation-screen fixtures for mouse/key dispatch, modal behavior, saved-data migration, selection, and notification lifecycle. See [PORTING.md](PORTING.md) for the historical PF workflow and review record.

## Credits and License

goman is a Go reimplementation derived from [Ghostman](https://github.com/4yk5/ghostman), created by [4yk5](https://github.com/4yk5). Credit for the original application, workflow, and visual design belongs to that project and its creator.

The terminal UI contains an internal copy of the customized [gocui](https://github.com/jesseduffield/gocui) code supplied with [lazygit](https://github.com/jesseduffield/lazygit). There is no lazygit module dependency. Original authors and provenance are recorded in [internal/gocui/UPSTREAM.md](internal/gocui/UPSTREAM.md). Syntax highlighting uses [Chroma](https://github.com/alecthomas/chroma).

goman is released under the [MIT License](LICENSE), retaining the original `Copyright (c) 2026 4yk5` notice. The internal gocui package retains its [BSD-3-Clause license](internal/gocui/LICENSE). Binary distributions must include both license notices.