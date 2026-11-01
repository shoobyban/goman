# Internal gocui Local Changes

The customized gocui source is maintained inside goman. Its original [BSD license](LICENSE), [author list](AUTHORS), and source copyright notices remain in place.

The source was copied from the supplied lazygit checkout's `pkg/gocui` package. There is no lazygit module dependency.

- Replaced the integer set in redraw tracking with a native `map[int]bool`, removing the `github.com/jesseduffield/generics` module dependency. Redraw ordering and overlap propagation are unchanged.
- Bracketed paste goes directly to editable fields instead of invoking local, parent, or global command bindings.
- The headless terminal serializes emulator restart against outstanding reads and writes, avoiding the tcell 3.6.0 mock-terminal suspend/resume race. Real-terminal behavior is unchanged.

Verified with redraw, paste, and repeated suspend/resume tests, `go test -race ./... -count=1`, `go vet ./...`, and `go build ./...`.