package tui

import (
	"fmt"
	"strings"

	bubbletea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TrackItem is one selectable row in the interactive sync picker.
type TrackItem struct {
	ID         string
	Title      string
	Workspace  string
	CreatedAt  string
	Downloaded bool
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e5b567"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#8f8a83"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6ee7a0"))
	cursorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7"))
	dlYesStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6ee7a0"))
	dlNoStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#f0615f"))
)

const visibleRows = 15

type pickerModel struct {
	items     []TrackItem
	checked   map[int]bool
	cursor    int
	offset    int
	confirmed bool
	canceled  bool
}

// SelectTracks runs the interactive multi-select TUI and returns the IDs of
// confirmed tracks. Returns nil, nil when the user cancels.
func SelectTracks(items []TrackItem) ([]string, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no tracks to select")
	}
	m := &pickerModel{
		items:   items,
		checked: map[int]bool{},
	}
	p := bubbletea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return nil, fmt.Errorf("tui: %w", err)
	}
	if m.canceled || !m.confirmed {
		return nil, nil
	}
	var ids []string
	for i := range m.items {
		if m.checked[i] {
			ids = append(ids, m.items[i].ID)
		}
	}
	return ids, nil
}

func (m *pickerModel) Init() bubbletea.Cmd { return nil }

func (m *pickerModel) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	switch key := msg.(type) {
	case bubbletea.KeyMsg:
		switch key.String() {
		case "ctrl+c", "q", "esc":
			m.canceled = true
			return m, bubbletea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.clampOffset()
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				m.clampOffset()
			}
		case " ", "space":
			m.checked[m.cursor] = !m.checked[m.cursor]
		case "a":
			for i := range m.items {
				m.checked[i] = true
			}
		case "n":
			m.checked = map[int]bool{}
		case "home", "g":
			m.cursor = 0
			m.clampOffset()
		case "end", "G":
			m.cursor = len(m.items) - 1
			m.clampOffset()
		case "enter":
			m.confirmed = true
			return m, bubbletea.Quit
		}
	}
	return m, nil
}

func (m *pickerModel) clampOffset() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visibleRows {
		m.offset = m.cursor - visibleRows + 1
	}
}

func (m *pickerModel) View() string {
	selectedCount := 0
	for _, on := range m.checked {
		if on {
			selectedCount++
		}
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf(
		" Интерактивная синхронизация — выбрано %d / %d", selectedCount, len(m.items))))
	b.WriteString("\n")

	end := m.offset + visibleRows
	if end > len(m.items) {
		end = len(m.items)
	}
	for i := m.offset; i < end; i++ {
		it := m.items[i]
		mark := "[ ]"
		if m.checked[i] {
			mark = "[x]"
		}
		line := fmt.Sprintf("%s %-38s %-14s %s %s",
			mark, truncateStr(it.Title, 38), truncateStr(it.Workspace, 14),
			it.CreatedAt, downloadBadge(it.Downloaded))
		if i == m.cursor {
			line = cursorStyle.Render("> ") + line
		} else {
			line = "  " + line
		}
		if m.checked[i] {
			line = selectedStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	if m.offset > 0 || end < len(m.items) {
		b.WriteString(helpStyle.Render(fmt.Sprintf("  (%d-%d из %d)\n", m.offset+1, end, len(m.items))))
	}

	b.WriteString(helpStyle.Render(
		"\n ↑/↓ j/k — движение · space — выбор · a — все · n — снять · enter — синхронизировать · q — отмена"))
	return b.String()
}

func downloadBadge(downloaded bool) string {
	if downloaded {
		return dlYesStyle.Render("[скачано]")
	}
	return dlNoStyle.Render("[новый]   ")
}

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return ""
	}
	return string(r[:n-1]) + "…"
}
