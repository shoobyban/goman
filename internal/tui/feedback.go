package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rivo/uniseg"
	"goman/internal/gocui"
)

type noticeLevel int

const (
	noticeInfo noticeLevel = iota
	noticeSuccess
	noticeWarning
	noticeError
)

func noticeColor(level noticeLevel) gocui.Attribute {
	switch level {
	case noticeSuccess:
		return gocui.GetRGBColor(0x3fb950)
	case noticeWarning:
		return gocui.GetRGBColor(0xd29922)
	case noticeError:
		return gocui.GetRGBColor(0xf85149)
	default:
		return colors.accent
	}
}

func coloredText(text string, level noticeLevel) string {
	color := "88;166;255"
	switch level {
	case noticeSuccess:
		color = "63;185;80"
	case noticeWarning:
		color = "210;153;34"
	case noticeError:
		color = "248;81;73"
	}
	return "\x1b[38;2;" + color + "m" + plain(text) + "\x1b[0m"
}

func (a *App) feedback(message string, level noticeLevel) {
	a.status, a.statusLevel = plain(message), level
	a.notify(message, level, 2*time.Second)
}

func (a *App) notify(message string, level noticeLevel, duration time.Duration) {
	a.dismissNotice()
	a.notice, a.noticeLevel = plain(message), level
	id := a.noticeID
	ctx, cancel := context.WithCancel(a.ctx)
	a.noticeCancel = cancel
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			a.gui.Update(func(*gocui.Gui) error {
				if a.noticeID == id {
					a.dismissNotice()
				}
				return nil
			})
		}
	}()
}

func (a *App) dismissNotice() {
	if a.noticeCancel != nil {
		a.noticeCancel()
		a.noticeCancel = nil
	}
	a.noticeID++
	a.notice = ""
	if view, err := a.gui.View("notification"); err == nil {
		view.Visible = false
	}
}

func noticeLines(text string, width int) string {
	lineWidth := max(1, width-3)
	clusters := uniseg.NewGraphemes(strings.Join(strings.Fields(plain(text)), " "))
	var lines []string
	var line strings.Builder
	column := 0
	for clusters.Next() {
		if column+clusters.Width() > lineWidth {
			if len(lines) == 2 {
				return strings.Join(append(lines, line.String()+"..."), "\n ")
			}
			lines = append(lines, line.String())
			line.Reset()
			column = 0
		}
		line.WriteString(clusters.Str())
		column += clusters.Width()
	}
	return strings.Join(append(lines, line.String()), "\n ")
}

func (a *App) drawNotice() error {
	if a.notice == "" || a.dialog != "" || a.methodOpen {
		return nil
	}
	width, height := a.gui.Size()
	if width < 80 || height < 22 {
		return nil
	}
	noticeWidth := min(54, width-4)
	left, top := width-noticeWidth-2, height-9
	title := []string{"Notice", "Success", "Warning", "Error"}[a.noticeLevel]
	view, err := a.view("notification", left, top, left+noticeWidth, top+5, title, false)
	if err != nil {
		return err
	}
	view.BgColor = colors.editor
	view.FrameColor = noticeColor(a.noticeLevel)
	view.TitleColor = noticeColor(a.noticeLevel)
	view.FgColor = colors.text
	a.write("notification", "\n "+noticeLines(a.notice, view.InnerWidth()-2))
	_, err = a.gui.SetViewOnTop("notification")
	return err
}

func (a *App) drawMetrics() {
	label := "Status: Idle"
	if a.busy {
		label = coloredText(a.requestProgress, noticeInfo)
	} else if a.response.Err != nil {
		label = coloredText("Failed", noticeError) + fmt.Sprintf(" | %.2fms", float64(a.response.Elapsed)/float64(time.Millisecond))
	} else if a.response.StatusCode > 0 {
		level := noticeSuccess
		if a.response.StatusCode >= 400 {
			level = noticeError
		}
		label = coloredText(fmt.Sprintf("%d %s", a.response.StatusCode, a.response.Reason), level) + fmt.Sprintf(" | %.2fms | %d bytes", float64(a.response.Elapsed)/float64(time.Millisecond), a.response.SizeBytes)
	}
	a.chrome("metrics", label)
}
