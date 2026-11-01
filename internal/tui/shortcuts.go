package tui

import (
	"fmt"
	"slices"
	"strings"

	"goman/internal/gocui"
)

type shortcut struct {
	id, label string
	keys      []gocui.Key
	views     []string
}

func defaultShortcuts() []shortcut {
	fields := []string{"url", "headers", "body", "response", "hook-detail"}
	ctrl := func(value string) []gocui.Key { return []gocui.Key{gocui.NewKeyStrMod(value, gocui.ModCtrl)} }
	return []shortcut{
		{"send", "Send", ctrl("s"), nil},
		{"save", "Save", ctrl("k"), nil},
		{"new", "New request", ctrl("n"), nil},
		{"receiver", "Receiver", ctrl("w"), nil},
		{"quit", "Quit", ctrl("q"), nil},
		{"configure", "Configuration", []gocui.Key{gocui.NewKeyName(gocui.KeyF2)}, nil},
		{"previous", "Previous tab", ctrl("["), nil},
		{"next", "Next tab", ctrl("]"), nil},
		{"body", "Body tab", []gocui.Key{gocui.NewKeyStrMod("1", gocui.ModAlt)}, nil},
		{"response", "Response tab", []gocui.Key{gocui.NewKeyStrMod("2", gocui.ModAlt)}, nil},
		{"webhooks", "Webhooks tab", []gocui.Key{gocui.NewKeyStrMod("3", gocui.ModAlt)}, nil},
		{"all", "Select all", []gocui.Key{gocui.NewKeyStrMod("g", gocui.ModCtrl), gocui.NewKeyStrMod("a", gocui.ModCtrl|gocui.ModShift), gocui.NewKeyStrMod("A", gocui.ModCtrl|gocui.ModShift)}, fields},
		{"copy", "Copy", ctrl("c"), fields},
	}
}

func cloneShortcuts(source []shortcut) []shortcut {
	result := slices.Clone(source)
	for index := range result {
		result[index].keys = slices.Clone(source[index].keys)
	}
	return result
}

func (a *App) shortcutHandler(id string) func(*gocui.Gui, *gocui.View) error {
	switch id {
	case "send":
		return a.send
	case "save":
		return a.save
	case "new":
		return a.newRequest
	case "receiver":
		return a.toggleWebhook
	case "quit":
		return a.requestQuit
	case "configure":
		return a.openShortcuts
	case "previous":
		return func(*gocui.Gui, *gocui.View) error { return a.switchTab((a.tab + 2) % 3) }
	case "next":
		return func(*gocui.Gui, *gocui.View) error { return a.switchTab((a.tab + 1) % 3) }
	case "body":
		return func(*gocui.Gui, *gocui.View) error { return a.switchTab(0) }
	case "response":
		return func(*gocui.Gui, *gocui.View) error { return a.switchTab(1) }
	case "webhooks":
		return func(*gocui.Gui, *gocui.View) error { return a.switchTab(2) }
	case "all":
		return a.selectAll
	case "copy":
		return a.copySelection
	}
	return nil
}

func (a *App) bindShortcuts() {
	for _, command := range a.shortcuts {
		views := command.views
		if len(views) == 0 {
			views = []string{"", "url", "headers", "body"}
		}
		for _, key := range command.keys {
			for _, name := range views {
				a.gui.SetKeybinding(name, key, func(gui *gocui.Gui, view *gocui.View) error {
					if a.dialog == "keys" && a.capturing {
						return a.captureShortcut(key)
					}
					if a.dialog != "" {
						return nil
					}
					return a.shortcutHandler(command.id)(gui, view)
				})
			}
		}
	}
}

func keyLabel(key gocui.Key) string {
	var parts []string
	for _, modifier := range []struct {
		value gocui.Modifier
		label string
	}{{gocui.ModCtrl, "Ctrl"}, {gocui.ModAlt, "Alt"}, {gocui.ModShift, "Shift"}, {gocui.ModMeta, "Meta"}} {
		if key.Mod()&modifier.value != 0 {
			parts = append(parts, modifier.label)
		}
	}
	label := strings.ToUpper(key.Str())
	if label == " " {
		label = "Space"
	}
	if label == "" {
		if key.KeyName() >= gocui.KeyF1 && key.KeyName() <= gocui.KeyF12 {
			label = fmt.Sprintf("F%d", int(key.KeyName()-gocui.KeyF1)+1)
		} else {
			label = fmt.Sprintf("Key %d", key.KeyName())
		}
	}
	return strings.Join(append(parts, label), "+")
}

func shortcutLabels(command shortcut) string {
	var labels []string
	for _, key := range command.keys {
		label := keyLabel(key)
		if !slices.Contains(labels, label) {
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, " / ")
}

func (a *App) shortcutFooter() string {
	var parts []string
	for _, command := range a.shortcuts {
		if slices.Contains([]string{"send", "save", "quit", "configure"}, command.id) {
			parts = append(parts, keyLabel(command.keys[0])+" "+command.label)
		}
	}
	return strings.Join(parts, " | ")
}

func (a *App) openShortcuts(*gocui.Gui, *gocui.View) error {
	if a.dialog != "" || !a.usable() {
		return nil
	}
	if current := a.gui.CurrentView(); current != nil {
		a.dialogReturn = current.Name()
	}
	if a.dialogReturn == "method-menu" {
		a.dialogReturn = "method"
	}
	a.closeMethodMenu()
	a.clearSelection()
	a.dialog = "keys"
	a.dialogChoice = 0
	a.shortcutIndex = 0
	a.shortcutMessage = ""
	a.shortcutDraft = cloneShortcuts(a.shortcuts)
	return a.drawDialog()
}

func (a *App) resetShortcuts(*gocui.Gui, *gocui.View) error {
	if a.dialog != "keys" {
		return nil
	}
	a.shortcutDraft = defaultShortcuts()
	a.capturing = false
	a.shortcutMessage = "Defaults restored in draft"
	return a.drawDialog()
}

func (a *App) captureShortcut(key gocui.Key) error {
	if !a.capturing {
		return nil
	}
	message := ""
	switch key.KeyName() {
	case gocui.KeyEsc, gocui.KeyEnter, gocui.KeyTab, gocui.KeyBacktab:
		message = "Escape, Tab, and Enter are reserved"
	case gocui.KeyArrowUp, gocui.KeyArrowDown, gocui.KeyArrowLeft, gocui.KeyArrowRight, gocui.KeyHome, gocui.KeyEnd, gocui.KeyPgup, gocui.KeyPgdn, gocui.KeyBackspace, gocui.KeyDelete, gocui.KeyInsert:
		message = "Key is used by field navigation"
	}
	if message == "" && key.Str() == "" && (key.KeyName() < gocui.KeyF1 || key.KeyName() > gocui.KeyF12) {
		message = "Use a modified character key or F1-F12"
	}
	if key.IsPrintable() || key.Equals(gocui.NewKeyStrMod("a", gocui.ModCtrl)) {
		message = "Keep text-editing keys available; use a modified key or F1-F12"
	}
	for index, command := range a.shortcutDraft {
		if index == a.shortcutIndex {
			continue
		}
		for _, existing := range command.keys {
			if existing.Equals(key) {
				message = "Already assigned to " + command.label
			}
		}
	}
	if message != "" {
		a.shortcutMessage = message
		return a.drawDialog()
	}
	a.shortcutDraft[a.shortcutIndex].keys = []gocui.Key{key}
	a.capturing = false
	a.shortcutMessage = "New binding: " + keyLabel(key)
	return a.drawDialog()
}

func (a *App) bindShortcutDialog() {
	a.gui.SetKeybinding("dialog-shortcuts", gocui.NewKeyName(gocui.KeyEsc), a.escape)
	a.gui.SetKeybinding("dialog-shortcuts", gocui.NewKeyName(gocui.KeyTab), func(*gocui.Gui, *gocui.View) error { return a.cycleFocus(1) })
	a.gui.SetKeybinding("dialog-shortcuts", gocui.NewKeyName(gocui.KeyBacktab), func(*gocui.Gui, *gocui.View) error { return a.cycleFocus(-1) })
	a.gui.SetKeybinding("dialog-shortcuts", gocui.NewKey(gocui.KeyTab, "", gocui.ModShift), func(*gocui.Gui, *gocui.View) error { return a.cycleFocus(-1) })
	a.gui.SetKeybinding("dialog-shortcuts", gocui.NewKeyName(gocui.KeyEnter), func(*gocui.Gui, *gocui.View) error {
		if a.capturing {
			a.shortcutMessage = "Enter is reserved"
		} else {
			a.capturing = true
			a.shortcutMessage = "Awaiting key..."
		}
		return a.drawDialog()
	})
	for _, movement := range []struct {
		key   gocui.KeyName
		delta int
	}{{gocui.KeyArrowUp, -1}, {gocui.KeyArrowDown, 1}, {gocui.MouseWheelUp, -1}, {gocui.MouseWheelDown, 1}} {
		a.gui.SetKeybinding("dialog-shortcuts", gocui.NewKeyName(movement.key), func(*gocui.Gui, *gocui.View) error {
			if a.capturing {
				return a.captureShortcut(gocui.NewKeyName(movement.key))
			}
			a.shortcutIndex = max(0, min(len(a.shortcutDraft)-1, a.shortcutIndex+movement.delta))
			return a.drawDialog()
		})
	}
	a.gui.SetViewClickBinding(&gocui.ViewMouseBinding{ViewName: "dialog-shortcuts", Key: gocui.MouseLeft, Handler: func(opts gocui.ViewMouseBindingOpts) error {
		if a.capturing {
			return nil
		}
		a.shortcutIndex = max(0, min(len(a.shortcutDraft)-1, opts.Y))
		a.dialogChoice = 0
		return a.drawDialog()
	}})
}

func (a *App) drawShortcutDialog() error {
	width, height := a.gui.Size()
	dialogWidth, dialogHeight := min(72, width-4), min(22, height-2)
	left, top := (width-dialogWidth)/2, (height-dialogHeight)/2
	for _, spec := range []struct {
		name           string
		x0, y0, x1, y1 int
		title          string
	}{
		{"dialog", left, top, left + dialogWidth, top + dialogHeight, "Shortcuts"},
		{"dialog-shortcuts", left + 2, top + 1, left + dialogWidth - 2, top + dialogHeight - 5, ""},
		{"dialog-reset", left + 3, top + dialogHeight - 5, left + 17, top + dialogHeight - 1, ""},
		{"dialog-primary", left + dialogWidth - 35, top + dialogHeight - 5, left + dialogWidth - 21, top + dialogHeight - 1, ""},
		{"dialog-cancel", left + dialogWidth - 19, top + dialogHeight - 5, left + dialogWidth - 5, top + dialogHeight - 1, ""},
	} {
		if _, err := a.view(spec.name, spec.x0, spec.y0, spec.x1, spec.y1, spec.title, false); err != nil {
			return err
		}
		if _, err := a.gui.SetViewOnTop(spec.name); err != nil {
			return err
		}
	}
	var rows []string
	for _, command := range a.shortcutDraft {
		rows = append(rows, fmt.Sprintf("%-16s %s", command.label, shortcutLabels(command)))
	}
	a.write("dialog-shortcuts", strings.Join(rows, "\n"))
	a.highlight("dialog-shortcuts", a.shortcutIndex)
	list, _ := a.gui.View("dialog-shortcuts")
	list.Editable = true
	list.Editor = gocui.EditorFunc(func(_ *gocui.View, key gocui.Key) bool {
		if !a.capturing {
			return false
		}
		_ = a.captureShortcut(key)
		return true
	})
	a.write("dialog", strings.Repeat("\n", dialogHeight-6)+"  "+a.shortcutMessage)
	a.button("dialog-reset", "RESET")
	a.button("dialog-primary", "APPLY")
	a.button("dialog-cancel", "CANCEL")
	name := []string{"dialog-shortcuts", "dialog-reset", "dialog-primary", "dialog-cancel"}[a.dialogChoice]
	if _, err := a.gui.SetCurrentView(name); err != nil {
		return err
	}
	if name != "dialog-shortcuts" {
		view, _ := a.gui.View(name)
		view.BgColor = colors.blue
	}
	a.gui.Cursor = false
	return nil
}
