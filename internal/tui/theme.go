package tui

import (
	"strings"

	"goman/internal/gocui"
)

var colors = struct {
	background, surface, editor, border, text, muted, accent, green, blue, teal gocui.Attribute
}{
	background: gocui.GetRGBColor(0x0b0e14),
	surface:    gocui.GetRGBColor(0x0d1117),
	editor:     gocui.GetRGBColor(0x161b22),
	border:     gocui.GetRGBColor(0x30363d),
	text:       gocui.GetRGBColor(0xe6edf3),
	muted:      gocui.GetRGBColor(0x8b949e),
	accent:     gocui.GetRGBColor(0x58a6ff),
	green:      gocui.GetRGBColor(0x238636),
	blue:       gocui.GetRGBColor(0x1f6feb),
	teal:       gocui.GetRGBColor(0x00ff9f),
}

func (a *App) styleView(view *gocui.View) {
	view.FgColor = colors.text
	view.BgColor = colors.surface
	view.FrameColor = colors.border
	view.SelFgColor = colors.text
	view.SelBgColor = colors.blue
	view.TitleColor = colors.muted
	view.Frame = true
	if current := a.gui.CurrentView(); current != nil && current.Name() == view.Name() {
		view.FrameColor = colors.accent
	}
	switch view.Name() {
	case "canvas":
		view.Frame = false
		view.BgColor = colors.background
	case "sidebar", "collections", "sidebar-title", "sidebar-rule", "header-rule", "tabs", "metrics":
		view.Frame = false
		if view.Name() == "tabs" {
			view.BgColor = colors.background
		}
		if view.Name() == "sidebar-title" {
			view.FgColor = colors.accent | gocui.AttrBold
		}
		if view.Name() == "sidebar-rule" {
			view.FgColor = colors.border
		}
		if view.Name() == "header-rule" {
			view.BgColor = colors.editor
			view.FgColor = colors.teal
		}
		if view.Name() == "metrics" {
			view.FgColor = colors.muted
		}
	case "header", "footer":
		view.Frame = false
		view.BgColor = colors.editor
		view.FgColor = colors.muted
	case "url", "method", "headers", "body", "receiver":
		view.BgColor = colors.editor
	case "method-menu":
		view.BgColor = colors.editor
		view.SelFgColor = gocui.ColorWhite | gocui.AttrBold
	case "dialog":
		view.BgColor = colors.editor
		view.FrameColor = colors.accent
	case "dialog-shortcuts":
		view.Frame = false
		view.BgColor = colors.editor
	case "dialog-primary", "dialog-cancel", "dialog-reset":
		view.Frame = false
		view.BgColor = colors.border
		view.FgColor = colors.text | gocui.AttrBold
	case "send", "save", "new":
		view.Frame = false
		view.FgColor = gocui.ColorWhite | gocui.AttrBold
		switch view.Name() {
		case "send":
			view.BgColor = colors.green
		case "save":
			view.BgColor = colors.blue
		case "new":
			view.BgColor = colors.border
			view.FgColor = colors.accent | gocui.AttrBold
		}
	}
}

func (a *App) chrome(name, text string) {
	if previous, ok := a.content[name]; ok && previous == text {
		return
	}
	if view, err := a.gui.View(name); err == nil {
		view.SetContent(text)
		a.content[name] = text
	}
}

func tabAt(column int) int {
	for index, label := range tabNames {
		if column >= 0 && column < len(label)+4 {
			return index
		}
		column -= len(label) + 4
	}
	return -1
}

func (a *App) drawTabs() {
	view, err := a.gui.View("tabs")
	if err != nil {
		return
	}
	var labels, rule strings.Builder
	for index, label := range tabNames {
		color := "\x1b[38;2;139;148;158m"
		lineColor := "\x1b[38;2;48;54;61m"
		if index == a.tab {
			color = "\x1b[38;2;230;237;243m\x1b[1m"
			lineColor = "\x1b[38;2;0;152;232m"
		}
		labels.WriteString(color + "  " + label + "  \x1b[0m")
		rule.WriteString(lineColor + strings.Repeat("\u2501", len(label)+4) + "\x1b[0m")
	}
	used := 0
	for _, label := range tabNames {
		used += len(label) + 4
	}
	rule.WriteString("\x1b[38;2;48;54;61m" + strings.Repeat("\u2500", max(0, view.InnerWidth()-used)) + "\x1b[0m")
	a.chrome("tabs", labels.String()+"\n"+rule.String())
}

func (a *App) button(name, label string) {
	view, err := a.gui.View(name)
	if err != nil {
		return
	}
	a.write(name, strings.Repeat("\n", max(0, (view.InnerHeight()-1)/2))+strings.Repeat(" ", max(0, (view.InnerWidth()-len(label))/2))+label)
}
