package tui

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/atotto/clipboard"
	"github.com/rivo/uniseg"
	"goman/internal/gocui"
)

type textSelection struct {
	field       string
	text        string
	anchor, end int
	dragging    bool
	tokens      []chroma.Token
}

func fieldText(view *gocui.View) string {
	if view.Editable {
		return view.TextArea.GetContent()
	}
	return strings.TrimSuffix(view.Buffer(), "\n")
}

func textOffset(text string, x, y int) int {
	lines := strings.Split(text, "\n")
	if y < 0 {
		return 0
	}
	if y >= len(lines) {
		return len(text)
	}
	offset := 0
	for _, line := range lines[:y] {
		offset += len(line) + 1
	}
	column := 0
	clusters := uniseg.NewGraphemes(lines[y])
	for clusters.Next() {
		start, end := clusters.Positions()
		width := clusters.Width()
		if clusters.Str() == "\t" {
			width = 4 - column%4
		}
		if x < column+width {
			return offset + start
		}
		column += width
		if x == column {
			return offset + end
		}
	}
	return offset + len(lines[y])
}

func (a *App) setupTextSelection() {
	for _, name := range []string{"url", "headers", "body", "response", "hook-detail"} {
		for _, event := range []struct {
			key      gocui.KeyName
			modifier gocui.Modifier
		}{{gocui.MouseLeft, gocui.ModNone}, {gocui.MouseLeft, gocui.ModMotion}, {gocui.MouseRelease, gocui.ModNone}} {
			key := event.key
			drag := event.modifier == gocui.ModMotion
			a.gui.SetViewClickBinding(&gocui.ViewMouseBinding{ViewName: name, Key: key, Modifier: event.modifier, Handler: func(opts gocui.ViewMouseBindingOpts) error {
				view, err := a.gui.View(name)
				if err != nil {
					return err
				}
				if key == gocui.MouseLeft && !drag {
					a.clearSelection()
					if _, err := a.gui.SetCurrentView(name); err != nil {
						return err
					}
					text := fieldText(view)
					offset := textOffset(text, opts.X, opts.Y)
					a.selection = textSelection{field: name, text: text, anchor: offset, end: offset, dragging: true}
				} else if a.selection.field == name && a.selection.dragging {
					x := max(view.OriginX(), min(opts.X, view.OriginX()+view.InnerWidth()))
					y := max(view.OriginY(), min(opts.Y, view.OriginY()+view.InnerHeight()-1))
					a.selection.end = textOffset(a.selection.text, x, y)
					a.renderSelection(view)
					if key == gocui.MouseRelease {
						a.selection.dragging = false
					}
				}
				return nil
			}})
		}
		a.gui.SetKeybinding(name, gocui.NewKeyName(gocui.KeyEsc), a.escape)
	}
}

func (a *App) copySelection(_ *gocui.Gui, view *gocui.View) error {
	if view == nil {
		return nil
	}
	text := a.selectedText(view.Name())
	if text == "" {
		return nil
	}
	if err := clipboard.WriteAll(text); err != nil {
		a.feedback("Copy failed: "+err.Error(), noticeError)
	} else {
		a.feedback(fmt.Sprintf("Copied %d bytes", len(text)), noticeSuccess)
	}
	return nil
}

func (a *App) selectAll(_ *gocui.Gui, view *gocui.View) error {
	if view == nil || !view.Visible {
		return nil
	}
	a.clearSelection()
	text := fieldText(view)
	if text != "" {
		a.selection = textSelection{field: view.Name(), text: text, end: len(text)}
		a.renderSelection(view)
	}
	return nil
}

func (a *App) selectedText(field string) string {
	if a.selection.field != field {
		return ""
	}
	start, end := min(a.selection.anchor, a.selection.end), max(a.selection.anchor, a.selection.end)
	return a.selection.text[start:end]
}

func (a *App) renderSelection(view *gocui.View) {
	start, end := min(a.selection.anchor, a.selection.end), max(a.selection.anchor, a.selection.end)
	if a.selection.tokens == nil {
		a.selection.tokens = syntaxTokens(view.Name(), a.selection.text)
	}
	renderTokens(view, a.selection.tokens, start, end)
}

func (a *App) clearSelection() {
	if a.selection.field == "" {
		return
	}
	if view, err := a.gui.View(a.selection.field); err == nil {
		if a.selection.tokens == nil {
			a.selection.tokens = syntaxTokens(view.Name(), a.selection.text)
		}
		renderTokens(view, a.selection.tokens, 0, 0)
	}
	a.selection = textSelection{}
}

func (a *App) editSelection(view *gocui.View, key gocui.Key) bool {
	if a.selection.field == view.Name() {
		if fieldText(view) == a.selection.text && a.selectedText(view.Name()) != "" && (key.IsPrintable() || key.Equals(gocui.NewKeyName(gocui.KeyEnter)) || key.Equals(gocui.NewKeyName(gocui.KeyBackspace)) || key.Equals(gocui.NewKeyName(gocui.KeyDelete))) {
			start, end := min(a.selection.anchor, a.selection.end), max(a.selection.anchor, a.selection.end)
			prefix := a.selection.text[:start]
			suffix := a.selection.text[end:]
			text := prefix + suffix
			a.clearSelection()
			setEditor(view, text)
			clusters := uniseg.NewGraphemes(suffix)
			for clusters.Next() {
				view.TextArea.MoveCursorLeft()
			}
			if key.Equals(gocui.NewKeyName(gocui.KeyBackspace)) || key.Equals(gocui.NewKeyName(gocui.KeyDelete)) {
				view.RenderTextArea()
				renderSyntax(view, view.TextArea.GetContent(), 0, 0)
				return true
			}
		} else {
			a.clearSelection()
		}
	}
	changed := gocui.SimpleEditor(view, key)
	if changed {
		renderSyntax(view, view.TextArea.GetContent(), 0, 0)
	}
	return changed
}
