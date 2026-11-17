package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// modeNotes edits the selected lecture's notes in a side pane. There is no
// save key: notes are written shortly after typing stops, and on close.
const modeNotes = "lecture-notes"

// notesDelay is how long typing must pause before the notes are saved.
const notesDelay = 400 * time.Millisecond

// notesSaveMsg asks to save the notes, unless more was typed since (gen).
type notesSaveMsg struct{ gen int }

// notesEditor holds the open notes. It is shared between copies of the
// model, as the textarea keeps state behind pointers anyway.
type notesEditor struct {
	area    textarea.Model
	lecture int    // ID of the lecture whose notes are open.
	saved   string // The notes as last written to the database.
	gen     int    // Bumped on every change, to debounce saving.
}

func newNotesArea() textarea.Model {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.Placeholder = "Write your notes in Markdown…"
	ta.MaxHeight = 0 // No limit on the number of lines.
	ta.MaxWidth = 0
	ta.CharLimit = 0
	s := ta.Styles()
	text := lipgloss.NewStyle().Foreground(ink)
	s.Focused = textarea.StyleState{
		Base:        lipgloss.NewStyle(),
		Text:        text,
		CursorLine:  text,
		EndOfBuffer: lipgloss.NewStyle().Foreground(faint),
		Placeholder: lipgloss.NewStyle().Foreground(faint).Italic(true),
		Prompt:      lipgloss.NewStyle(),
	}
	s.Blurred = s.Focused
	s.Cursor = textarea.CursorStyle{Color: cyan, Shape: tea.CursorBlock}
	ta.SetStyles(s)
	return ta
}

// openNotes opens the selected lecture's notes, ready to type.
func (m *Model) openNotes() tea.Cmd {
	li := m.dashLecture()
	if li < 0 {
		return nil
	}
	lec := m.lectures[li]
	m.notes = &notesEditor{area: newNotesArea(), lecture: lec.ID, saved: lec.Notes}
	m.notes.area.SetValue(lec.Notes)
	m.notes.area.MoveToBegin()
	m.mode = modeNotes
	m.sizeNotes()
	return m.notes.area.Focus()
}

// closeNotes saves the notes and goes back to the lectures.
func (m *Model) closeNotes() {
	if err := m.saveNotes(); err != nil {
		m.notify("", err)
		return
	}
	m.notes.area.Blur()
	m.notes = nil
	m.mode = modeNormal
	m.notify("Notes saved.", nil)
}

// saveNotes writes the notes if they changed since the last save.
func (m *Model) saveNotes() error {
	if m.notes == nil {
		return nil
	}
	value := m.notes.area.Value()
	if value == m.notes.saved {
		return nil
	}
	if _, err := m.db.Exec("UPDATE lectures SET notes = ? WHERE id = ?", value, m.notes.lecture); err != nil {
		return err
	}
	m.notes.saved = value
	for i := range m.lectures {
		if m.lectures[i].ID == m.notes.lecture {
			m.lectures[i].Notes = value
		}
	}
	return nil
}

// updateNotes hands a key or a paste to the editor, and schedules a save
// when the notes changed.
func (m *Model) updateNotes(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "esc" {
		m.closeNotes()
		return nil
	}
	before := m.notes.area.Value()
	var cmd tea.Cmd
	m.notes.area, cmd = m.notes.area.Update(msg)
	if m.notes.area.Value() == before {
		return cmd
	}
	m.notes.gen++
	gen := m.notes.gen
	return tea.Batch(cmd, tea.Tick(notesDelay, func(time.Time) tea.Msg { return notesSaveMsg{gen} }))
}

// sizeNotes fits the editor to its pane.
func (m *Model) sizeNotes() {
	if m.notes == nil {
		return
	}
	l := m.layout()
	w, h := notesAreaSize(l)
	m.notes.area.SetWidth(w)
	m.notes.area.SetHeight(h)
}

// notesAreaSize is the room for the text: the pane minus its border,
// padding, two header lines and a rule above and below, and the footer.
func notesAreaSize(l layout) (width, height int) {
	return max(1, l.notesWidth-6), max(1, l.bodyHeight-2-6)
}

func (m Model) notesPane(l layout) []string {
	width := l.notesWidth
	bg := base
	inner := max(1, width-6)
	lec, ci := Lecture{}, 0
	for _, x := range m.lectures {
		if x.ID == m.notes.lecture {
			lec = x
		}
	}
	for i, c := range m.courses {
		if c.ID == lec.CourseID {
			ci = i
		}
	}
	accent := courseColor(ci)

	status := st(lime, bg).Render("✓ ") + st(faint, bg).Italic(true).Render("saved")
	if m.notes.area.Value() != m.notes.saved {
		status = st(amber, bg).Render("● ") + st(faint, bg).Italic(true).Render("saving…")
	}
	label := ""
	if ci < len(m.courses) {
		label = st(accent, bg).Render("◆ ") + st(muted, bg).Bold(true).Render(m.courses[ci].label()) + st(line, bg).Render("  ·  ")
	}
	head := label + st(faint, bg).Bold(true).Render("N O T E S")
	title := gradient(fitted(lec.Title, max(1, inner-12)), st(ink, bg).Bold(true), accent, lipgloss.Lighten(accent, 0.4))
	date := st(faint, bg).Italic(true).Render(lec.Date.Format("Mon 02/01"))
	rule := st(line, bg).Render(strings.Repeat("─", inner))

	lines := []string{
		spread(head, status, inner, bg),
		spread(title, date, inner, bg),
		rule,
	}
	_, areaHeight := notesAreaSize(l)
	area := strings.Split(m.notes.area.View(), "\n")
	for i := 0; i < areaHeight; i++ {
		s := ""
		if i < len(area) {
			s = area[i]
		}
		lines = append(lines, fill(s, inner, bg))
	}
	words := len(strings.Fields(m.notes.area.Value()))
	count := st(faint, bg).Italic(true).Render(plural1(words, "word"))
	lines = append(lines, rule,
		spread(hints(bg, "esc", "close", "enter", "new line"), count+st(faint, bg).Italic(true).Render(" · autosaved"), inner, bg))

	box := lipgloss.NewStyle().Width(width).Padding(0, 2).
		Border(lipgloss.RoundedBorder()).BorderForeground(accent).BorderBackground(bg).
		Render(strings.Join(lines, "\n"))
	out := strings.Split(box, "\n")
	for len(out) < l.bodyHeight {
		out = append(out, fill("", width, bg))
	}
	return out[:l.bodyHeight]
}

func plural1(n int, word string) string { return fmt.Sprintf("%d %s%s", n, word, plural(n)) }
