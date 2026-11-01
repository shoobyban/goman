package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
	"goman/internal/gocui"
	"goman/internal/goman"
)

var methods = []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
var tabNames = []string{"Body & Payload", "Response", "Live Webhooks"}
var viewNames = []string{"canvas", "sidebar", "sidebar-title", "sidebar-rule", "header-rule", "header", "new", "collections", "method", "method-menu", "url", "send", "save", "tabs", "headers", "body", "metrics", "response", "receiver", "hooks", "hook-detail", "footer", "notification", "dialog", "dialog-primary", "dialog-cancel", "dialog-reset", "dialog-shortcuts"}

type App struct {
	store           goman.Storage
	collections     []goman.Collection
	client          *http.Client
	webhook         *goman.WebhookServer
	address         string
	gui             *gocui.Gui
	ctx             context.Context
	cancel          context.CancelFunc
	workers         sync.WaitGroup
	method          int
	customMethod    string
	methodOpen      bool
	methodOption    int
	selected        int
	hookSelected    int
	tab             int
	busy            bool
	toggling        bool
	saving          bool
	status          string
	requestProgress string
	response        goman.Response
	content         map[string]string
	selection       textSelection
	dialog          string
	dialogChoice    int
	dialogReturn    string
	shortcuts       []shortcut
	shortcutDraft   []shortcut
	shortcutIndex   int
	capturing       bool
	shortcutMessage string
	statusLevel     noticeLevel
	notice          string
	noticeLevel     noticeLevel
	noticeID        uint64
	noticeCancel    context.CancelFunc
}

func New(store goman.Storage, collections []goman.Collection, client *http.Client, address string) *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{store: store, collections: collections, client: client, address: address, webhook: &goman.WebhookServer{}, ctx: ctx, cancel: cancel, status: "Ready", content: map[string]string{}, shortcuts: defaultShortcuts()}
}

func (a *App) Run() error {
	gui, err := gocui.NewGui(gocui.NewGuiOpts{OutputMode: gocui.OutputTrue})
	if err != nil {
		a.cancel()
		return err
	}
	a.setup(gui)
	defer gui.Close()
	defer a.Close()
	err = gui.MainLoop()
	if errors.Is(err, gocui.ErrQuit) {
		return nil
	}
	return err
}

func (a *App) Close() {
	a.cancel()
	a.workers.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.webhook.Stop(ctx); err != nil {
		a.status = err.Error()
	}
	a.client.CloseIdleConnections()
}

func (a *App) setup(gui *gocui.Gui) {
	a.gui = gui
	gui.Mouse = true
	gui.FgColor = colors.text
	gui.BgColor = colors.background
	gui.FrameColor = colors.border
	gui.SelFgColor = colors.text
	gui.SelBgColor = colors.blue
	gui.SelFrameColor = colors.accent
	gui.ShouldHandleMouseEvent = func(view *gocui.View, key gocui.KeyName) bool {
		if a.dialog != "" {
			return strings.HasPrefix(view.Name(), "dialog")
		}
		if a.methodOpen && view.Name() != "method-menu" && view.Name() != "method" {
			a.closeMethodMenu()
			return false
		}
		return true
	}
	gui.SetManagerFunc(a.layout)
	a.webhook.OnHook = func(hook goman.Hook) {
		gui.Update(func(*gocui.Gui) error { a.feedback("Hook received: "+hook.Path, noticeInfo); return nil })
	}
	a.bindKeys()
}

func (a *App) bindKeys() {
	gui := a.gui
	gui.DeleteAllKeybindings()
	a.bindShortcuts()
	gui.SetKeybinding("", gocui.NewKeyName(gocui.KeyTab), func(*gocui.Gui, *gocui.View) error { return a.cycleFocus(1) })
	gui.SetKeybinding("", gocui.NewKeyName(gocui.KeyBacktab), func(*gocui.Gui, *gocui.View) error { return a.cycleFocus(-1) })
	gui.SetKeybinding("", gocui.NewKey(gocui.KeyTab, "", gocui.ModShift), func(*gocui.Gui, *gocui.View) error { return a.cycleFocus(-1) })
	gui.SetKeybinding("", gocui.NewKeyName(gocui.KeyEsc), a.escape)
	gui.SetKeybinding("", gocui.NewKeyName(gocui.KeyEnter), func(*gocui.Gui, *gocui.View) error {
		if !a.usable() && a.dialog == "quit" {
			return gocui.ErrQuit
		}
		return nil
	})
	for name, handler := range map[string]func(*gocui.Gui, *gocui.View) error{"new": a.newRequest, "send": a.send, "save": a.save, "receiver": a.toggleWebhook, "collections": a.loadSelected, "hooks": a.replay, "method": a.toggleMethodMenu} {
		gui.SetKeybinding(name, gocui.NewKeyName(gocui.KeyEnter), handler)
	}
	gui.SetKeybinding("url", gocui.NewKeyName(gocui.KeyEnter), a.send)
	a.bindMethodMenu()
	for _, name := range []string{"method", "tabs"} {
		gui.SetKeybinding(name, gocui.NewKeyName(gocui.KeyArrowRight), func(*gocui.Gui, *gocui.View) error {
			if name == "method" {
				return a.nextMethod(gui, nil)
			}
			return a.switchTab((a.tab + 1) % 3)
		})
		gui.SetKeybinding(name, gocui.NewKeyName(gocui.KeyArrowLeft), func(*gocui.Gui, *gocui.View) error {
			if name == "method" {
				a.method = (a.method + len(methods) - 1) % len(methods)
				a.customMethod = ""
				return nil
			}
			return a.switchTab((a.tab + 2) % 3)
		})
	}
	gui.SetKeybinding("tabs", gocui.NewKeyName(gocui.KeyEnter), func(*gocui.Gui, *gocui.View) error { return a.switchTab((a.tab + 1) % 3) })
	for _, name := range []string{"collections", "hooks", "response", "hook-detail"} {
		for _, movement := range []struct {
			key   gocui.KeyName
			delta int
		}{{gocui.KeyArrowUp, -1}, {gocui.KeyArrowDown, 1}, {gocui.KeyPgup, -10}, {gocui.KeyPgdn, 10}} {
			gui.SetKeybinding(name, gocui.NewKeyName(movement.key), func(_ *gocui.Gui, view *gocui.View) error { a.move(name, view, movement.delta); return nil })
		}
	}
	for _, name := range []string{"collections", "hooks", "url", "headers", "body", "response", "hook-detail"} {
		for _, wheel := range []struct {
			key    gocui.KeyName
			dx, dy int
		}{{gocui.MouseWheelUp, 0, -3}, {gocui.MouseWheelDown, 0, 3}, {gocui.MouseWheelLeft, -3, 0}, {gocui.MouseWheelRight, 3, 0}} {
			gui.SetKeybinding(name, gocui.NewKeyName(wheel.key), func(_ *gocui.Gui, view *gocui.View) error {
				a.scroll(view, wheel.dx, wheel.dy)
				return nil
			})
		}
	}
	for _, name := range viewNames {
		if strings.HasPrefix(name, "dialog") || name == "notification" || name == "hooks" {
			continue
		}
		gui.SetKeybinding(name, gocui.NewKeyName(gocui.MouseLeft), func(gui *gocui.Gui, view *gocui.View) error {
			if view == nil {
				return nil
			}
			a.clearSelection()
			if _, err := gui.SetCurrentView(name); err != nil {
				return err
			}
			switch name {
			case "new":
				return a.newRequest(gui, view)
			case "send":
				return a.send(gui, view)
			case "save":
				return a.save(gui, view)
			case "method":
				return a.toggleMethodMenu(gui, view)
			case "method-menu":
				option := view.OriginY() + view.CursorY() - 1
				if option < 0 || option >= len(methods) {
					return nil
				}
				a.methodOption = option
				return a.chooseMethod(gui, view)
			case "receiver":
				return a.toggleWebhook(gui, view)
			case "tabs":
				if index := tabAt(view.CursorX()); index >= 0 {
					return a.switchTab(index)
				}
				return nil
			case "collections":
				a.selected = view.OriginY() + view.CursorY()
				return a.loadSelected(gui, view)
			}
			return nil
		})
	}
	gui.SetViewClickBinding(&gocui.ViewMouseBinding{ViewName: "hooks", Key: gocui.MouseLeft, Handler: a.clickHook})
	a.setupTextSelection()
	a.bindDialog()
	a.bindShortcutDialog()
	gui.SetKeybinding("notification", gocui.NewKeyName(gocui.MouseLeft), func(*gocui.Gui, *gocui.View) error { a.dismissNotice(); return nil })
}

func (a *App) clickHook(opts gocui.ViewMouseBindingOpts) error {
	view, err := a.gui.View("hooks")
	if err != nil {
		return err
	}
	x, y := opts.X-view.OriginX(), opts.Y-view.OriginY()
	if x < 0 || x >= view.InnerWidth() || y < 0 || y >= view.InnerHeight() || opts.Y < 0 || opts.Y >= len(a.webhook.Hooks()) {
		return nil
	}
	a.clearSelection()
	if _, err := a.gui.SetCurrentView("hooks"); err != nil {
		return err
	}
	a.hookSelected = opts.Y
	return a.replay(a.gui, view)
}

func (a *App) scroll(view *gocui.View, dx, dy int) {
	if dy != 0 && (view.Name() == "collections" || view.Name() == "hooks") {
		a.move(view.Name(), view, dy)
		return
	}
	lines := view.BufferLines()
	width := 0
	for _, line := range lines {
		width = max(width, uniseg.StringWidth(line))
	}
	view.SetOrigin(max(0, min(view.OriginX()+dx, width-view.InnerWidth())), max(0, min(view.OriginY()+dy, len(lines)-view.InnerHeight())))
}

func plain(text string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return -1
		}
		return character
	}, text)
}

const maxPreviewBytes = 128 << 10

func preview(text string) string {
	if len(text) <= maxPreviewBytes {
		return text
	}
	end := maxPreviewBytes
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "\n\n[Preview truncated at 128 KiB]"
}

func (a *App) view(name string, x0, y0, x1, y1 int, title string, editable bool) (*gocui.View, error) {
	view, err := a.gui.SetView(name, x0, y0, x1, y1, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return nil, err
	}
	view.Visible = true
	view.Title = title
	view.Editable = editable
	a.styleView(view)
	if errors.Is(err, gocui.ErrUnknownView) && editable {
		initial := ""
		if name == "url" {
			initial = "https://jsonplaceholder.typicode.com/posts/1"
		}
		if name == "body" {
			initial = "{\n  \"hello\": \"world\"\n}"
		}
		if name == "headers" {
			initial = "{}"
		}
		setEditor(view, initial)
		view.Editor = gocui.EditorFunc(a.editSelection)
	}
	return view, nil
}

func setEditor(view *gocui.View, text string) {
	view.ClearTextArea()
	view.TextArea.TypeCharacter(text)
	view.RenderTextArea()
	renderSyntax(view, text, 0, 0)
}

func (a *App) write(name, text string) {
	text = plain(text)
	if previous, ok := a.content[name]; ok && previous == text {
		return
	}
	if view, err := a.gui.View(name); err == nil {
		if a.selection.field == name {
			a.clearSelection()
		}
		if name == "response" || name == "hook-detail" {
			renderSyntax(view, text, 0, 0)
		} else {
			view.SetContent(text)
		}
		a.content[name] = text
	}
}

func (a *App) layout(gui *gocui.Gui) error {
	width, height := gui.Size()
	for _, name := range viewNames {
		if view, err := gui.View(name); err == nil {
			view.Visible = false
		}
	}
	if width < 80 || height < 22 {
		view, err := gui.SetView("small", -1, -1, width, height, 0)
		if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}
		view.Frame = false
		view.Visible = true
		view.SetContent("goman\nTerminal requires at least 80 columns x 22 rows.\nCtrl+Q to quit.")
		if a.dialog == "quit" {
			view.SetContent("Quit goman?\nEnter to quit | Escape to cancel.")
		}
		gui.Cursor = false
		_, err = gui.SetCurrentView("small")
		return err
	}
	if view, err := gui.View("small"); err == nil {
		view.Visible = false
	}
	if _, err := a.view("canvas", -1, -1, width, height, "", false); err != nil {
		return err
	}
	if _, err := gui.SetViewOnBottom("canvas"); err != nil {
		return err
	}
	sidebar := min(30, width/4)
	left := sidebar + 1
	for _, spec := range []struct {
		name           string
		x0, y0, x1, y1 int
		title          string
		edit           bool
	}{
		{"header", -1, -1, width, 2, "", false},
		{"header-rule", -1, 0, width, 2, "", false},
		{"sidebar", -1, 1, sidebar + 1, height - 3, "", false},
		{"sidebar-title", 0, 2, sidebar - 8, 5, "", false},
		{"sidebar-rule", sidebar, 1, sidebar + 2, height - 3, "", false},
		{"new", sidebar - 8, 1, sidebar, 5, "", false},
		{"collections", 0, 5, sidebar, height - 3, "", false},
		{"method", left, 1, left + 14, 5, "", false},
		{"url", left + 14, 2, width - 26, 5, "", true},
		{"send", width - 24, 1, width - 12, 5, "", false},
		{"save", width - 10, 1, width - 1, 5, "", false},
		{"tabs", left, 5, width - 1, 8, "", false},
		{"headers", left, 8, width - 1, 13, "Headers (JSON)", true},
		{"body", left, 13, width - 1, height - 3, "Body & Payload", true},
		{"metrics", left, 8, width - 1, 11, "Status", false},
		{"response", left, 11, width - 1, height - 3, "Response & Headers", false},
		{"receiver", left, 8, width - 1, 11, "Webhook Receiver", false},
		{"hooks", left, 11, width - 1, 16, "Live Webhooks", false},
		{"hook-detail", left, 16, width - 1, height - 3, "Hook Details (Enter on list to replay)", false},
		{"footer", -1, height - 3, width, height, "", false},
	} {
		view, err := a.view(spec.name, spec.x0, spec.y0, spec.x1, spec.y1, spec.title, spec.edit)
		if err != nil {
			return err
		}
		if spec.name == "header" || spec.name == "footer" {
			view.Frame = false
		}
		switch spec.name {
		case "headers", "body":
			view.Visible = a.tab == 0
		case "metrics", "response":
			view.Visible = a.tab == 1
		case "receiver", "hooks", "hook-detail":
			view.Visible = a.tab == 2
		}
	}
	a.chrome("header", "\x1b[38;2;0;255;159m\x1b[1mGOMAN\x1b[0m  Terminal API & Webhook Studio (Local-First)")
	a.chrome("header-rule", strings.Repeat("\u2500", width))
	a.write("sidebar-title", "COLLECTIONS")
	a.chrome("sidebar-rule", strings.TrimSuffix(strings.Repeat("\u2502\n", height-5), "\n"))
	a.button("new", "+ NEW")
	methodLabel := []rune(a.requestMethod())
	label := string(methodLabel[:min(6, len(methodLabel))])
	a.button("method", "  "+label+strings.Repeat(" ", max(0, len("DELETE")-len(label)))+"  v  ")
	a.button("send", "SEND")
	a.button("save", "SAVE")
	a.drawTabs()
	var rows []string
	for _, request := range a.requests() {
		rows = append(rows, "["+request.Method+"] "+strings.ReplaceAll(plain(request.Name), "\n", " "))
	}
	a.write("collections", strings.Join(rows, "\n"))
	a.highlight("collections", a.selected)
	if a.tab == 1 {
		a.drawMetrics()
		a.write("response", headerText(a.response.Headers)+"\n\n"+preview(a.response.Body))
	}
	if a.tab == 2 {
		receiver := "OFFLINE | " + a.address + " | Enter to start"
		if a.webhook.Running() {
			receiver = "ONLINE | http://" + a.webhook.Address() + " | Enter to stop"
		}
		if err := a.webhook.Err(); err != nil {
			receiver = "Receiver failed: " + err.Error()
		}
		if a.toggling {
			receiver = "Updating receiver..."
		}
		a.write("receiver", receiver)
		view, _ := gui.View("receiver")
		if a.webhook.Err() != nil {
			view.FgColor = noticeColor(noticeError)
		} else if a.webhook.Running() {
			view.FgColor = noticeColor(noticeSuccess)
		}
		hooks := a.webhook.Hooks()
		rows = nil
		for _, hook := range hooks {
			rows = append(rows, fmt.Sprintf("%s %s %s (%dB) -> %s", hook.Timestamp, hook.Method, strings.Join(strings.Fields(plain(hook.Path)), " "), len(hook.RawBody), hookPreview(hook.Body)))
		}
		a.write("hooks", strings.Join(rows, "\n"))
		a.hookSelected = min(a.hookSelected, max(0, len(hooks)-1))
		a.highlight("hooks", a.hookSelected)
		detail := ""
		if len(hooks) > 0 {
			hook := hooks[a.hookSelected]
			detail = hook.Method + " " + hook.Path + "\n" + headerText(hook.Headers) + "\n\n" + preview(hook.Body)
		}
		a.write("hook-detail", detail)
	}
	a.chrome("footer", plain(a.shortcutFooter())+"\nEsc Quit | Tab Focus | "+coloredText(a.status, a.statusLevel))
	if current := gui.CurrentView(); current == nil || !current.Visible {
		if _, err := gui.SetCurrentView("url"); err != nil {
			return err
		}
	}
	gui.Cursor = gui.CurrentView().Editable
	if err := a.drawNotice(); err != nil {
		return err
	}
	if a.dialog != "" {
		return a.drawDialog()
	}
	if a.methodOpen {
		return a.drawMethodMenu(left)
	}
	return nil
}

func (a *App) escape(*gocui.Gui, *gocui.View) error {
	if a.dialog != "" {
		if a.dialog == "keys" && a.capturing {
			a.capturing = false
			a.shortcutMessage = ""
			return a.drawDialog()
		}
		a.closeDialog()
		return nil
	}
	return a.requestQuit(a.gui, a.gui.CurrentView())
}

func hookPreview(text string) string {
	limit := min(len(text), 256)
	for limit > 0 && limit < len(text) && !utf8.RuneStart(text[limit]) {
		limit--
	}
	line := strings.Join(strings.Fields(plain(text[:limit])), " ")
	clusters := uniseg.NewGraphemes(line)
	var snippet strings.Builder
	for count := 0; clusters.Next(); count++ {
		if count == 40 {
			return snippet.String() + "..."
		}
		snippet.WriteString(clusters.Str())
	}
	if limit < len(text) {
		return snippet.String() + "..."
	}
	return snippet.String()
}

func headerText(headers http.Header) string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var lines []string
	for _, key := range keys {
		lines = append(lines, key+": "+strings.Join(headers[key], ", "))
	}
	return strings.Join(lines, "\n")
}

func (a *App) requests() []goman.Request {
	var requests []goman.Request
	for _, collection := range a.collections {
		requests = append(requests, collection.Requests...)
	}
	return requests
}

func (a *App) highlight(name string, selected int) {
	view, err := a.gui.View(name)
	if err != nil {
		return
	}
	view.Highlight = true
	origin := view.OriginY()
	if selected < origin {
		origin = selected
	}
	if selected >= origin+view.InnerHeight() {
		origin = selected - view.InnerHeight() + 1
	}
	view.SetOrigin(0, max(0, origin))
	view.SetCursor(0, selected-max(0, origin))
}

func (a *App) move(name string, view *gocui.View, delta int) {
	switch name {
	case "collections":
		a.selected = max(0, min(a.selected+delta, len(a.requests())-1))
	case "hooks":
		a.hookSelected = max(0, min(a.hookSelected+delta, len(a.webhook.Hooks())-1))
	default:
		view.SetOriginY(max(0, min(view.OriginY()+delta, len(view.ViewBufferLines())-view.InnerHeight())))
	}
}

func (a *App) focusOrder() []string {
	order := []string{"collections", "new", "method", "url", "send", "save", "tabs"}
	switch a.tab {
	case 0:
		return append(order, "headers", "body")
	case 1:
		return append(order, "response")
	default:
		return append(order, "receiver", "hooks", "hook-detail")
	}
}

func (a *App) cycleFocus(delta int) error {
	if a.dialog != "" {
		return a.cycleDialog(delta)
	}
	if !a.usable() {
		return nil
	}
	a.closeMethodMenu()
	order := a.focusOrder()
	index := -1
	if view := a.gui.CurrentView(); view != nil {
		index = slices.Index(order, view.Name())
	}
	_, err := a.gui.SetCurrentView(order[(index+delta+len(order))%len(order)])
	return err
}

func (a *App) switchTab(index int) error {
	if a.dialog != "" {
		return nil
	}
	if !a.usable() {
		return nil
	}
	a.closeMethodMenu()
	a.clearSelection()
	a.tab = index
	if err := a.layout(a.gui); err != nil {
		return err
	}
	name := []string{"body", "response", "hooks"}[index]
	_, err := a.gui.SetCurrentView(name)
	return err
}

func (a *App) nextMethod(*gocui.Gui, *gocui.View) error {
	a.method = (a.method + 1) % len(methods)
	a.customMethod = ""
	return nil
}

func (a *App) requestMethod() string {
	if a.customMethod != "" {
		return a.customMethod
	}
	return methods[a.method]
}

func (a *App) currentRequest() (goman.Request, error) {
	request := goman.Request{Method: a.requestMethod()}
	for _, name := range []string{"url", "body", "headers"} {
		view, err := a.gui.View(name)
		if err != nil {
			return request, err
		}
		text := view.TextArea.GetContent()
		switch name {
		case "url":
			request.URL, err = goman.NormalizeURL(text)
		case "body":
			request.Body = text
		case "headers":
			if strings.TrimSpace(text) != "" {
				err = json.Unmarshal([]byte(text), &request.Headers)
			}
		}
		if err != nil {
			return request, fmt.Errorf("%s: %w", name, err)
		}
	}
	return request, nil
}

func (a *App) load(request goman.Request) error {
	a.clearSelection()
	a.customMethod = ""
	if index := slices.Index(methods, request.Method); index >= 0 {
		a.method = index
	} else {
		a.method = 0
		a.customMethod = request.Method
	}
	headers := request.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	raw, err := json.MarshalIndent(headers, "", "  ")
	if err != nil {
		return err
	}
	for name, text := range map[string]string{"url": request.URL, "body": request.Body, "headers": string(raw)} {
		view, err := a.gui.View(name)
		if err != nil {
			return err
		}
		setEditor(view, text)
	}
	return a.switchTab(0)
}

func (a *App) loadSelected(*gocui.Gui, *gocui.View) error {
	requests := a.requests()
	if a.selected < 0 || a.selected >= len(requests) {
		return nil
	}
	a.feedback("Selected: "+requests[a.selected].Name, noticeInfo)
	return a.load(requests[a.selected])
}

func (a *App) newRequest(*gocui.Gui, *gocui.View) error {
	if !a.usable() {
		return nil
	}
	a.feedback("New request ready", noticeInfo)
	if err := a.load(goman.Request{Method: "GET"}); err != nil {
		return err
	}
	_, err := a.gui.SetCurrentView("url")
	return err
}

func (a *App) usable() bool { width, height := a.gui.Size(); return width >= 80 && height >= 22 }

func (a *App) save(*gocui.Gui, *gocui.View) error {
	if a.saving {
		a.feedback("Save already in progress", noticeWarning)
		return nil
	}
	request, err := a.currentRequest()
	if err != nil {
		a.feedback(err.Error(), noticeError)
		return nil
	}
	parsed, _ := url.Parse(request.URL)
	name := path.Base(strings.TrimRight(parsed.Path, "/"))
	if name == "." || name == "/" {
		name = parsed.Hostname()
	}
	request.Name = request.Method + " " + string([]rune(name)[:min(18, len([]rune(name)))])
	collections := slices.Clone(a.collections)
	if len(collections) == 0 {
		collections = []goman.Collection{{Name: "My Collection"}}
	}
	collections[0].Requests = append(slices.Clone(collections[0].Requests), request)
	a.saving = true
	a.status = "Saving..."
	a.statusLevel = noticeInfo
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		err := a.store.Save(collections)
		a.gui.Update(func(*gocui.Gui) error {
			a.saving = false
			if err != nil {
				a.feedback("Save failed: "+err.Error(), noticeError)
			} else {
				a.collections = collections
				a.feedback("Saved: "+request.Name, noticeSuccess)
			}
			return nil
		})
	}()
	return nil
}

func (a *App) send(*gocui.Gui, *gocui.View) error {
	if a.busy {
		a.feedback("Request already in progress", noticeWarning)
		return nil
	}
	request, err := a.currentRequest()
	if err != nil {
		a.feedback(err.Error(), noticeError)
		return nil
	}
	view, _ := a.gui.View("url")
	setEditor(view, request.URL)
	a.busy = true
	a.requestProgress = "Sending " + request.Method + " to " + request.URL
	a.status = a.requestProgress
	a.statusLevel = noticeInfo
	a.response = goman.Response{Body: "Waiting for response..."}
	if err := a.switchTab(1); err != nil {
		a.busy = false
		return err
	}
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		result := goman.Execute(a.ctx, a.client, request)
		a.gui.Update(func(*gocui.Gui) error {
			a.busy = false
			a.response = result
			if result.Err != nil {
				a.feedback(fmt.Sprintf("Failed | %.2fms | %s", float64(result.Elapsed)/float64(time.Millisecond), result.Err), noticeError)
			} else {
				level := noticeSuccess
				if result.StatusCode >= 400 {
					level = noticeError
				}
				a.feedback(fmt.Sprintf("%d %s | %.2fms | %d bytes", result.StatusCode, result.Reason, float64(result.Elapsed)/float64(time.Millisecond), result.SizeBytes), level)
			}
			if view, err := a.gui.View("response"); err == nil {
				view.SetOrigin(0, 0)
			}
			return nil
		})
	}()
	return nil
}

func (a *App) toggleWebhook(*gocui.Gui, *gocui.View) error {
	if a.toggling {
		return nil
	}
	a.toggling = true
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		var err error
		if a.webhook.Running() {
			ctx, cancel := context.WithTimeout(a.ctx, 2*time.Second)
			err = a.webhook.Stop(ctx)
			cancel()
		} else {
			err = a.webhook.Start(a.address)
		}
		a.gui.Update(func(*gocui.Gui) error {
			a.toggling = false
			if err != nil {
				a.feedback("Receiver error: "+err.Error(), noticeError)
			} else if a.webhook.Running() {
				a.feedback("Webhook listener started", noticeSuccess)
			} else {
				a.feedback("Webhook listener stopped", noticeInfo)
			}
			return nil
		})
	}()
	return nil
}

func (a *App) replay(*gocui.Gui, *gocui.View) error {
	hooks := a.webhook.Hooks()
	if a.hookSelected < 0 || a.hookSelected >= len(hooks) {
		return nil
	}
	hook := hooks[a.hookSelected]
	view, err := a.gui.View("url")
	if err != nil {
		return err
	}
	request := goman.Request{Method: hook.Method, URL: view.TextArea.GetContent(), Body: hook.RawBody, Headers: map[string]string{}}
	for _, key := range []string{"Content-Type", "Accept"} {
		if value := hook.Headers.Get(key); value != "" {
			request.Headers[key] = value
		}
	}
	a.feedback("Loaded hook payload: "+hook.Path, noticeInfo)
	return a.load(request)
}
