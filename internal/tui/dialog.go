package tui

import "goman/internal/gocui"

func (a *App) requestQuit(*gocui.Gui, *gocui.View) error {
	if a.dialog != "" {
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
	a.dialog = "quit"
	a.dialogChoice = 0
	return a.drawDialog()
}

func (a *App) bindDialog() {
	for _, name := range []string{"dialog-primary", "dialog-cancel", "dialog-reset"} {
		handler := a.confirmQuit
		if name == "dialog-cancel" {
			handler = func(*gocui.Gui, *gocui.View) error { a.closeDialog(); return nil }
		}
		if name == "dialog-reset" {
			handler = a.resetShortcuts
		}
		a.gui.SetKeybinding(name, gocui.NewKeyName(gocui.KeyEnter), func(gui *gocui.Gui, view *gocui.View) error {
			if a.dialog == "quit" {
				return a.confirmQuit(gui, view)
			}
			return handler(gui, view)
		})
		a.gui.SetKeybinding(name, gocui.NewKeyName(gocui.MouseLeft), handler)
		for _, movement := range []struct {
			key   gocui.KeyName
			delta int
		}{{gocui.KeyArrowLeft, -1}, {gocui.KeyArrowRight, 1}} {
			a.gui.SetKeybinding(name, gocui.NewKeyName(movement.key), func(*gocui.Gui, *gocui.View) error { return a.cycleDialog(movement.delta) })
		}
	}
}

func (a *App) confirmQuit(*gocui.Gui, *gocui.View) error {
	if a.dialog == "quit" {
		return gocui.ErrQuit
	}
	if a.dialog == "keys" {
		a.shortcuts = cloneShortcuts(a.shortcutDraft)
		a.closeDialog()
		a.bindKeys()
		a.feedback("Shortcuts applied", noticeSuccess)
	}
	return nil
}

func (a *App) cycleDialog(delta int) error {
	if a.capturing {
		a.shortcutMessage = "Tab is reserved"
		return a.drawDialog()
	}
	count := 2
	if a.dialog == "keys" {
		count = 4
	}
	a.dialogChoice = (a.dialogChoice + delta + count) % count
	return a.drawDialog()
}

func (a *App) drawDialog() error {
	if !a.usable() {
		return a.layout(a.gui)
	}
	if a.dialog == "keys" {
		return a.drawShortcutDialog()
	}
	width, height := a.gui.Size()
	dialogWidth := max(1, min(54, width-4))
	left, top := max(0, (width-dialogWidth)/2), max(0, (height-9)/2)
	for _, spec := range []struct {
		name           string
		x0, y0, x1, y1 int
		title          string
	}{
		{"dialog", left, top, left + dialogWidth, top + 9, "Quit goman"},
		{"dialog-primary", left + 4, top + 4, left + 18, top + 8, ""},
		{"dialog-cancel", left + dialogWidth - 19, top + 4, left + dialogWidth - 5, top + 8, ""},
	} {
		if _, err := a.view(spec.name, spec.x0, spec.y0, spec.x1, spec.y1, spec.title, false); err != nil {
			return err
		}
		if _, err := a.gui.SetViewOnTop(spec.name); err != nil {
			return err
		}
	}
	a.write("dialog", "\n  Quit goman?\n  Unsaved editor changes will be lost.")
	a.button("dialog-primary", "QUIT")
	a.button("dialog-cancel", "CANCEL")
	name := []string{"dialog-primary", "dialog-cancel"}[a.dialogChoice]
	if _, err := a.gui.SetCurrentView(name); err != nil {
		return err
	}
	view, _ := a.gui.View(name)
	view.BgColor = colors.blue
	a.gui.Cursor = false
	return nil
}

func (a *App) closeDialog() {
	a.dialog = ""
	a.capturing = false
	a.shortcutDraft = nil
	for _, name := range []string{"dialog", "dialog-primary", "dialog-cancel", "dialog-reset", "dialog-shortcuts"} {
		if view, err := a.gui.View(name); err == nil {
			view.Visible = false
		}
	}
	if view, err := a.gui.View(a.dialogReturn); err == nil && view.Visible {
		a.gui.SetCurrentView(a.dialogReturn)
	}
}
