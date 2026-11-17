package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// Each course gets its own accent, like the coloured icons of a Notion page.
var courseColors = []color.Color{
	cyan, pink, lime, violet, amber,
	lipgloss.Color("#FF8A3D"), lipgloss.Color("#4D8DFF"), lipgloss.Color("#2DE1C2"),
}

func courseColor(index int) color.Color { return courseColors[index%len(courseColors)] }

// Column widths of a lecture row, besides the flexible title.
const (
	dateWidth   = 11
	statusWidth = 13
	blockGap    = 2
)

func statusColor(value string) color.Color {
	switch value {
	case "done":
		return lime
	case "ongoing":
		return amber
	case "no exo":
		return faint
	}
	return red // none, to do
}

// statusPill renders a Notion-like tag: a dot and its value on a tinted,
// rounded background. rowBg is the background around the tag.
func statusPill(value string, rowBg color.Color) string {
	c := statusColor(value)
	tint := lipgloss.Darken(c, 0.72)
	text := lipgloss.Lighten(c, 0.35)
	if value == "no exo" {
		tint, text = keyBg, muted
	}
	return st(tint, rowBg).Render("\ue0b6") + st(c, tint).Render("●") +
		st(text, tint).Render(" "+value) + st(tint, rowBg).Render("\ue0b4")
}

// weekStart returns the Monday starting t's week.
func weekStart(t time.Time) time.Time {
	y, mo, d := t.Date()
	day := time.Date(y, mo, d, 0, 0, 0, 0, time.Local)
	return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
}

func (m Model) dashBodyHeight(l layout) int {
	// Summary box, the gap below it, and a last line for the scroll hint.
	return max(0, l.bodyHeight-5)
}

// blockWidth is the width of a course block for the given number of columns.
func blockWidth(width, cols int) int {
	return (width - (cols-1)*blockGap) / cols
}

// courseBlock renders a course and its lectures. cursor is the line of the
// selected row in the block, or -1 when the block is not focused.
func (m Model) courseBlock(ci, width int, now time.Time) (lines []string, cursor int) {
	bg := base
	course := m.courses[ci]
	accent := courseColor(ci)
	focused := ci == m.dashCourse
	inner := max(1, width-4)
	ls := m.lecturesOf(course.ID)
	cursor = -1

	// Header: course name, count and progress.
	theory, exo, exoTotal := 0, 0, 0
	for _, li := range ls {
		if m.lectures[li].Theory == "done" {
			theory++
		}
		if m.lectures[li].Exo != "no exo" {
			exoTotal++
			if m.lectures[li].Exo == "done" {
				exo++
			}
		}
	}
	nameStyle := st(muted, bg).Bold(true)
	if focused {
		nameStyle = st(ink, bg).Bold(true)
	}
	title := nameStyle.Render(fitted(course.Name, max(4, inner-34)))
	if course.Nick != "" {
		title = nameStyle.Render(course.Nick) + st(faint, bg).Italic(true).Render(fitted("  "+course.Name, max(1, inner-36-lipgloss.Width(course.Nick))))
	}
	left := st(accent, bg).Render("◆ ") + title +
		st(faint, bg).Italic(true).Render(fmt.Sprintf("  %d lecture%s", len(ls), plural(len(ls))))
	right := ""
	if total := len(ls) + exoTotal; total > 0 {
		done := theory + exo
		const barWidth = 10
		filled := barWidth * done / total
		right = gradient(strings.Repeat("━", filled), st(ink, bg), accent, lipgloss.Lighten(accent, 0.4)) +
			st(line, bg).Render(strings.Repeat("─", barWidth-filled)) +
			st(ink, bg).Bold(true).Render(fmt.Sprintf(" %3d%%", 100*done/total))
	}
	lines = append(lines, spread(left, right, inner, bg))

	titleWidth := max(4, inner-2-dateWidth-2*statusWidth)
	label := st(faint, bg).Bold(true)
	lines = append(lines,
		st(line, bg).Render(strings.Repeat("─", max(0, inner))),
		label.Render("  "+fmt.Sprintf("%-*s", titleWidth, "TITLE")+fmt.Sprintf("%-*s", dateWidth, "DATE")+
			fmt.Sprintf("%-*s", statusWidth, "THEORY")+fmt.Sprintf("%-*s", statusWidth, "EXO")))

	thisWeek := weekStart(now)
	for r, li := range ls {
		lec := m.lectures[li]
		selected := focused && r == m.dashRow
		rbg := bg
		marker := st(ink, rbg).Render("  ")
		title := st(muted, rbg)
		if selected {
			rbg = selection
			cursor = len(lines)
			marker = st(accent, rbg).Render("▌") + st(ink, rbg).Render(" ")
			title = st(ink, rbg).Bold(true)
		}
		dateStyle := st(muted, rbg)
		switch week := weekStart(lec.Date); {
		case week.Equal(thisWeek):
			dateStyle = st(cyan, rbg).Bold(true)
		case week.After(thisWeek):
			dateStyle = st(faint, rbg).Italic(true)
		}
		date := dateStyle.Render(lec.Date.Format("Mon 02/01"))
		name := title.Render(fitted(lec.Title, titleWidth-1))
		if lec.Notes != "" {
			// A pencil tells which lectures have notes.
			name = title.Render(fitted(lec.Title, titleWidth-4)) + st(accent, rbg).Render(" ✎")
		}
		lines = append(lines, marker+
			fill(name, titleWidth, rbg)+
			fill(date, dateWidth, rbg)+
			fill(statusPill(lec.Theory, rbg), statusWidth, rbg)+
			fill(statusPill(lec.Exo, rbg), statusWidth, rbg))
	}

	// The "+ New lecture" row, Notion's "+ New page".
	newRow := st(ink, bg).Render("  ") + st(faint, bg).Render("+ ") + st(faint, bg).Italic(true).Render("New lecture")
	if focused && m.dashRow == len(ls) {
		cursor = len(lines)
		newRow = st(accent, selection).Render("▌ ") + st(accent, selection).Bold(true).Render("+ ") +
			st(ink, selection).Italic(true).Render("New lecture") +
			st(faint, selection).Italic(true).Render(fmt.Sprintf("  · %s", m.nextDate(course.ID).Format("Mon 02/01")))
		newRow = fill(newRow, inner, selection)
	}
	lines = append(lines, newRow)

	border := line
	if focused {
		border = accent
	}
	box := lipgloss.NewStyle().Width(width).Padding(0, 1).
		Border(lipgloss.RoundedBorder()).BorderForeground(border).BorderBackground(bg).
		Render(strings.Join(lines, "\n"))
	if cursor >= 0 {
		cursor++ // Top border.
	}
	return strings.Split(box, "\n"), cursor
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// dashboardLines lays the course blocks out in a grid and reports the line
// of the selected row and the total height.
// dashboardLines lays the course blocks out in a grid. It reports the line
// of the selected row, the lines the focused block spans, and the height.
func (m Model) dashboardLines(l layout) (lines []string, cursor, blockTop, blockBottom, total int) {
	if len(m.courses) == 0 {
		return nil, 0, 0, 0, 0
	}
	cols := dashColumns(l.listWidth)
	width := blockWidth(l.listWidth, cols)
	now := time.Now()
	gap := st(ink, base).Render(strings.Repeat(" ", blockGap))
	for start := 0; start < len(m.courses); start += cols {
		var blocks [][]string
		height := 0
		for ci := start; ci < min(len(m.courses), start+cols); ci++ {
			b, c := m.courseBlock(ci, width, now)
			if c >= 0 {
				cursor = len(lines) + c
			}
			if ci == m.dashCourse {
				blockTop, blockBottom = len(lines), len(lines)+len(b)
			}
			blocks = append(blocks, b)
			height = max(height, len(b))
		}
		for y := 0; y < height; y++ {
			row := ""
			for i, b := range blocks {
				if i > 0 {
					row += gap
				}
				cell := ""
				if y < len(b) {
					cell = b[y]
				}
				row += fill(cell, width, base)
			}
			lines = append(lines, fill(row, l.listWidth, base))
		}
		lines = append(lines, fill("", l.listWidth, base))
	}
	return lines, cursor, blockTop, blockBottom, len(lines)
}

// summaryBox sums up progress over every course.
func (m Model) summaryBox(width int) []string {
	bg := base
	inner := max(1, width-4)
	now := time.Now()
	thisWeek := weekStart(now)
	theory, exo, exoTotal, week, late := 0, 0, 0, 0, 0
	for _, lec := range m.lectures {
		if lec.Theory == "done" {
			theory++
		}
		if lec.Exo != "no exo" {
			exoTotal++
			if lec.Exo == "done" {
				exo++
			} else if weekStart(lec.Date).Before(thisWeek) {
				late++
			}
		}
		if weekStart(lec.Date).Equal(thisWeek) {
			week++
		}
	}
	bar := func(label string, done, total int, stops ...color.Color) string {
		const w = 12
		filled := 0
		if total > 0 {
			filled = w * done / total
		}
		return st(muted, bg).Italic(true).Render(label+" ") +
			gradient(strings.Repeat("━", filled), st(ink, bg), stops...) +
			st(line, bg).Render(strings.Repeat("─", w-filled)) +
			st(ink, bg).Bold(true).Render(fmt.Sprintf(" %d", done)) + st(faint, bg).Render(fmt.Sprintf("/%d", total))
	}
	sep := st(line, bg).Render("   │   ")
	content := bar("theory", theory, len(m.lectures), cyan, violet) + sep +
		bar("exos", exo, exoTotal, violet, pink) + sep +
		st(cyan, bg).Bold(true).Render(fmt.Sprint(week)) + st(muted, bg).Italic(true).Render(" this week")
	lateStyle := st(faint, bg)
	if late > 0 {
		lateStyle = st(red, bg).Bold(true)
	}
	content += sep + lateStyle.Render(fmt.Sprint(late)) + st(muted, bg).Italic(true).Render(" exos late")
	box := lipgloss.NewStyle().Width(width).Padding(0, 1).
		Border(lipgloss.RoundedBorder()).BorderForeground(line).BorderBackground(bg).
		Render(fill(content, inner, bg))
	return strings.Split(box, "\n")
}

func (m Model) lecturesPane(l layout) []string {
	var lines []string
	if m.editing() || m.mode == modeLectureDelete {
		lines = m.commandBox(l.listWidth)
	} else {
		lines = m.summaryBox(l.listWidth)
	}
	lines = append(lines, fill("", l.listWidth, base))
	height := m.dashBodyHeight(l)
	grid, _, _, _, total := m.dashboardLines(l)
	if len(m.courses) == 0 {
		bg := base
		empty := []string{
			gradient("◇  ◆  ◇", st(ink, bg), brand...), "",
			gradient("Your dashboard needs a course.", st(ink, bg).Bold(true).Italic(true), brand...), "",
			st(faint, bg).Italic(true).Render("Press ") + keycap("tab", bg) + st(faint, bg).Italic(true).Render(" to go to Courses and add one."),
		}
		for i := 0; i < max(0, height/3-2); i++ {
			lines = append(lines, fill("", l.listWidth, bg))
		}
		for _, e := range empty {
			pad := max(0, (l.listWidth-lipgloss.Width(e))/2)
			lines = append(lines, fill(st(ink, bg).Render(strings.Repeat(" ", pad))+e, l.listWidth, bg))
		}
	} else {
		end := min(total, m.dashOffset+height)
		lines = append(lines, grid[min(m.dashOffset, end):end]...)
	}
	for len(lines) < l.bodyHeight {
		lines = append(lines, fill("", l.listWidth, base))
	}
	lines = lines[:l.bodyHeight]
	// Scroll hints when blocks continue above or below.
	if m.dashOffset > 0 && l.bodyHeight > 3 {
		lines[3] = spread(fill("", 0, base), st(faint, base).Italic(true).Render("more above ")+st(cyan, base).Render("↑ "), l.listWidth, base)
	}
	if m.dashOffset+height < total && l.bodyHeight > 0 {
		lines[l.bodyHeight-1] = spread(fill("", 0, base), st(faint, base).Italic(true).Render("more below ")+st(cyan, base).Render("↓ "), l.listWidth, base)
	}
	return lines
}
