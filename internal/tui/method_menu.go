package tui

import (
	"strings"

	"goman/internal/gocui"
)

func (a *App) bindMethodMenu() {
	a.gui.SetKeybinding("method", gocui.NewKeyName(gocui.KeyArrowDown), a.toggleMethodMenu)
	for _, movement := range []struct {
		key   gocui.KeyName
		delta int
	}{{gocui.KeyArrowUp, -1}, {gocui.KeyArrowDown, 1}, {gocui.MouseWheelUp, -1}, {gocui.MouseWheelDown, 1}} {
		a.gui.SetKeybinding("method-menu", gocui.NewKeyName(movement.key), func(*gocui.Gui, *gocui.View) error {
			a.methodOption = max(0, min(len(methods)-1, a.methodOption+movement.delta))
			return nil
		})
	}
	a.gui.SetKeybinding("method-menu", gocui.NewKeyName(gocui.KeyEnter), a.chooseMethod)
	a.gui.SetKeybinding("method-menu", gocui.NewKeyName(gocui.KeyEsc), a.escape)
}

func (a *App) toggleMethodMenu(*gocui.Gui, *gocui.View) error {
	if !a.usable() {
		return nil
	}
	if a.methodOpen {
		a.closeMethodMenu()
		return nil
	}
	a.clearSelection()
	a.methodOpen = true
	a.methodOption = a.method
	return a.layout(a.gui)
}

func (a *App) drawMethodMenu(left int) error {
	view, err := a.view("method-menu", left, 5, left+14, 5+len(methods)+3, "", false)
	if err != nil {
		return err
	}
	a.write("method-menu", "\n  "+strings.Join(methods, "\n  ")+"\n")
	a.highlight("method-menu", a.methodOption+1)
	if _, err := a.gui.SetViewOnTop("method-menu"); err != nil {
		return err
	}
	if _, err := a.gui.SetCurrentView("method-menu"); err != nil {
		return err
	}
	view.FrameColor = colors.accent
	a.gui.Cursor = false
	return nil
}

func (a *App) closeMethodMenu() {
	if !a.methodOpen {
		return
	}
	a.methodOpen = false
	if view, err := a.gui.View("method-menu"); err == nil {
		view.Visible = false
	}
	if current := a.gui.CurrentView(); current != nil && current.Name() == "method-menu" {
		a.gui.SetCurrentView("method")
	}
}

func (a *App) chooseMethod(*gocui.Gui, *gocui.View) error {
	a.method = a.methodOption
	a.customMethod = ""
	a.closeMethodMenu()
	return nil
}
