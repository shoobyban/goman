package gocui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyIsPrintable(t *testing.T) {
	assert.True(t, NewKeyRune('x').IsPrintable())
	assert.True(t, NewKeyRune('界').IsPrintable())
	assert.True(t, NewKeyRune(' ').IsPrintable())
	assert.False(t, NewKeyStrMod("x", ModCtrl).IsPrintable())
	assert.False(t, NewKeyName(KeyEnter).IsPrintable())
	assert.False(t, Key{}.IsPrintable())
}

func TestPasteBypassesCommandBindings(t *testing.T) {
	gui := newTestGui(t)
	parent, view := setupParentAndChildView(t, gui)
	view.Editable = true
	view.Editor = DefaultEditor
	activated := 0
	for _, name := range []string{"", parent.Name(), view.Name()} {
		for _, key := range []Key{NewKeyName(KeyEnter), NewKeyRune('x'), NewKeyStrMod("s", ModCtrl)} {
			gui.SetKeybinding(name, key, func(*Gui, *View) error { activated++; return nil })
		}
	}
	gui.IsPasting = true
	for _, key := range []Key{NewKeyRune('x'), NewKeyName(KeyEnter), NewKeyStrMod("j", ModCtrl), NewKeyStrMod("s", ModCtrl)} {
		assert.NoError(t, gui.onKey(&GocuiEvent{Type: eventKey, Key: key}))
	}
	assert.Equal(t, "x\n\n", view.TextArea.GetContent())
	assert.Zero(t, activated)
	view.Editable = false
	assert.NoError(t, gui.onKey(&GocuiEvent{Type: eventKey, Key: NewKeyName(KeyEnter)}))
	assert.Zero(t, activated)
	gui.IsPasting = false
	assert.NoError(t, gui.onKey(&GocuiEvent{Type: eventKey, Key: NewKeyName(KeyEnter)}))
	assert.Equal(t, 1, activated)
}
