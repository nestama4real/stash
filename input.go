package main

import (
	"image/color"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The text being typed is m.input; the cursor sits m.inputBack runes before
// its end. Counting from the end means a freshly filled input starts with
// the cursor at the end, wherever the value came from.

func (m Model) caret() int {
	n := len([]rune(m.input))
	return n - min(max(0, m.inputBack), n)
}

func (m *Model) setCaret(pos int) {
	n := len([]rune(m.input))
	m.inputBack = n - max(0, min(n, pos))
}

// inputChanged keeps the filter, which is typed live, in step.
func (m *Model) inputChanged() {
	if m.mode == modeFilter {
		m.filter = m.input
		m.cursor = 0
	}
}

// typeText inserts s at the cursor.
func (m *Model) typeText(s string) {
	s = cleanName(s)
	if s == "" {
		return
	}
	r := []rune(m.input)
	pos := m.caret()
	m.input = string(r[:pos]) + s + string(r[pos:])
	m.inputChanged()
}

// cut removes the runes between from and to, leaving the cursor at from.
func (m *Model) cut(from, to int) {
	r := []rune(m.input)
	from, to = max(0, from), min(len(r), to)
	if from >= to {
		return
	}
	m.input = string(r[:from]) + string(r[to:])
	m.setCaret(from)
	m.inputChanged()
}

func (m *Model) backspace() { m.cut(m.caret()-1, m.caret()) }

// wordLeft and wordRight find where the previous or next word starts or ends.
func (m Model) wordLeft() int {
	r, i := []rune(m.input), m.caret()
	for i > 0 && unicode.IsSpace(r[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(r[i-1]) {
		i--
	}
	return i
}

func (m Model) wordRight() int {
	r, i := []rune(m.input), m.caret()
	for i < len(r) && unicode.IsSpace(r[i]) {
		i++
	}
	for i < len(r) && !unicode.IsSpace(r[i]) {
		i++
	}
	return i
}

// editKey applies the usual text-editing keys, as in a shell or a browser.
// It reports whether the key was one of them.
func (m *Model) editKey(k string) bool {
	switch k {
	case "left", "ctrl+b":
		m.setCaret(m.caret() - 1)
	case "right", "ctrl+f":
		m.setCaret(m.caret() + 1)
	case "home", "ctrl+a":
		m.setCaret(0)
	case "end", "ctrl+e":
		m.setCaret(len([]rune(m.input)))
	case "ctrl+left", "alt+left", "alt+b":
		m.setCaret(m.wordLeft())
	case "ctrl+right", "alt+right", "alt+f":
		m.setCaret(m.wordRight())
	case "backspace", "ctrl+h":
		m.backspace()
	case "delete", "ctrl+d":
		m.cut(m.caret(), m.caret()+1)
	case "ctrl+w", "alt+backspace", "ctrl+backspace":
		m.cut(m.wordLeft(), m.caret())
	case "alt+delete", "alt+d", "ctrl+delete":
		m.cut(m.caret(), m.wordRight())
	case "ctrl+u":
		m.cut(0, m.caret())
	case "ctrl+k":
		m.cut(m.caret(), len([]rune(m.input)))
	default:
		return false
	}
	return true
}

// inputField renders value with a block cursor after back runes from the
// end, scrolled so the cursor always shows within width cells.
func inputField(value string, back, width int, fg, bg, accent color.Color) string {
	r := []rune(value)
	pos := len(r) - min(max(0, back), len(r))
	before, at, after := string(r[:pos]), " ", ""
	if pos < len(r) {
		at, after = string(r[pos]), string(r[pos+1:])
	}
	room := max(1, width-lipgloss.Width(at))
	bw, aw := lipgloss.Width(before), lipgloss.Width(after)
	if bw+aw > room {
		// Show some text after the cursor, and as much as fits before it.
		right := min(aw, max(room/3, room-bw))
		left := room - right
		if bw > left {
			before = ansi.TruncateLeft(before, bw-left+1, "…")
		}
		if aw > right {
			after = ansi.Truncate(after, right, "…")
		}
	}
	text := st(fg, bg)
	return text.Render(before) + st(onAccent, accent).Render(at) + text.Render(after)
}

// trimmed is the input without surrounding spaces.
func (m Model) trimmed() string { return strings.TrimSpace(m.input) }
