package main

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	gansi "charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
)

// modeNotes shows the selected lecture's notes in a side pane, rendered,
// or in an editor while they are being written. Leaving the editor saves.
const modeNotes = "lecture-notes"

// notesEditor holds the open notes. It is shared between copies of the
// model, as the textarea keeps state behind pointers anyway.
type notesEditor struct {
	area     textarea.Model
	editing  bool
	lecture  int      // ID of the lecture whose notes are open.
	saved    string   // The notes as last written to the database.
	rendered []string // The saved notes as Markdown, rendered…
	width    int      // …at this width.
	scroll   int      // First rendered line shown.
}

func newNotesArea(accent color.Color) textarea.Model {
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
	s.Cursor = textarea.CursorStyle{Color: accent, Shape: tea.CursorBlock}
	ta.SetStyles(s)
	return ta
}

// notesCourse is the index in m.courses of the open notes' course.
func (m Model) notesCourse() int {
	for _, l := range m.lectures {
		if l.ID == m.notes.lecture {
			for i, c := range m.courses {
				if c.ID == l.CourseID {
					return i
				}
			}
		}
	}
	return 0
}

// openNotes opens the selected lecture's notes: rendered when there are
// some, in the editor when there are none yet.
func (m *Model) openNotes() tea.Cmd {
	li := m.dashLecture()
	if li < 0 {
		return nil
	}
	lec := m.lectures[li]
	m.notes = &notesEditor{area: newNotesArea(courseColor(m.dashCourse)), lecture: lec.ID, saved: lec.Notes}
	m.mode = modeNotes
	if lec.Notes == "" {
		return m.editNotes()
	}
	m.sizeNotes()
	return nil
}

// editNotes switches the pane to the editor.
func (m *Model) editNotes() tea.Cmd {
	m.notes.editing = true
	m.notes.area.SetValue(m.notes.saved)
	m.notes.area.MoveToBegin()
	m.sizeNotes()
	return m.notes.area.Focus()
}

// stopEditing saves the notes and shows them rendered.
func (m *Model) stopEditing() {
	if err := m.saveNotes(); err != nil {
		m.notify("", err)
		return
	}
	m.notes.area.Blur()
	m.notes.editing = false
	m.notes.width = 0 // Render again.
	m.sizeNotes()
	m.notify("Notes saved.", nil)
}

// saveNotes writes the notes being edited if they changed.
func (m *Model) saveNotes() error {
	if m.notes == nil || !m.notes.editing {
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

// updateNotes handles a key or a paste while the notes are open.
func (m *Model) updateNotes(msg tea.Msg) tea.Cmd {
	key, isKey := msg.(tea.KeyPressMsg)
	if m.notes.editing {
		if isKey && key.String() == "esc" {
			m.stopEditing()
			return nil
		}
		var cmd tea.Cmd
		m.notes.area, cmd = m.notes.area.Update(msg)
		return cmd
	}
	if !isKey {
		return nil
	}
	_, height := notesAreaSize(m.layout())
	switch key.String() {
	case "esc", "q":
		m.notes = nil
		m.mode = modeNormal
		return nil
	case "enter", "e", "i":
		return m.editNotes()
	case "down", "j":
		m.notes.scroll++
	case "up", "k":
		m.notes.scroll--
	case "pgdown", "space", "ctrl+d":
		m.notes.scroll += height / 2
	case "pgup", "ctrl+u":
		m.notes.scroll -= height / 2
	case "g", "home":
		m.notes.scroll = 0
	case "G", "end":
		m.notes.scroll = len(m.notes.rendered)
	}
	m.notes.scroll = max(0, min(m.notes.scroll, len(m.notes.rendered)-height))
	return nil
}

// sizeNotes fits the editor to its pane, and renders the notes again when
// the pane changed width.
func (m *Model) sizeNotes() {
	if m.notes == nil {
		return
	}
	w, h := notesAreaSize(m.layout())
	if m.notes.editing {
		m.notes.area.SetWidth(w)
		m.notes.area.SetHeight(h)
		return
	}
	if m.notes.width != w {
		m.notes.rendered = renderMarkdown(m.notes.saved, w, courseColor(m.notesCourse()))
		m.notes.width = w
	}
	m.notes.scroll = max(0, min(m.notes.scroll, len(m.notes.rendered)-h))
}

// notesAreaSize is the room for the text: the pane minus its border,
// padding, two header lines and a rule above and below, and the footer.
func notesAreaSize(l layout) (width, height int) {
	return max(1, l.notesWidth-6), max(1, l.bodyHeight-2-6)
}

func hex(c color.Color) *string {
	r, g, b, _ := c.RGBA()
	s := fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
	return &s
}

// markdownStyle dresses Markdown in the app's palette, headings and marks
// in the course's colour.
func markdownStyle(accent color.Color) gansi.StyleConfig {
	yes := true
	zero, one, two := uint(0), uint(1), uint(2)
	bar := "▌ "
	soft := lipgloss.Lighten(accent, 0.35)
	heading := func(prefix string, c color.Color) gansi.StyleBlock {
		return gansi.StyleBlock{StylePrimitive: gansi.StylePrimitive{Prefix: prefix, Color: hex(c), Bold: &yes}}
	}
	return gansi.StyleConfig{
		Document:  gansi.StyleBlock{StylePrimitive: gansi.StylePrimitive{Color: hex(ink)}, Margin: &zero},
		Paragraph: gansi.StyleBlock{StylePrimitive: gansi.StylePrimitive{Color: hex(ink)}},
		Heading:   gansi.StyleBlock{StylePrimitive: gansi.StylePrimitive{BlockSuffix: "\n", Color: hex(accent), Bold: &yes}},
		H1: gansi.StyleBlock{StylePrimitive: gansi.StylePrimitive{
			Prefix: " ", Suffix: " ", Color: hex(onAccent), BackgroundColor: hex(accent), Bold: &yes,
		}},
		H2: heading("▍", accent),
		H3: heading("› ", soft),
		H4: heading("", soft),
		H5: heading("", muted),
		H6: heading("", muted),
		BlockQuote: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{Color: hex(muted), Italic: &yes},
			Indent:         &one, IndentToken: &bar,
		},
		List:           gansi.StyleList{LevelIndent: 2},
		Item:           gansi.StylePrimitive{BlockPrefix: "• "},
		Enumeration:    gansi.StylePrimitive{BlockPrefix: ". "},
		Task:           gansi.StyleTask{Ticked: "✓ ", Unticked: "☐ "},
		Strong:         gansi.StylePrimitive{Bold: &yes, Color: hex(soft)},
		Emph:           gansi.StylePrimitive{Italic: &yes},
		Strikethrough:  gansi.StylePrimitive{CrossedOut: &yes, Color: hex(faint)},
		HorizontalRule: gansi.StylePrimitive{Color: hex(line), Format: "\n────────\n"},
		Link:           gansi.StylePrimitive{Color: hex(faint), Underline: &yes},
		LinkText:       gansi.StylePrimitive{Color: hex(accent), Bold: &yes},
		Image:          gansi.StylePrimitive{Color: hex(faint), Underline: &yes},
		ImageText:      gansi.StylePrimitive{Color: hex(muted), Format: "Image: {{.text}} →"},
		Code: gansi.StyleBlock{StylePrimitive: gansi.StylePrimitive{
			Prefix: " ", Suffix: " ", Color: hex(soft), BackgroundColor: hex(keyBg),
		}},
		CodeBlock: gansi.StyleCodeBlock{StyleBlock: gansi.StyleBlock{
			StylePrimitive: gansi.StylePrimitive{Color: hex(muted)}, Margin: &two,
		}},
		Table: gansi.StyleTable{
			StyleBlock:      gansi.StyleBlock{StylePrimitive: gansi.StylePrimitive{Color: hex(ink)}},
			CenterSeparator: strPtr("┼"), ColumnSeparator: strPtr("│"), RowSeparator: strPtr("─"),
		},
		DefinitionDescription: gansi.StylePrimitive{BlockPrefix: "\n› "},
	}
}

func strPtr(s string) *string { return &s }

// renderMarkdown renders notes to lines of at most width cells.
func renderMarkdown(src string, width int, accent color.Color) []string {
	if strings.TrimSpace(src) == "" {
		return nil
	}
	r, err := glamour.NewTermRenderer(glamour.WithStyles(markdownStyle(accent)), glamour.WithWordWrap(width))
	out := ""
	if err == nil {
		out, err = r.Render(src)
	}
	if err != nil {
		// Show the notes as typed rather than nothing.
		out = lipgloss.NewStyle().Width(width).Render(src)
	}
	lines := strings.Split(strings.Trim(out, "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return lines
}

func (m Model) notesPane(l layout) []string {
	width := l.notesWidth
	bg := base
	inner := max(1, width-6)
	lec := Lecture{}
	for _, x := range m.lectures {
		if x.ID == m.notes.lecture {
			lec = x
		}
	}
	ci := m.notesCourse()
	accent := courseColor(ci)
	n := m.notes

	var status string
	switch {
	case !n.editing:
		status = st(faint, bg).Italic(true).Render("preview")
	case n.area.Value() != n.saved:
		status = st(accent, bg).Render("● ") + st(faint, bg).Italic(true).Render("unsaved · esc saves")
	default:
		status = st(accent, bg).Render("✎ ") + st(faint, bg).Italic(true).Render("editing")
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
	_, height := notesAreaSize(l)
	var body []string
	if n.editing {
		body = strings.Split(n.area.View(), "\n")
	} else if len(n.rendered) == 0 {
		body = []string{st(faint, bg).Italic(true).Render("No notes yet.")}
	} else {
		body = n.rendered[min(n.scroll, len(n.rendered)):]
	}
	for i := 0; i < height; i++ {
		s := ""
		if i < len(body) {
			s = body[i]
		}
		lines = append(lines, fill(s, inner, bg))
	}

	var keys, right string
	if n.editing {
		keys = hints(bg, "esc", "save")
		right = st(faint, bg).Italic(true).Render(fmt.Sprintf("%d words", len(strings.Fields(n.area.Value()))))
	} else {
		keys = hints(bg, "enter", "edit", "↑↓", "scroll", "esc", "close")
		if len(n.rendered) > height {
			right = st(faint, bg).Italic(true).Render(fmt.Sprintf("%d%%", 100*min(n.scroll+height, len(n.rendered))/len(n.rendered)))
		}
	}
	lines = append(lines, rule, spread(keys, right, inner, bg))

	box := lipgloss.NewStyle().Width(width).Padding(0, 2).
		Border(lipgloss.RoundedBorder()).BorderForeground(accent).BorderBackground(bg).
		Render(strings.Join(lines, "\n"))
	out := strings.Split(box, "\n")
	for len(out) < l.bodyHeight {
		out = append(out, fill("", width, bg))
	}
	return out[:l.bodyHeight]
}
