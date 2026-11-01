package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
	"github.com/rivo/uniseg"
	"goman/internal/gocui"
	"goman/internal/goman"
)

func testApp(t *testing.T, width, height int) *App {
	t.Helper()
	gui, err := gocui.NewGui(gocui.NewGuiOpts{OutputMode: gocui.OutputTrue, Headless: true, Width: width, Height: height})
	if err != nil {
		t.Fatal(err)
	}
	app := New(goman.Storage{Dir: filepath.Join(t.TempDir(), ".goman")}, goman.DefaultCollections(), goman.NewClient(time.Second, false), "127.0.0.1:0")
	app.setup(gui)
	t.Cleanup(func() { app.Close(); gui.Close() })
	if err := app.layout(gui); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestEditorSelectionAndNavigation(t *testing.T) {
	app := testApp(t, 120, 36)
	app.selected = 1
	if err := app.loadSelected(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	request, err := app.currentRequest()
	if err != nil || request.Method != "POST" || request.Headers["Content-Type"] != "application/json" || !strings.Contains(request.Body, "userId") {
		t.Fatalf("selected request: %+v %v", request, err)
	}
	body, _ := app.gui.View("body")
	if !body.Editor.Edit(body, gocui.NewKeyRune('!')) {
		t.Fatal("editor rejected input")
	}
	if !strings.HasSuffix(body.TextArea.GetContent(), "!") {
		t.Fatal("editor content not updated")
	}
	if err := app.switchTab(1); err != nil {
		t.Fatal(err)
	}
	response, _ := app.gui.View("response")
	if !response.Visible || body.Visible {
		t.Fatal("tab visibility incorrect")
	}
	if err := app.cycleFocus(1); err != nil {
		t.Fatal(err)
	}
	if app.gui.CurrentView().Name() != "collections" {
		t.Fatal("focus did not cycle")
	}
	if err := app.newRequest(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	address, _ := app.gui.View("url")
	if address.TextArea.GetContent() != "" || body.TextArea.GetContent() != "" || app.gui.CurrentView().Name() != "url" {
		t.Fatal("new request did not reset editor")
	}
}

func TestWebhookInspectAndReplay(t *testing.T) {
	app := testApp(t, 100, 28)
	if err := app.webhook.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	response, err := http.Post("http://"+app.webhook.Address()+"/event", "application/json", strings.NewReader(`{"event":"local"}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if err := app.switchTab(2); err != nil {
		t.Fatal(err)
	}
	detail, _ := app.gui.View("hook-detail")
	if !strings.Contains(detail.Buffer(), "Content-Type: application/json") || !strings.Contains(detail.Buffer(), "local") {
		t.Fatal("hook headers/body not displayed")
	}
	hooks, _ := app.gui.View("hooks")
	if !strings.Contains(hooks.Buffer(), `-> { "event": "local" }`) {
		t.Fatal("webhook list did not include a single-line payload preview")
	}
	if err := app.replay(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	request, err := app.currentRequest()
	if err != nil || request.Body != `{"event":"local"}` || request.Method != "POST" || !strings.HasPrefix(request.URL, "https://jsonplaceholder") {
		t.Fatalf("replay: %+v %v", request, err)
	}
	if err := app.webhook.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWebhookClicksRespectViewport(t *testing.T) {
	app := testApp(t, 100, 28)
	if err := app.webhook.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	for index := range 6 {
		response, err := http.Post("http://"+app.webhook.Address()+"/event", "text/plain", strings.NewReader(fmt.Sprintf("payload %d", index)))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	if err := app.switchTab(2); err != nil {
		t.Fatal(err)
	}
	body, _ := app.gui.View("body")
	setEditor(body, "unsaved editor text")
	hooks, _ := app.gui.View("hooks")
	hooks.SetOrigin(2, 2)
	for _, point := range []struct{ x, y int }{{1, 3}, {2 + hooks.InnerWidth(), 3}, {3, 1}, {3, 2 + hooks.InnerHeight()}} {
		if err := app.clickHook(gocui.ViewMouseBindingOpts{X: point.x, Y: point.y}); err != nil {
			t.Fatal(err)
		}
		if app.tab != 2 || app.hookSelected != 0 || fieldText(body) != "unsaved editor text" {
			t.Fatal("scrolled border click changed the selection or editor")
		}
	}
	if err := app.clickHook(gocui.ViewMouseBindingOpts{X: 3, Y: 3}); err != nil {
		t.Fatal(err)
	}
	if app.tab != 0 || app.hookSelected != 3 || fieldText(body) != "payload 3" {
		t.Fatal("scrolled content click loaded the wrong webhook")
	}
}

func TestWebhookReplayPreservesMethods(t *testing.T) {
	app := testApp(t, 100, 28)
	if err := app.webhook.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	for index, method := range []string{"HEAD", "OPTIONS", "PROPFIND", "custom"} {
		request, err := http.NewRequest(method, "http://"+app.webhook.Address()+"/event", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := app.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		app.hookSelected = index
		if err := app.replay(app.gui, nil); err != nil {
			t.Fatal(err)
		}
		loaded, err := app.currentRequest()
		if err != nil || loaded.Method != method {
			t.Fatalf("replayed method lost: %+v %v", loaded, err)
		}
	}
}

func TestHookPreview(t *testing.T) {
	for _, fixture := range []struct{ text, expected string }{
		{"one\n\ttwo\r\nthree", "one two three"},
		{"", ""},
		{strings.Repeat("x", 41), strings.Repeat("x", 40) + "..."},
		{strings.Repeat("e\u0301", 41), strings.Repeat("e\u0301", 40) + "..."},
		{"\x1b[31mhello", "[31mhello"},
	} {
		if result := hookPreview(fixture.text); result != fixture.expected {
			t.Fatalf("preview: got %q, want %q", result, fixture.expected)
		}
	}
}

func TestRenderedStatusFeedback(t *testing.T) {
	app := testApp(t, 100, 28)
	app.switchTab(1)
	app.busy = true
	app.requestProgress = "Sending GET to https://example.test/slow"
	for _, message := range []string{"Hook received: /unrelated", "Saved: test", "Request already in progress"} {
		app.feedback(message, noticeInfo)
		app.drawMetrics()
		metrics, _ := app.gui.View("metrics")
		if strings.TrimSuffix(metrics.Buffer(), "\n") != app.requestProgress || app.status != message || app.notice != message {
			t.Fatal("action feedback overwrote in-flight telemetry or failed to display separately")
		}
	}
	app.busy = false
	for _, fixture := range []struct {
		code  int
		err   error
		label string
		color tcell.Color
	}{
		{200, nil, "200 OK", tcell.NewRGBColor(63, 185, 80)},
		{404, nil, "404 OK", tcell.NewRGBColor(248, 81, 73)},
		{0, errors.New("connection refused"), "Failed", tcell.NewRGBColor(248, 81, 73)},
	} {
		app.response = goman.Response{StatusCode: fixture.code, Reason: "OK", Err: fixture.err}
		app.drawMetrics()
		view, _ := app.gui.View("metrics")
		app.gui.ForceFlushViewsContentOnly([]*gocui.View{view})
		_, style, _ := gocui.Screen.Get(27, 9)
		if !strings.Contains(view.Buffer(), fixture.label) || style.GetForeground() != fixture.color {
			t.Fatalf("status color did not render for %s", fixture.label)
		}
	}
	app.feedback("Saved: test", noticeSuccess)
	before := app.gui.CurrentView().Name()
	if err := app.layout(app.gui); err != nil {
		t.Fatal(err)
	}
	view, _ := app.gui.View("notification")
	if !view.Visible || view.FrameColor != noticeColor(noticeSuccess) || !strings.Contains(view.Buffer(), "Saved: test") || app.gui.CurrentView().Name() != before {
		t.Fatal("notification failed or stole focus")
	}
	app.requestQuit(app.gui, nil)
	app.layout(app.gui)
	if view.Visible {
		t.Fatal("notification appeared over confirmation")
	}
	app.closeDialog()
	app.dismissNotice()
	app.layout(app.gui)
	if view.Visible {
		t.Fatal("dismissed notification remained visible")
	}
	for _, text := range []string{strings.Repeat("long-word", 100), strings.Repeat("\u754ce\u0301 ", 100), "first\nsecond\tthird"} {
		lines := strings.Split(noticeLines(text, 20), "\n")
		if len(lines) > 3 {
			t.Fatal("notification exceeded three lines")
		}
		for _, line := range lines {
			if uniseg.StringWidth(strings.TrimPrefix(line, " ")) > 20 {
				t.Fatalf("notification text overflowed: %q", line)
			}
		}
	}
}

func TestSmallTerminal(t *testing.T) {
	app := testApp(t, 60, 16)
	view, err := app.gui.View("small")
	if err != nil || !strings.Contains(view.Buffer(), "80 columns") {
		t.Fatalf("small terminal: %v", err)
	}
	if err := app.newRequest(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	if err := app.switchTab(2); err != nil {
		t.Fatal(err)
	}
	if err := app.cycleFocus(1); err != nil {
		t.Fatal(err)
	}
}

func TestSmallTerminalHidesDialogs(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 6}, {1, 1}, {79, 22}, {80, 21}} {
		app := testApp(t, size.width, size.height)
		for _, dialog := range []string{"keys", "quit"} {
			app.dialog = dialog
			app.shortcutDraft = defaultShortcuts()
			if err := app.layout(app.gui); err != nil {
				t.Fatalf("small layout %s: %v", dialog, err)
			}
			if err := app.drawDialog(); err != nil {
				t.Fatalf("small dialog %s: %v", dialog, err)
			}
			for _, name := range []string{"dialog", "dialog-shortcuts", "dialog-primary", "dialog-cancel"} {
				if view, err := app.gui.View(name); err == nil && view.Visible {
					t.Fatalf("undersized dialog visible: %s", name)
				}
			}
		}
	}
}

func TestSmallTerminalQuit(t *testing.T) {
	app := testApp(t, 60, 6)
	if err := app.requestQuit(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	view := app.gui.CurrentView()
	if view == nil || view.Name() != "small" || !strings.Contains(view.Buffer(), "Enter to quit") {
		t.Fatal("small terminal quit prompt is not focused or visible")
	}
	if !errors.Is(app.confirmQuit(app.gui, nil), gocui.ErrQuit) {
		t.Fatal("small terminal quit could not be confirmed")
	}
	app.escape(app.gui, nil)
	if app.dialog != "" {
		t.Fatal("small terminal quit could not be cancelled")
	}
}

func TestShortcutDialogResizeRecovery(t *testing.T) {
	app := testApp(t, 100, 28)
	body, _ := app.gui.View("body")
	setEditor(body, "unsaved body")
	if err := app.openShortcuts(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	app.shortcutIndex = 3
	app.shortcutDraft[0].keys = []gocui.Key{gocui.NewKeyName(gocui.KeyF3)}
	terminal, _ := gocui.Screen.Tty()
	mock := terminal.(vt.MockTerm)
	mock.SetSize(vt.Coord{X: 60, Y: 6})
	gocui.Screen.Sync()
	if err := app.gui.ForceLayoutAndRedraw(); err != nil {
		t.Fatal(err)
	}
	list, _ := app.gui.View("dialog-shortcuts")
	if list.Visible || app.gui.CurrentView().Name() != "small" {
		t.Fatal("shortcuts did not hide on resize")
	}
	mock.SetSize(vt.Coord{X: 100, Y: 28})
	gocui.Screen.Sync()
	if err := app.gui.ForceLayoutAndRedraw(); err != nil {
		t.Fatal(err)
	}
	if !list.Visible || app.gui.CurrentView().Name() != "dialog-shortcuts" || app.dialog != "keys" || app.shortcutIndex != 3 || !app.shortcutDraft[0].keys[0].Equals(gocui.NewKeyName(gocui.KeyF3)) || fieldText(body) != "unsaved body" {
		t.Fatal("resizing lost shortcut state, focus, or editor contents")
	}
}

func TestLoadedMethodsPreserved(t *testing.T) {
	app := testApp(t, 100, 28)
	for _, method := range []string{"HEAD", "OPTIONS", "PROPFIND", "custom", "POST"} {
		if err := app.load(goman.Request{Method: method, URL: "https://example.test", Body: "payload"}); err != nil {
			t.Fatal(err)
		}
		request, err := app.currentRequest()
		if err != nil || request.Method != method || request.Body != "payload" {
			t.Fatalf("loaded method lost: %+v %v", request, err)
		}
	}
	app.customMethod = "HEAD"
	app.nextMethod(app.gui, nil)
	if app.customMethod != "" {
		t.Fatal("keyboard method choice did not clear custom method")
	}
	app.customMethod = "OPTIONS"
	app.methodOption = 1
	app.chooseMethod(app.gui, nil)
	if app.requestMethod() != "POST" {
		t.Fatal("menu choice did not replace custom method")
	}
}

func TestQuitConfirmation(t *testing.T) {
	app := testApp(t, 100, 28)
	if err := app.escape(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	if app.dialog != "quit" || app.gui.CurrentView().Name() != "dialog-primary" || app.tab != 0 {
		t.Fatal("Escape did not open quit confirmation with the positive action focused")
	}
	if err := app.escape(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	if app.dialog != "" || app.gui.CurrentView().Name() != "url" || app.tab != 0 {
		t.Fatal("second Escape did not cancel quitting")
	}
	if err := app.requestQuit(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	if err := app.cycleFocus(1); err != nil {
		t.Fatal(err)
	}
	if app.gui.CurrentView().Name() != "dialog-cancel" || !errors.Is(app.confirmQuit(app.gui, nil), gocui.ErrQuit) {
		t.Fatal("confirmed quit did not exit")
	}
	app.closeDialog()
	app.toggleMethodMenu(app.gui, nil)
	app.escape(app.gui, nil)
	app.escape(app.gui, nil)
	if app.gui.CurrentView().Name() != "method" || app.methodOpen {
		t.Fatal("quit cancellation did not restore method focus")
	}
}

func TestShortcutRemapping(t *testing.T) {
	app := testApp(t, 80, 22)
	if err := app.openShortcuts(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	list, _ := app.gui.View("dialog-shortcuts")
	if app.dialog != "keys" || list.InnerHeight() < len(app.shortcuts) || app.gui.CurrentView().Name() != "dialog-shortcuts" {
		t.Fatal("shortcut window does not fit or focus its list")
	}
	action := app.shortcutDraft[0].keys[0]
	for _, key := range []gocui.Key{gocui.NewKeyName(gocui.KeyEsc), gocui.NewKeyName(gocui.KeyEnter), gocui.NewKeyName(gocui.KeyTab), gocui.NewKeyStrMod("a", gocui.ModCtrl), gocui.NewKeyStrMod("k", gocui.ModCtrl), gocui.NewKeyRune('x'), gocui.NewKeyName(gocui.MouseWheelDown)} {
		app.capturing = true
		if err := app.captureShortcut(key); err != nil {
			t.Fatal(err)
		}
		if !app.capturing || !app.shortcutDraft[0].keys[0].Equals(action) || app.shortcutMessage == "" {
			t.Fatal("invalid or conflicting shortcut was accepted")
		}
	}
	if err := app.captureShortcut(gocui.NewKeyName(gocui.KeyF6)); err != nil {
		t.Fatal(err)
	}
	if app.capturing || !app.shortcuts[0].keys[0].Equals(action) {
		t.Fatal("draft remapping mutated active shortcuts")
	}
	app.closeDialog()
	if !app.shortcuts[0].keys[0].Equals(action) {
		t.Fatal("Cancel applied the draft")
	}
	app.openShortcuts(app.gui, nil)
	app.capturing = true
	app.captureShortcut(gocui.NewKeyName(gocui.KeyF6))
	if err := app.confirmQuit(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	if app.dialog != "" || !app.shortcuts[0].keys[0].Equals(gocui.NewKeyName(gocui.KeyF6)) || !strings.Contains(app.shortcutFooter(), "F6 Send") {
		t.Fatal("Apply did not update active shortcuts and footer")
	}
	app.openShortcuts(app.gui, nil)
	app.resetShortcuts(app.gui, nil)
	if !app.shortcuts[0].keys[0].Equals(gocui.NewKeyName(gocui.KeyF6)) || !app.shortcutDraft[0].keys[0].Equals(action) {
		t.Fatal("Reset changed active bindings before Apply")
	}
	app.confirmQuit(app.gui, nil)
	if !app.shortcuts[0].keys[0].Equals(action) {
		t.Fatal("reset defaults did not apply")
	}
}

func TestThemeSurfaces(t *testing.T) {
	app := testApp(t, 100, 28)
	for _, expected := range []struct {
		name   string
		color  gocui.Attribute
		framed bool
	}{{"send", colors.green, false}, {"save", colors.blue, false}, {"new", colors.border, false}, {"body", colors.editor, true}, {"response", colors.surface, true}} {
		view, err := app.gui.View(expected.name)
		if err != nil || view.BgColor != expected.color || view.Frame != expected.framed {
			t.Fatalf("%s surface not styled correctly: %v", expected.name, err)
		}
		if expected.name == "send" || expected.name == "save" {
			app.gui.ForceFlushViewsContentOnly([]*gocui.View{view})
		}
	}
	_, send, _ := gocui.Screen.Get(85, 3)
	_, save, _ := gocui.Screen.Get(93, 3)
	if send.GetBackground() != tcell.NewRGBColor(35, 134, 54) || save.GetBackground() != tcell.NewRGBColor(31, 111, 235) {
		t.Fatal("button fills did not render")
	}
	for _, name := range []string{"new", "send", "save", "method"} {
		view, _ := app.gui.View(name)
		lines := strings.Split(view.Buffer(), "\n")
		if view.InnerHeight() != 3 || len(lines) < 2 || lines[0] != "" || strings.TrimSpace(lines[1]) == "" || !strings.HasPrefix(lines[1], " ") {
			t.Fatalf("%s lacks balanced control padding: %q", name, view.Buffer())
		}
	}
}

func TestMethodDropdownAndFlatTabs(t *testing.T) {
	app := testApp(t, 100, 28)
	if err := app.toggleMethodMenu(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	menu, err := app.gui.View("method-menu")
	if err != nil || !menu.Visible || app.gui.CurrentView().Name() != "method-menu" || !strings.Contains(menu.Buffer(), "PATCH") {
		t.Fatal("method dropdown did not open")
	}
	if menu.InnerHeight() != len(methods)+2 || !strings.HasPrefix(menu.Buffer(), "\n  GET") || menu.CursorY() != app.method+1 {
		t.Fatalf("dropdown padding or selection offset is incorrect: %q", menu.Buffer())
	}
	app.methodOption = 4
	if err := app.chooseMethod(app.gui, menu); err != nil {
		t.Fatal(err)
	}
	if app.method != 4 || app.methodOpen || menu.Visible || app.gui.CurrentView().Name() != "method" {
		t.Fatal("dropdown choice did not commit and close")
	}
	if err := app.toggleMethodMenu(app.gui, nil); err != nil {
		t.Fatal(err)
	}
	app.methodOption = 0
	app.closeMethodMenu()
	if app.method != 4 {
		t.Fatal("dismissing menu changed the method")
	}
	tabs, _ := app.gui.View("tabs")
	if tabs.Frame || strings.Contains(tabs.Buffer(), "[") || !strings.Contains(tabs.Buffer(), "\u2501") {
		t.Fatal("tabs are not flat with an underline indicator")
	}
	if tabAt(2) != 0 || tabAt(len(tabNames[0])+6) != 1 || tabAt(500) != -1 {
		t.Fatal("tab hit areas do not match their labels")
	}
}

func TestRefinedLayoutAcrossTerminalSizes(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 22}, {120, 36}, {200, 45}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			app := testApp(t, size.width, size.height)
			for _, name := range app.focusOrder() {
				view, err := app.gui.View(name)
				if err != nil || view.InnerWidth() < 1 || view.InnerHeight() < 1 {
					t.Fatalf("invalid %s view: %v", name, err)
				}
				if name == "send" || name == "save" || name == "new" {
					if len(strings.TrimSpace(view.Buffer())) > view.InnerWidth() {
						t.Fatalf("%s label overflows", name)
					}
				}
			}
			if err := app.toggleMethodMenu(app.gui, nil); err != nil {
				t.Fatal(err)
			}
			menu, _ := app.gui.View("method-menu")
			if menu.InnerHeight() < len(methods) {
				t.Fatal("dropdown options do not fit")
			}
		})
	}
}

func TestEventLoopWorkflow(t *testing.T) {
	requestStarted := make(chan struct{}, 1)
	releaseResponse := make(chan struct{})
	endpoint := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestStarted <- struct{}{}
		select {
		case <-releaseResponse:
		case <-request.Context().Done():
			return
		}
		writer.Header().Set("X-Response", "visible")
		writer.Write([]byte(`{"worked":true}`))
	}))
	defer endpoint.Close()
	store := goman.Storage{Dir: filepath.Join(t.TempDir(), ".goman")}
	ready := make(chan *App, 1)
	done := make(chan error, 1)
	go func() {
		gui, err := gocui.NewGui(gocui.NewGuiOpts{OutputMode: gocui.OutputTrue, Headless: true, PlayRecording: true, Width: 120, Height: 36})
		if err != nil {
			done <- err
			return
		}
		app := New(store, goman.DefaultCollections(), goman.NewClient(10*time.Second, false), "127.0.0.1:0")
		app.setup(gui)
		if err := app.layout(gui); err != nil {
			gui.Close()
			app.Close()
			done <- err
			return
		}
		address, _ := gui.View("url")
		setEditor(address, endpoint.URL)
		ready <- app
		err = gui.MainLoop()
		app.Close()
		gui.Close()
		done <- err
	}()
	var app *App
	select {
	case app = <-ready:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("event loop did not start")
	}
	finished := false
	t.Cleanup(func() {
		if finished {
			return
		}
		app.gui.Update(func(*gocui.Gui) error { return gocui.ErrQuit })
		select {
		case err := <-done:
			if !errors.Is(err, gocui.ErrQuit) {
				t.Errorf("event loop: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("event loop did not stop")
		}
	})
	press := func(key string) {
		t.Helper()
		app.gui.ReplayKeyEvent(gocui.NewTcellKeyEventWrapper(tcell.NewEventKey(tcell.KeyRune, key, tcell.ModCtrl)))
		app.gui.WaitUntilIdle()
	}
	wait := func(check func() bool) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		state := ""
		for {
			result := make(chan struct {
				ok    bool
				state string
			}, 1)
			app.gui.Update(func(*gocui.Gui) error {
				result <- struct {
					ok    bool
					state string
				}{check(), fmt.Sprintf("method=%d option=%d open=%t focus=%s tab=%d dialog=%s recording=%t message=%q", app.method, app.methodOption, app.methodOpen, app.gui.CurrentView().Name(), app.tab, app.dialog, app.capturing, app.shortcutMessage)}
				return nil
			})
			select {
			case observed := <-result:
				state = observed.state
				if observed.ok {
					app.gui.WaitUntilIdle()
					return
				}
			case <-ctx.Done():
				t.Fatalf("workflow did not reach expected state: %s", state)
			}
		}
	}
	wait(func() bool {
		var screen strings.Builder
		for row := range 36 {
			for column := range 120 {
				text, _, _ := gocui.Screen.Get(column, row)
				screen.WriteString(text)
			}
			screen.WriteByte('\n')
		}
		return strings.Contains(screen.String(), "GOMAN") && strings.Contains(screen.String(), "+ NEW") && strings.Contains(screen.String(), "COLLECTIONS")
	})
	mouse := func(x, y int, button tcell.ButtonMask) {
		t.Helper()
		app.gui.ReplayMouseEvent(gocui.NewTcellMouseEventWrapper(tcell.NewEventMouse(x, y, button, tcell.ModNone)))
		app.gui.WaitUntilIdle()
	}
	navigation := func(key tcell.Key) {
		t.Helper()
		app.gui.ReplayKeyEvent(gocui.NewTcellKeyEventWrapper(tcell.NewEventKey(key, "", tcell.ModNone)))
		app.gui.WaitUntilIdle()
	}
	app.gui.Update(func(*gocui.Gui) error { app.gui.IsPasting = true; return nil })
	app.gui.WaitUntilIdle()
	navigation(tcell.KeyEnter)
	press("s")
	wait(func() bool {
		address, _ := app.gui.View("url")
		return !app.busy && address.TextArea.GetContent() == endpoint.URL+"\n"
	})
	select {
	case <-requestStarted:
		t.Fatal("bracketed URL paste sent a request")
	default:
	}
	app.gui.Update(func(*gocui.Gui) error {
		app.gui.IsPasting = false
		address, _ := app.gui.View("url")
		setEditor(address, endpoint.URL)
		return nil
	})
	app.gui.WaitUntilIdle()
	escapeCancel := func() {
		t.Helper()
		navigation(tcell.KeyEscape)
		wait(func() bool { return app.dialog == "quit" })
		navigation(tcell.KeyEscape)
		wait(func() bool { return app.dialog == "" })
	}
	mouse(34, 3, tcell.Button1)
	mouse(34, 3, tcell.ButtonNone)
	wait(func() bool { return app.methodOpen && app.gui.CurrentView().Name() == "method-menu" })
	navigation(tcell.KeyDown)
	navigation(tcell.KeyEnter)
	wait(func() bool { return !app.methodOpen && app.method == 1 })
	mouse(34, 3, tcell.Button1)
	mouse(34, 3, tcell.ButtonNone)
	mouse(34, 7, tcell.WheelUp)
	mouse(34, 6, tcell.Button1)
	mouse(34, 6, tcell.ButtonNone)
	wait(func() bool { return app.methodOpen && app.method == 1 })
	mouse(34, 7, tcell.Button1)
	mouse(34, 7, tcell.ButtonNone)
	wait(func() bool { return !app.methodOpen && app.method == 0 })
	mouse(34, 3, tcell.Button1)
	mouse(34, 3, tcell.ButtonNone)
	mouse(60, 14, tcell.Button1)
	mouse(60, 14, tcell.ButtonNone)
	wait(func() bool {
		return !app.methodOpen && app.gui.CurrentView().Name() == "method" && app.selection.field == ""
	})
	mouse(52, 6, tcell.Button1)
	mouse(52, 6, tcell.ButtonNone)
	wait(func() bool { return app.tab == 1 })
	mouse(34, 6, tcell.Button1)
	mouse(34, 6, tcell.ButtonNone)
	wait(func() bool { return app.tab == 0 })
	mouse(34, 14, tcell.Button1)
	mouse(34, 14, tcell.ButtonNone)
	navigation(tcell.KeyEscape)
	wait(func() bool { return app.dialog == "quit" && app.tab == 0 })
	press("s")
	mouse(104, 3, tcell.Button1)
	mouse(104, 3, tcell.ButtonNone)
	wait(func() bool { return app.dialog == "quit" && !app.busy && app.response.StatusCode == 0 })
	navigation(tcell.KeyTab)
	wait(func() bool { return app.gui.CurrentView().Name() == "dialog-cancel" })
	navigation(tcell.KeyTab)
	navigation(tcell.KeyEscape)
	wait(func() bool { return app.dialog == "" && app.tab == 0 && app.gui.CurrentView().Name() == "body" })
	press("q")
	wait(func() bool { return app.dialog == "quit" && app.gui.CurrentView().Name() == "dialog-primary" })
	navigation(tcell.KeyEscape)
	wait(func() bool { return app.dialog == "" })
	app.gui.Update(func(gui *gocui.Gui) error {
		_, err := gui.SetCurrentView("body")
		return err
	})
	app.gui.WaitUntilIdle()
	bracket := func(value string) {
		t.Helper()
		app.gui.ReplayKeyEvent(gocui.NewTcellKeyEventWrapper(tcell.NewEventKey(tcell.KeyRune, value, tcell.ModCtrl)))
		app.gui.WaitUntilIdle()
	}
	for _, expected := range []int{1, 2, 0} {
		bracket("]")
		wait(func() bool { return app.tab == expected })
	}
	bracket("[")
	wait(func() bool { return app.tab == 2 })
	escapeCancel()
	wait(func() bool { return app.tab == 2 })
	bracket("[")
	wait(func() bool { return app.tab == 1 })
	bracket("[")
	wait(func() bool {
		url, _ := app.gui.View("url")
		body, _ := app.gui.View("body")
		return app.tab == 0 && url.TextArea.GetContent() == endpoint.URL && body.TextArea.GetContent() == "{\n  \"hello\": \"world\"\n}"
	})
	app.gui.Update(func(gui *gocui.Gui) error {
		body, _ := gui.View("body")
		setEditor(body, "first\nsecond")
		_, err := gui.SetCurrentView("body")
		return err
	})
	app.gui.WaitUntilIdle()
	press("a")
	app.gui.ReplayKeyEvent(gocui.NewTcellKeyEventWrapper(tcell.NewEventKey(tcell.KeyRune, "X", tcell.ModNone)))
	app.gui.WaitUntilIdle()
	wait(func() bool {
		body, _ := app.gui.View("body")
		return body.TextArea.GetContent() == "first\nXsecond" && app.selectedText("body") == ""
	})
	press("g")
	wait(func() bool {
		_, style, _ := gocui.Screen.Get(32, 14)
		return app.selectedText("body") == "first\nXsecond" && app.selectedText("url") == "" && style.HasReverse()
	})
	for _, value := range []string{"a", "A"} {
		escapeCancel()
		wait(func() bool { return app.selection.field == "" && app.tab == 0 })
		app.gui.ReplayKeyEvent(gocui.NewTcellKeyEventWrapper(tcell.NewEventKey(tcell.KeyRune, value, tcell.ModCtrl|tcell.ModShift)))
		app.gui.WaitUntilIdle()
		wait(func() bool { return app.selectedText("body") == "first\nXsecond" })
	}
	escapeCancel()
	wait(func() bool {
		_, style, _ := gocui.Screen.Get(32, 14)
		return app.selection.field == "" && app.tab == 0 && !style.HasReverse()
	})
	navigation(tcell.KeyF2)
	wait(func() bool { return app.dialog == "keys" && app.gui.CurrentView().Name() == "dialog-shortcuts" })
	navigation(tcell.KeyEnter)
	press("k")
	wait(func() bool { return app.capturing && strings.Contains(app.shortcutMessage, "Save") && !app.saving })
	navigation(tcell.KeyTab)
	wait(func() bool { return app.capturing && strings.Contains(app.shortcutMessage, "reserved") })
	navigation(tcell.KeyEnter)
	wait(func() bool { return app.capturing && strings.Contains(app.shortcutMessage, "reserved") })
	navigation(tcell.KeyEscape)
	wait(func() bool { return !app.capturing && app.dialog == "keys" })
	navigation(tcell.KeyEnter)
	navigation(tcell.KeyF6)
	wait(func() bool {
		return !app.capturing && app.shortcutDraft[0].keys[0].Equals(gocui.NewKeyName(gocui.KeyF6))
	})
	navigation(tcell.KeyTab)
	navigation(tcell.KeyTab)
	navigation(tcell.KeyEnter)
	wait(func() bool { return app.dialog == "" && app.shortcuts[0].keys[0].Equals(gocui.NewKeyName(gocui.KeyF6)) })
	press("s")
	wait(func() bool { return !app.busy && app.response.StatusCode == 0 && app.tab == 0 })
	navigation(tcell.KeyF2)
	navigation(tcell.KeyEnter)
	press("u")
	wait(func() bool {
		return !app.capturing && app.shortcutDraft[0].keys[0].Equals(gocui.NewKeyStrMod("u", gocui.ModCtrl))
	})
	navigation(tcell.KeyTab)
	navigation(tcell.KeyTab)
	navigation(tcell.KeyEnter)
	wait(func() bool {
		return app.dialog == "" && app.shortcuts[0].keys[0].Equals(gocui.NewKeyStrMod("u", gocui.ModCtrl))
	})
	app.gui.Update(func(gui *gocui.Gui) error {
		if _, err := gui.SetCurrentView("url"); err != nil {
			return err
		}
		body, _ := gui.View("body")
		setEditor(body, "alpha beta\nsecond line\n"+strings.Repeat(strings.Repeat("x", 100)+"\n", 40))
		body.SetOrigin(0, 0)
		return nil
	})
	app.gui.WaitUntilIdle()
	mouse(33, 15, tcell.WheelDown)
	wait(func() bool {
		body, _ := app.gui.View("body")
		return body.OriginY() == 3 && app.gui.CurrentView().Name() == "url"
	})
	mouse(33, 15, tcell.WheelUp)
	mouse(33, 15, tcell.WheelRight)
	wait(func() bool { body, _ := app.gui.View("body"); return body.OriginX() == 3 && body.OriginY() == 0 })
	mouse(33, 15, tcell.WheelLeft)
	mouse(32, 14, tcell.Button1)
	mouse(37, 14, tcell.Button1)
	mouse(37, 14, tcell.ButtonNone)
	wait(func() bool {
		return app.selectedText("body") == "alpha" && app.selectedText("url") == "" && !app.selection.dragging
	})
	wait(func() bool {
		_, selected, _ := gocui.Screen.Get(32, 14)
		_, outside, _ := gocui.Screen.Get(37, 14)
		return selected.HasReverse() && !outside.HasReverse()
	})
	mouse(32, 14, tcell.Button1)
	mouse(104, 3, tcell.Button1)
	mouse(104, 3, tcell.ButtonNone)
	wait(func() bool {
		return app.selectedText("body") == "alpha beta" && app.gui.CurrentView().Name() == "body" && !app.busy && app.tab == 0
	})
	press("u")
	select {
	case <-requestStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP fixture did not receive the request")
	}
	app.webhook.OnHook(goman.Hook{Path: "/during-request"})
	wait(func() bool {
		metrics, _ := app.gui.View("metrics")
		return app.busy && app.status == "Hook received: /during-request" && app.notice == app.status && strings.TrimSuffix(metrics.Buffer(), "\n") == "Sending GET to "+endpoint.URL
	})
	close(releaseResponse)
	wait(func() bool {
		return !app.busy && app.tab == 1 && app.response.StatusCode == 200 && strings.Contains(app.response.Body, "worked")
	})
	wait(func() bool {
		view, _ := app.gui.View("response")
		return strings.Contains(view.Buffer(), "X-Response: visible")
	})
	mouse(32, 12, tcell.Button1)
	mouse(37, 12, tcell.Button1)
	mouse(37, 12, tcell.ButtonNone)
	wait(func() bool {
		view, _ := app.gui.View("response")
		text := fieldText(view)
		return len(text) >= 5 && app.selectedText("response") == text[:5] && app.selectedText("body") == ""
	})
	press("g")
	wait(func() bool {
		view, _ := app.gui.View("response")
		return app.selectedText("response") == fieldText(view) && app.selectedText("body") == ""
	})
	press("k")
	wait(func() bool { return !app.saving && strings.HasPrefix(app.status, "Saved:") })
	collections, err := store.Load()
	if err != nil || len(collections[0].Requests) != 4 || collections[0].Requests[3].URL != endpoint.URL {
		t.Fatalf("saved request: %v %v", collections, err)
	}
	press("w")
	wait(func() bool { return !app.toggling && app.webhook.Running() })
	response, err := http.Post("http://"+app.webhook.Address()+"/live", "application/json", strings.NewReader(`{"live":true}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	wait(func() bool { return len(app.webhook.Hooks()) == 1 })
	bracket("]")
	wait(func() bool {
		view, _ := app.gui.View("hooks")
		return app.tab == 2 && strings.Contains(view.Buffer(), "->") && strings.Contains(view.Buffer(), `"live"`)
	})
	app.gui.Update(func(gui *gocui.Gui) error {
		body, _ := gui.View("body")
		setEditor(body, "unsaved editor text")
		return nil
	})
	app.gui.WaitUntilIdle()
	for _, point := range []struct{ x, y int }{{33, 11}, {31, 12}, {119, 12}, {33, 16}, {33, 14}} {
		mouse(point.x, point.y, tcell.Button1)
		mouse(point.x, point.y, tcell.ButtonNone)
		wait(func() bool {
			request, err := app.currentRequest()
			return err == nil && app.tab == 2 && request.URL == endpoint.URL && request.Body == "unsaved editor text" && app.hookSelected == 0
		})
	}
	mouse(33, 12, tcell.Button1)
	mouse(33, 12, tcell.ButtonNone)
	wait(func() bool {
		request, err := app.currentRequest()
		return err == nil && app.tab == 0 && request.URL == endpoint.URL && request.Method == "POST" && request.Body == `{"live":true}` && request.Headers["Content-Type"] == "application/json" && app.gui.CurrentView().Name() == "body"
	})
	app.gui.Update(func(*gocui.Gui) error {
		app.notify("Older message", noticeInfo, 20*time.Millisecond)
		app.notify("Expires shortly", noticeWarning, 100*time.Millisecond)
		return nil
	})
	wait(func() bool {
		view, err := app.gui.View("notification")
		return err == nil && view.Visible && app.notice == "Expires shortly" && view.FrameColor == noticeColor(noticeWarning) && app.gui.CurrentView().Name() == "body"
	})
	wait(func() bool {
		view, _ := app.gui.View("notification")
		return app.notice == "" && !view.Visible && app.gui.CurrentView().Name() == "body"
	})
	press("w")
	wait(func() bool { return !app.toggling && !app.webhook.Running() })
	press("n")
	wait(func() bool { view, _ := app.gui.View("url"); return view.TextArea.GetContent() == "" && app.tab == 0 })
	navigation(tcell.KeyEscape)
	wait(func() bool { return app.dialog == "quit" })
	navigation(tcell.KeyTab)
	app.gui.ReplayKeyEvent(gocui.NewTcellKeyEventWrapper(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone)))
	select {
	case err := <-done:
		finished = true
		if !errors.Is(err, gocui.ErrQuit) {
			t.Fatalf("confirmed quit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("confirmed quit did not finish the event loop")
	}
}

func TestLargePreviewPreservesSource(t *testing.T) {
	app := testApp(t, 80, 22)
	body := strings.Repeat("x", maxPreviewBytes-1) + "\u2603" + strings.Repeat("x", 100)
	app.response = goman.Response{Body: body}
	if err := app.switchTab(1); err != nil {
		t.Fatal(err)
	}
	view, _ := app.gui.View("response")
	if !strings.Contains(view.Buffer(), "Preview truncated") || len(view.Buffer()) > maxPreviewBytes+100 || app.response.Body != body {
		t.Fatal("preview limit lost source data or failed to limit rendering")
	}
	for _, name := range app.focusOrder() {
		view, err := app.gui.View(name)
		if err != nil || view.InnerWidth() < 1 || view.InnerHeight() < 1 {
			t.Fatalf("invalid minimum-terminal view %s: %v", name, err)
		}
	}
}

func TestScrollStaysWithinField(t *testing.T) {
	app := testApp(t, 100, 28)
	body, _ := app.gui.View("body")
	address, _ := app.gui.View("url")
	text := strings.Repeat(strings.Repeat("x", 100)+"\n", 40)
	setEditor(body, text)
	body.SetOrigin(0, 0)
	address.SetOrigin(0, 0)
	app.scroll(body, 3, 3)
	if body.OriginX() != 3 || body.OriginY() != 3 || address.OriginX() != 0 || address.OriginY() != 0 {
		t.Fatal("scroll moved a neighboring field or did not move the body")
	}
	app.scroll(body, -1000, -1000)
	if body.OriginX() != 0 || body.OriginY() != 0 {
		t.Fatal("scroll escaped the beginning of the field")
	}
	app.scroll(body, 1000, 1000)
	if body.OriginX() > 100-body.InnerWidth() || body.OriginY() > len(body.BufferLines())-body.InnerHeight() || body.TextArea.GetContent() != text {
		t.Fatal("scroll escaped the end of the field or modified its value")
	}
}

func TestSelectionIsFieldLocal(t *testing.T) {
	app := testApp(t, 100, 28)
	body, _ := app.gui.View("body")
	address, _ := app.gui.View("url")
	text := "before \u2603e\u0301 after\nsecond line"
	setEditor(body, text)
	start := textOffset(text, 7, 0)
	end := textOffset(text, 9, 0)
	app.selection = textSelection{field: "body", text: text, anchor: start, end: end}
	app.renderSelection(body)
	if app.selectedText("body") != "\u2603e\u0301" || app.selectedText("url") != "" || address.TextArea.GetContent() == text {
		t.Fatal("selection crossed fields or split a Unicode character")
	}
	if !body.Editor.Edit(body, gocui.NewKeyRune('X')) || body.TextArea.GetContent() != "before X after\nsecond line" {
		t.Fatal("typing did not replace only the selected value")
	}
	app.selection = textSelection{field: "body", text: body.TextArea.GetContent(), anchor: 7, end: 8}
	app.renderSelection(body)
	if !body.Editor.Edit(body, gocui.NewKeyName(gocui.KeyDelete)) || body.TextArea.GetContent() != "before  after\nsecond line" {
		t.Fatal("delete did not remove only the selected value")
	}
	if textOffset(text, 100, 0) != len("before \u2603e\u0301 after") || textOffset(text, 0, 100) != len(text) {
		t.Fatal("selection offsets escaped field content")
	}
	if textOffset("\tvalue", 4, 0) != 1 || textOffset("\u754cvalue", 1, 0) != 0 || textOffset("\u754cvalue", 2, 0) != 3 {
		t.Fatal("selection did not respect tabs or wide characters")
	}
}

func TestSelectAllIsFieldLocal(t *testing.T) {
	app := testApp(t, 100, 28)
	for _, name := range []string{"url", "headers", "body", "response", "hook-detail"} {
		t.Run(name, func(t *testing.T) {
			view, _ := app.gui.View(name)
			view.Visible = true
			text := "first \u754c e\u0301\n" + strings.Repeat("more text\n", 40)
			if view.Editable {
				setEditor(view, text)
			} else {
				view.SetContent(text)
				text = fieldText(view)
			}
			view.SetOrigin(0, 3)
			if err := app.selectAll(app.gui, view); err != nil {
				t.Fatal(err)
			}
			if app.selectedText(name) != text || app.selectedText("collections") != "" || fieldText(view) != text || view.OriginY() != 3 {
				t.Fatal("select all changed the value, viewport, or selected another field")
			}
			app.gui.ForceFlushViewsContentOnly([]*gocui.View{view})
			app.clearSelection()
			if fieldText(view) != text {
				t.Fatal("clearing select all changed field text")
			}
			if view.Editable {
				app.selectAll(app.gui, view)
				view.Editor.Edit(view, gocui.NewKeyRune('X'))
				if fieldText(view) != "X" {
					t.Fatal("typing did not replace the full selection")
				}
				app.selectAll(app.gui, view)
				view.Editor.Edit(view, gocui.NewKeyName(gocui.KeyDelete))
				if fieldText(view) != "" {
					t.Fatal("delete did not remove the full selection")
				}
				app.selectAll(app.gui, view)
				if app.selection.field != "" {
					t.Fatal("empty field created a selection")
				}
			}
		})
	}
}

func TestSyntaxPreservesValuesAndSelection(t *testing.T) {
	app := testApp(t, 100, 28)
	body, _ := app.gui.View("body")
	text := "{\n  \"name\": \"world\",\n  \"count\": 42,\n  \"enabled\": true,\n  \"empty\": null\n}\n"
	setEditor(body, text)
	if body.TextArea.GetContent() != text || strings.TrimSuffix(body.Buffer(), "\n") != strings.TrimSuffix(text, "\n") {
		t.Fatal("highlighting changed the payload value")
	}
	seen := map[string]bool{}
	for _, token := range syntaxTokens("body", text) {
		if tokenColor(token.Type) != "" {
			seen[token.Type.String()] = true
		}
	}
	if !seen[chroma.NameTag.String()] || !seen[chroma.LiteralStringDouble.String()] || !seen[chroma.LiteralNumberInteger.String()] || !seen[chroma.KeywordConstant.String()] {
		t.Fatalf("missing JSON token colors: %v", seen)
	}
	start := strings.Index(text, "world")
	app.selection = textSelection{field: "body", text: text, anchor: start, end: start + 5}
	app.renderSelection(body)
	if app.selectedText("body") != "world" {
		t.Fatal("color codes leaked into selected text")
	}
	app.clearSelection()
	if body.TextArea.GetContent() != text {
		t.Fatal("clearing selection changed payload")
	}
	if !body.Editor.Edit(body, gocui.NewKeyRune(' ')) || body.TextArea.GetContent() != text+" " {
		t.Fatal("highlighting broke editing")
	}
	for _, fixture := range []struct{ field, text string }{{"body", `{"incomplete":`}, {"response", "Content-Type: application/xml\n\n<root value=\"yes\">42</root>"}, {"response", "Content-Type: application/yaml\n\nname: world\ncount: 42"}, {"hook-detail", "POST /hook\nContent-Type: application/json\n\n{\"ok\":true}"}, {"response", "Content-Type: text/plain\n\nplain text"}} {
		var source strings.Builder
		for _, token := range syntaxTokens(fixture.field, fixture.text) {
			source.WriteString(token.Value)
		}
		if source.String() != fixture.text {
			t.Fatalf("tokens changed %s source", fixture.field)
		}
	}
}

func TestRenderedSyntaxColors(t *testing.T) {
	app := testApp(t, 100, 28)
	body, _ := app.gui.View("body")
	text := `{"key":"value","n":42,"ok":true}`
	setEditor(body, text)
	body.SetOrigin(0, 0)
	app.gui.ForceFlushViewsContentOnly([]*gocui.View{body})
	for _, expected := range []struct {
		value string
		color tcell.Color
	}{{"key", tcell.NewRGBColor(100, 200, 240)}, {"value", tcell.NewRGBColor(140, 210, 150)}, {"42", tcell.NewRGBColor(240, 190, 100)}, {"true", tcell.NewRGBColor(230, 140, 180)}} {
		_, style, _ := gocui.Screen.Get(27+strings.Index(text, expected.value), 14)
		if style.GetForeground() != expected.color {
			t.Fatalf("%s color = %v, want %v", expected.value, style.GetForeground(), expected.color)
		}
	}
	start := strings.Index(text, "value")
	app.selection = textSelection{field: "body", text: text, anchor: start, end: start + 5}
	app.renderSelection(body)
	app.gui.ForceFlushViewsContentOnly([]*gocui.View{body})
	_, style, _ := gocui.Screen.Get(27+start, 14)
	if !style.HasReverse() || style.GetForeground() != tcell.NewRGBColor(140, 210, 150) {
		t.Fatal("selection replaced syntax color or lost its highlight")
	}
	app.clearSelection()
	app.gui.ForceFlushViewsContentOnly([]*gocui.View{body})
	_, style, _ = gocui.Screen.Get(27+start, 14)
	if style.HasReverse() || style.GetForeground() != tcell.NewRGBColor(140, 210, 150) {
		t.Fatal("clearing selection did not restore syntax color")
	}
	large := `{"value":"` + strings.Repeat("x", maxHighlightBytes) + `"}`
	var source strings.Builder
	for _, token := range syntaxTokens("body", large) {
		source.WriteString(token.Value)
	}
	if source.String() != large {
		t.Fatal("bounded highlighting truncated the source")
	}
}
