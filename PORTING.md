# goman Rebranding Pipeline

The IDE todo-list tool is unavailable. This checklist tracks the requested PF stages. Older entries below are historical records of the port; references to the original name and Python describe that earlier work, not the current distribution.

- [x] Planning: rename the app, Go module/package, runtime labels, client identity, and webhook response; remove Python-only code and assets.
- [x] Critiquing Plan: preserve the MIT copyright notice and existing saved requests; retain the customized lazygit dependency.
- [x] Fixing Plan: use `.goman` for new storage and import legacy `.ghostman` collections without rewriting the originals; keep historical documentation separate from current usage.
- [x] Implementation: renamed the app directory and Go code to goman, wired workspace import, and removed Python sources, tooling, and screenshots.
- [x] Critiquing Implementation: checked active branding references, protocol identity, migration precedence, packaging, and original attribution; identified missing JSON-only and empty-legacy migration coverage.
- [x] Fixing Implementation: added legacy edge-case coverage and verified that corrupt current collections never fall back to an older valid legacy copy.
- [x] Shrinking: reused the existing collection parser and atomic-save implementation; removed duplicate Python documentation and obsolete assets without modifying saved requests.
- [x] Critiquing: full race tests verify UI workflows, network/webhook behavior, rebranded identities, and legacy-data preservation.
- [x] Optimizing: retained the dependency graph, bounded rendering, history limits, and existing storage implementation without a separate migration framework.
- [x] Documenting: replaced the README with Go-only setup, controls, limits, migration, and credits to 4yk5 and the original Ghostman repository; retained the original MIT notice.

Verification passed after gofmt: `go test -race ./...`, `go vet ./...`, and a standalone build. The built executable reports `goman 0.1.0`; `--help` shows the new storage path and legacy import. The sibling lazygit source and existing collection files were not modified.
# Go Port Pipeline

The IDE todo-list tool is unavailable in this session. This checklist tracks the requested PF stages instead.

- [x] Planning: preserve Python and add a Go executable; reuse the local gocui fork and existing collection terminology.
- [x] Critiquing Plan: preserve YAML precedence and stored headers; avoid insecure TLS, hidden storage errors, unbounded history, and blocking terminal work.
- [x] Fixing Plan: use a local module replacement, net/http, yaml.v3, atomic file replacement, cancellation, size limits, and simulation-screen tests.
- [x] Implementation: CLI, editor/navigation, async send/save, response metrics and headers, webhook inspection and replay.
- [x] Critiquing Implementation: found shutdown lock inversion, aliased history headers, large history retention, and missing event-loop coverage.
- [x] Fixing Implementation: repaired lifecycle locking, isolated snapshots, bounded history; real keyboard-dispatch and async workflow tests pass with race detection.
- [x] Shrinking: removed the redundant editor wrapper and reused the fork's built-in editor and screen APIs.
- [x] Critiquing: verified editor, replay, focus, and minimum-terminal behavior; corrected tab clicks and added the visible New Request control.
- [x] Optimizing: render only active panes, cache unchanged content, cap previews at 128 KiB, retain at most 200 hooks and 8 MiB of payload text, and release evicted references.
- [x] Documenting: README now includes Go launch/build commands, flags, storage/replay contracts, limits, TLS differences, and the local fork dependency.

## Verification

Passed after gofmt:

```sh
go test -race ./...
go vet ./...
go build ./...
go run . --version
go run . --help
```

Local fixtures cover collections, methods, redirects, TLS, cancellation, payload limits, chunked webhooks, bind failures, history budgets, and lifecycle. Simulation screens cover editors, selection, replay, navigation, sending, saving, minimum dimensions, preview truncation, and rendered content. Python and lazygit source files remain unchanged.
## Mouse Interaction Follow-Up

- [x] Planning: identify missing wheel bindings and field-local character selection.
- [x] Critiquing Plan: whole-line range selection does not meet the request; terminal selection would include neighboring controls.
- [x] Fixing Plan: reuse gocui mouse capture, keep byte ranges within one field, and render selection without changing its value.
- [x] Implementation: hovered-field wheel scrolling, character selection, copying, replacement, deletion, and Escape clearing.
- [x] Critiquing Implementation: check real event dispatch, Unicode boundaries, tab widths, read-only fields, and drags over buttons.
- [x] Fixing Implementation: align tab mapping with four-column display stops and replace text using grapheme-safe cursor movement.
- [x] Shrinking: reuse the existing editor and mouse-capture APIs; no lazygit modifications.
- [x] Critiquing: helper and real event-loop tests pass, including field isolation and rendered selection highlighting.
- [x] Optimizing: reuse cached field content and clamp scrolling and selection to each field's bounds.
- [x] Documenting: mouse controls documented in README; go test -race ./..., go vet ./..., and go build ./... all passed after gofmt.
## Syntax Highlighting Follow-Up

- [x] Planning: highlight payloads, request headers, responses, and webhook details without changing source values.
- [x] Critiquing Plan: colors must not leak into stored values or selections; incomplete JSON and large payloads need safe handling.
- [x] Fixing Plan: use Chroma lexers, preserve original token text, combine syntax colors with selection overlays, and bound lexer input.
- [x] Implementation: JSON, XML/HTML, YAML, HTTP header colors, and editable rendering integrated.
- [x] Critiquing Implementation: test visible colors, lexer configuration, selection clearing, incomplete JSON, webhook separators, and replay.
- [x] Fixing Implementation: initialize lexer state explicitly and separate webhook metadata from its payload with a blank line.
- [x] Shrinking: use one renderer for normal content and selected content.
- [x] Critiquing: source-preservation, actual-screen color, selection, replay, and event-loop tests pass with race detection.
- [x] Optimizing: cap lexer work at 64 KiB and reuse tokenized selections across drag events.
- [x] Documenting: format and limit notes added to README; go test -race ./..., go vet ./..., and go build ./... passed after gofmt.
## Visual Refinement Follow-Up

- [x] Planning: use the Python stylesheet as the palette and control-hierarchy reference.
- [x] Critiquing Plan: preserve terminal-native editing, field-local selection, syntax colors, and minimum dimensions rather than promise pixel-identical Textual widgets.
- [x] Fixing Plan: centralize view styling, use filled buttons, flat underlined tabs, a background canvas, and a real method dropdown.
- [x] Implementation: layered surfaces, compact sidebar New button, wider Send and Save buttons, tab hit areas, and keyboard/mouse dropdown controls.
- [x] Critiquing Implementation: test actual screen colors, minimum and wider layouts, mouse/keyboard choices, and outside-click dismissal.
- [x] Fixing Implementation: remove cancellation of an already-ended mouse gesture when keyboard selection closes the dropdown; the next mouse click now works.
- [x] Shrinking: reuse gocui views, existing content caching, focus handling, and event simulation; no lazygit modifications.
- [x] Critiquing: full race tests, vet, and build passed after gofmt; existing editing, selection, syntax, network, and webhook checks remain green.
- [x] Optimizing: cache unchanged tab and chrome content and create the dropdown only when opened.
- [x] Documenting: README describes the palette, dropdown navigation and dismissal, tested sizes, and terminal-rendering limitations.
## Control Padding and macOS Shortcuts

- [x] Planning: fix asymmetric control padding and provide lazygit-style bracket tab navigation.
- [x] Critiquing Plan: preserve label coordinates, minimum dimensions, mouse hit areas, and Escape dismissal despite the legacy Ctrl+[ alias.
- [x] Fixing Plan: use centered three-row controls, padded dropdown rows, explicit Ctrl bracket bindings, and selection-aware Escape fallback.
- [x] Implementation: centered button and method labels, two-column dropdown insets, inert edge rows, wrapped previous/next tab shortcuts, and updated footer.
- [x] Critiquing Implementation: verify actual mouse/key dispatch, caret-only clicks, editor preservation, and narrow layouts.
- [x] Fixing Implementation: align option hit coordinates with the inset and check preservation against the workflow's actual local URL and sample body.
- [x] Shrinking: reuse the button renderer, selection clearing, tab switching, and existing event-loop test.
- [x] Critiquing: focused tests and full race tests, vet, and build passed after gofmt.
- [x] Optimizing: retain cached rendering and existing geometry for payload and response fields.
- [x] Documenting: README explains padding, bracket shortcuts, and the legacy Escape alias.
## Select All Follow-Up

- [x] Planning: add focused-field Select All without replacing Ctrl+A line navigation.
- [x] Critiquing Plan: Cmd+A is usually intercepted by the terminal, and legacy terminals may collapse Ctrl+Shift+A into Ctrl+A.
- [x] Fixing Plan: support Ctrl+Shift+A where distinct modifiers are available and Ctrl+G as a terminal-compatible fallback.
- [x] Implementation: select the full field value using the existing syntax-aware selection renderer; retain copy, replacement, deletion, and Escape clearing.
- [x] Critiquing Implementation: cover editable and read-only fields, offscreen Unicode text, empty values, unchanged viewports, and actual key dispatch.
- [x] Fixing Implementation: accept lowercase and uppercase shifted key events and leave plain Ctrl+A unchanged.
- [x] Shrinking: reuse selection state and rendering instead of adding another editor or clipboard path.
- [x] Critiquing: real event-loop tests verify both shortcut forms, visible highlighting, selection clearing, and Ctrl+A beginning-of-line editing.
- [x] Optimizing: use existing bounded syntax tokenization and preserve viewport position.
- [x] Documenting: README and footer describe shortcuts and optional terminal Cmd+A mapping; full race tests, vet, build, and editor diagnostics passed.
## Quit Confirmation and Dynamic Remapping

- [x] Planning: reserve Escape for quit confirmation with second-Escape cancellation and add a quick application-shortcut window.
- [x] Critiquing Plan: keep modal input isolated, protect fixed keys, handle legacy Escape aliases, and avoid silently persisting unrequested settings.
- [x] Fixing Plan: use centered gocui modal views, a draft key table, recorded key events, conflict checks, and session-only Apply/Cancel/Reset.
- [x] Implementation: Escape and Ctrl+Q confirmation, safe initial Cancel focus, mouse/keyboard buttons, F2 shortcut configuration, and dynamic footer bindings.
- [x] Critiquing Implementation: check key dispatch precedence, reserved keys, collisions, narrow-window fit, focus restoration, and background-action suppression.
- [x] Fixing Implementation: bind modal Escape/Tab locally ahead of the recording editor, give assigned commands precedence in editable fields, reject non-key recording events, and restore method focus after cancellation.
- [x] Shrinking: centralize application bindings and reuse existing selection, view styling, content caching, and event-loop tests; no lazygit changes.
- [x] Critiquing: real events verify double-Escape cancellation, recording, Apply, old-key removal, Ctrl-key remapping, and confirmed shutdown.
- [x] Optimizing: rebuild bindings only on Apply and reuse cached modal content; keep draft changes isolated from active bindings.
- [x] Documenting: README covers confirmation, F2 controls, protected keys, session-only changes, and the legacy Ctrl+[ limitation; full race tests, vet, and build passed after gofmt.
## Positive Confirmation on Enter

- [x] Planning: make Enter accept the positive quit action.
- [x] Critiquing Plan: keep Escape and mouse Cancel working; do not change configuration-window Enter behavior.
- [x] Fixing Plan: focus Quit initially and route confirmation Enter directly to the positive action.
- [x] Implementation: positive Enter handling is separate from each button's mouse handler.
- [x] Critiquing Implementation: check changed default focus and Enter with Cancel focused.
- [x] Fixing Implementation: update quit and event-loop expectations while retaining Escape cancellation.
- [x] Shrinking: reuse the existing confirmation handler without another modal abstraction.
- [x] Critiquing: focused race tests and full race tests, vet, and build passed.
- [x] Optimizing: no added rendering work or state.
- [x] Documenting: README now describes positive Enter confirmation and unchanged configuration controls; editor diagnostics are clean.
## Remaining UI Parity Features

- [x] Planning: implement click-to-load webhook payloads, inline webhook snippets, colored HTTP statuses, and transient notifications.
- [x] Critiquing Plan: preserve destination URLs, raw payloads, field-local selections, modal isolation, and bounded rendering.
- [x] Fixing Plan: reuse replay for clicks, bound single-line snippets, compute response telemetry independently of action status, and cancel obsolete notification timers.
- [x] Implementation: webhook clicks load the editor; list rows show payload snippets; status labels, receiver state, footer feedback, and notification frames are color-coded.
- [x] Critiquing Implementation: verify real mouse events, source preservation, rendered colors, Unicode wrapping, replacement, expiry, and shutdown.
- [x] Fixing Implementation: normalize progress colors and keep notification expiry on the GUI thread without changing focus or obscuring dialogs.
- [x] Shrinking: reuse selection, replay, cached chrome, worker cancellation, and existing event-loop tests.
- [x] Critiquing: focused and full race tests, vet, build, and editor diagnostics passed.
- [x] Optimizing: limit snippet inspection to 256 bytes and display to 40 graphemes; bound notifications to three text lines and cancel replaced timers.
- [x] Documenting: README covers click replay, keyboard inspection, snippet limits, status colors, notification replacement, and dismissal.
## Review Regression Fixes

- [x] Planning and critique: address confirmed webhook frame-click replay and notification overwrite of in-flight metrics.
- [x] Implementation: use raw content click coordinates with viewport and row bounds; store request progress separately from action feedback.
- [x] Validation: real mouse tests cover borders, empty rows, and valid replay; viewport tests cover scrolled lists. A held HTTP fixture verifies that a webhook notification cannot replace active request telemetry.
- [x] Final checks: gofmt, full race tests, vet, build, and documentation updated. No changes to Python or lazygit sources.
## Internal Terminal UI Library

- [x] Planning: replace the lazygit module dependency with an internal copy of the exact customized gocui implementation.
- [x] Critiquing Plan: the current checkout's sibling replacement path is stale; retain platform code, regression tests, author attribution, and BSD license notices from the supplied source.
- [x] Fixing Plan: copy only gocui Go files, tests, LICENSE, and AUTHORS from `_ext/lazygit-master/pkg/gocui` into `internal/gocui`; leave external reference trees unchanged.
- [x] Implementation: switch application imports to `goman/internal/gocui`, remove lazygit require/replace directives, and tidy the remaining Go module dependencies.
- [x] Critiquing Implementation: compare internalized files against the source snapshot and check the module graph for lingering lazygit dependencies.
- [x] Fixing Implementation: update stale build instructions and describe the distinct BSD licensing obligations for the internal library.
- [x] Shrinking: exclude unrelated lazygit code, vendored dependency trees, and upstream project workflow files; reuse the full standalone library instead of rewriting its behavior.
- [x] Critiquing: copied gocui tests and application tests pass under race detection; no lazygit imports remain in Go code or module files.
- [x] Optimizing: no runtime behavior changes or extra module boundary; library dependencies are explicit in the root module.
- [x] Documenting: README builds from the repository root without a sibling checkout; `internal/gocui/UPSTREAM.md` records provenance, maintenance, and redistribution requirements.

Verification: `go test -race ./...` and `go vet ./...` passed. A temporary copy containing only module files, `main.go`, and `internal/` passed tests and native/Linux/Windows builds with `GOWORK=off GOPROXY=off`, using the populated Go module cache. The native executable reports `goman 0.1.0`. No `_ext` files were needed or modified.