package tui

import (
	"testing"

	bubbletea "github.com/charmbracelet/bubbletea"
)

func keyStr(s string) bubbletea.KeyMsg {
	return bubbletea.KeyMsg{Type: bubbletea.KeyRunes, Runes: []rune(s)}
}

func newPicker(n int) *pickerModel {
	items := make([]TrackItem, n)
	for i := range items {
		items[i] = TrackItem{ID: string(rune('a' + i)), Title: "t", Workspace: "w"}
	}
	return &pickerModel{items: items, checked: map[int]bool{}}
}

func TestPickerToggleAndConfirm(t *testing.T) {
	m := newPicker(5)

	m.Update(keyStr(" ")) // toggle item 0
	if !m.checked[0] {
		t.Fatal("item 0 not checked after space")
	}
	m.Update(keyStr("j"))
	m.Update(keyStr(" ")) // toggle item 1
	m.Update(keyStr(" ")) // untoggle item 1
	if m.checked[1] {
		t.Fatal("item 1 should be unchecked")
	}
	if !m.checked[0] {
		t.Fatal("item 0 should stay checked")
	}

	model, _ := m.Update(bubbletea.KeyMsg{Type: bubbletea.KeyEnter})
	got := model.(*pickerModel)
	if !got.confirmed {
		t.Fatal("confirmed not set on enter")
	}
}

func TestPickerSelectAllAndNone(t *testing.T) {
	m := newPicker(4)
	m.Update(keyStr("a"))
	for i := range m.items {
		if !m.checked[i] {
			t.Fatalf("item %d not selected after 'a'", i)
		}
	}
	m.Update(keyStr("n"))
	for i := range m.items {
		if m.checked[i] {
			t.Fatalf("item %d still selected after 'n'", i)
		}
	}
}

func TestPickerCancel(t *testing.T) {
	m := newPicker(3)
	model, _ := m.Update(keyStr("q"))
	if got := model.(*pickerModel); !got.canceled || got.confirmed {
		t.Fatal("q must cancel without confirm")
	}
}

func TestPickerScrollWindow(t *testing.T) {
	m := newPicker(30)
	for i := 0; i < 20; i++ {
		m.Update(keyStr("j"))
	}
	if m.cursor != 20 {
		t.Fatalf("cursor = %d, want 20", m.cursor)
	}
	m.clampOffset()
	wantOff := 20 - visibleRows + 1
	if m.offset != wantOff {
		t.Fatalf("offset = %d, want %d", m.offset, wantOff)
	}
	m.Update(keyStr("g"))
	if m.cursor != 0 || m.offset != 0 {
		t.Fatalf("home reset failed: cursor=%d offset=%d", m.cursor, m.offset)
	}
}
