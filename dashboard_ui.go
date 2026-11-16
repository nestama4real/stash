package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// weekLabel renders "Week 4 · 16 – 22 Nov" for the header.
func (m Model) weekLabel(bg color.Color) string {
	week := m.viewWeek()
	n := m.weekNumber(week)
	label := " Week – "
	if n > 0 {
		label = fmt.Sprintf(" Week %d ", n)
	}
	out := st(ink, bg).Render("  ") + pill(label, brand...) +
		st(faint, bg).Italic(true).Render("  "+week.Format("02 Jan")+" – "+week.AddDate(0, 0, 6).Format("02 Jan"))
	if !week.Equal(weekStart(time.Now())) {
		out += st(amber, bg).Italic(true).Render("  · not this week, ") + st(amber, bg).Bold(true).Render("w") +
			st(amber, bg).Italic(true).Render(" to come back")
	}
	return out
}

// workTabs renders the Exercises / Theory switch with what is left in each.
func (m Model) workTabs(width int) string {
	bg := base
	left := func(tab int) int {
		saved := m.dashTab
		m.dashTab = tab
		n := 0
		for _, col := range []int{colThisWeek, colPrevious} {
			for _, li := range m.workItems(col) {
				if m.workField(m.lectures[li]) != "done" {
					n++
				}
			}
		}
		m.dashTab = saved
		return n
	}
	tab := func(id int, key, icon, name string) string {
		count := fmt.Sprintf(" %d ", left(id))
		if m.dashTab == id {
			return pill(" "+icon+" "+name+" ", brand...) + st(onAccent, lipgloss.Lighten(pink, 0.3)).Bold(true).Render(count)
		}
		return st(muted, keyBg).Render(" "+icon+" "+name+" ") + st(faint, keyBg).Render(count) +
			st(faint, bg).Render(" ") + st(faint, bg).Bold(true).Render(key)
	}
	tabs := st(ink, bg).Render(" ") + tab(tabExercises, "x", "", "Exercises") + st(ink, bg).Render("   ") +
		tab(tabTheory, "t", "", "Theory")
	return spread(tabs, hints(bg, "space", "done", "o", "ongoing", "[ ]", "weeks")+st(ink, bg).Render(" "), width, bg)
}

// workColumn renders "This week" or "Previous weeks" with its lectures.
func (m Model) workColumn(col, width, height int) []string {
	bg := base
	items := m.workItems(col)
	focused := m.dashZone == zoneWork && m.dashCol == col
	remaining := 0
	for _, li := range items {
		if m.workField(m.lectures[li]) != "done" {
			remaining++
		}
	}
	var title, count string
	if col == colThisWeek {
		title = "This week"
		count = st(faint, bg).Italic(true).Render(fmt.Sprintf("  %d left · %d done", remaining, len(items)-remaining))
	} else {
		title = "Previous weeks"
		if remaining > 0 {
			count = st(red, bg).Bold(true).Render(fmt.Sprintf("  %d late", remaining))
		} else {
			count = st(lime, bg).Italic(true).Render("  all caught up")
		}
	}
	titleStyle := st(muted, bg).Bold(true)
	rule := st(line, bg)
	if focused {
		titleStyle = st(ink, bg).Bold(true)
		rule = st(cyan, bg)
		if col == colPrevious {
			rule = st(pink, bg)
		}
	}
	head := titleStyle.Render(title) + count
	lines := []string{fill(head, width, bg), rule.Render(strings.Repeat("─", max(0, width)))}

	rows := height - len(lines)
	if len(items) == 0 {
		msg := "Nothing this week."
		if col == colPrevious {
			msg = "No delay. Well done."
		}
		lines = append(lines, fill(st(lime, bg).Render("✓ ")+st(faint, bg).Italic(true).Render(msg), width, bg))
	}
	// Keep the selection in view.
	first := 0
	if focused && m.dashItem >= rows {
		first = m.dashItem - rows + 1
	}
	now := time.Now()
	for r := first; r < len(items) && r < first+rows; r++ {
		lec := m.lectures[items[r]]
		ci := m.courseIndex(lec.CourseID)
		status := m.workField(lec)
		selected := focused && r == m.dashItem
		rbg := bg
		marker := st(ink, rbg).Render("  ")
		titleStyle := st(ink, rbg)
		if selected {
			rbg = selection
			marker = st(courseColor(ci), rbg).Render("▌") + st(ink, rbg).Render(" ")
			titleStyle = st(ink, rbg).Bold(true)
		}
		if status == "done" {
			titleStyle = st(faint, rbg).Strikethrough(true)
		}
		course := st(courseColor(ci), rbg).Bold(true).Render(fmt.Sprintf("%-7s", fitted(m.courses[ci].label(), 7)))
		when := lec.Date.Format("Mon")
		if col == colPrevious {
			if n := m.weekNumber(weekStart(lec.Date)); n > 0 {
				when = fmt.Sprintf("W%d", n)
			}
		} else if lec.Date.Equal(today()) {
			when = "today"
		}
		whenStyle := st(faint, rbg).Italic(true)
		if col == colPrevious && status != "done" && daysBetween(lec.Date, now) > 7 {
			whenStyle = st(red, rbg).Italic(true)
		}
		right := whenStyle.Render(fmt.Sprintf("%6s ", when)) + statusPill(status, rbg)
		titleWidth := max(1, width-2-8-lipgloss.Width(right)-1)
		lines = append(lines, fill(marker+course+st(ink, rbg).Render(" ")+
			fill(titleStyle.Render(fitted(lec.Title, titleWidth)), titleWidth, rbg)+st(ink, rbg).Render(" ")+right, width, rbg))
	}
	if hidden := len(items) - first - rows; hidden > 0 && len(lines) > 2 {
		lines[len(lines)-1] = spread("", st(faint, bg).Italic(true).Render(fmt.Sprintf("%d more ", hidden))+st(cyan, bg).Render("↓"), width, bg)
	}
	for len(lines) < height {
		lines = append(lines, fill("", width, bg))
	}
	return lines[:height]
}

func (m Model) courseIndex(id int) int {
	for i, c := range m.courses {
		if c.ID == id {
			return i
		}
	}
	return 0
}

// planText colours the course a plan starts with, as Notion's links do.
func (m Model) planText(text string, width int, bg color.Color, selected bool) string {
	text = fitted(text, width)
	word, rest, _ := strings.Cut(text, " ")
	base := st(muted, bg)
	if selected {
		base = st(ink, bg)
	}
	if ci := m.courseFor(word); ci >= 0 {
		out := st(courseColor(ci), bg).Bold(true).Render(word)
		if rest != "" {
			out += base.Render(" " + rest)
		}
		return out
	}
	return base.Render(text)
}

// dayColumn renders one day of the planner.
func (m Model) dayColumn(d, width, height int) []string {
	bg := base
	day := m.viewWeek().AddDate(0, 0, d)
	isToday := day.Equal(today())
	focused := m.dashZone == zonePlanner && m.dashDay == d
	name := day.Format("Monday")
	header := st(muted, bg).Bold(true).Render(" " + name + " ")
	switch {
	case isToday:
		header = pill(" "+name+" ", brand...)
	case focused:
		header = st(ink, keyBg).Bold(true).Render(" " + name + " ")
	}
	header += st(faint, bg).Italic(true).Render(" " + day.Format("02/01"))
	rule := st(line, bg)
	if focused {
		rule = st(violet, bg)
	}
	lines := []string{fill(header, width, bg), rule.Render(strings.Repeat("─", max(0, width)))}

	ps := m.plansOn(day)
	now := time.Now()
	var rows []planRow
	for r, pi := range ps {
		p := m.plans[pi]
		selected := focused && r == m.dashPlan
		rbg := bg
		marker := st(ink, rbg).Render(" ")
		if selected {
			rbg = selection
			marker = st(violet, rbg).Render("▌")
		}
		var entry []string
		if p.Start != "" {
			when := strings.ReplaceAll(p.Start, ":", "h")
			if p.End != "" {
				when += " → " + strings.ReplaceAll(p.End, ":", "h")
			}
			whenStyle := st(faint, rbg).Italic(true)
			if isToday && p.Start <= now.Format("15:04") && (p.End == "" || now.Format("15:04") < p.End) {
				whenStyle = st(cyan, rbg).Bold(true).Italic(true)
			}
			entry = append(entry, marker+fill(whenStyle.Render(when), width-1, rbg))
		}
		entry = append(entry, marker+fill(st(faint, rbg).Render("• ")+m.planText(p.Text, width-3, rbg, selected), width-1, rbg))
		rows = append(rows, planRow{entry, selected})
	}
	if focused {
		add := fill(st(faint, bg).Render(" + ")+st(faint, bg).Italic(true).Render("add"), width, bg)
		if m.dashPlan == len(ps) {
			add = st(violet, selection).Render("▌") + st(violet, selection).Bold(true).Render("+ ") +
				st(ink, selection).Italic(true).Render("add") + st(faint, selection).Italic(true).Render("  8-10 CompSec…")
			add = fill(add, width, selection)
		}
		rows = append(rows, planRow{[]string{add}, m.dashPlan == len(ps)})
	} else if len(ps) == 0 {
		rows = append(rows, planRow{[]string{fill(st(line, bg).Italic(true).Render(" free"), width, bg)}, false})
	}

	// Scroll so the selection stays visible; plans are a blank line apart.
	room := height - len(lines)
	first := 0
	for sel := range rows {
		if !rows[sel].selected {
			continue
		}
		for {
			used := 0
			for r := first; r <= sel; r++ {
				used += len(rows[r].lines) + 1
			}
			if used-1 <= room || first == sel {
				break
			}
			first++
		}
	}
	for r := first; r < len(rows); r++ {
		if len(lines)+len(rows[r].lines) > height {
			break
		}
		lines = append(lines, rows[r].lines...)
		if len(lines) < height {
			lines = append(lines, fill("", width, bg))
		}
	}
	for len(lines) < height {
		lines = append(lines, fill("", width, bg))
	}
	return lines[:height]
}

// planRow is a plan, or the "add" row, as rendered in a day.
type planRow struct {
	lines    []string
	selected bool
}

func (m Model) dashboardPane(l layout) []string {
	bg := base
	width := l.width
	var lines []string
	if m.mode == modePlanAdd || m.mode == modePlanEdit || m.mode == modeWeek {
		lines = m.commandBox(width)
	} else {
		lines = []string{fill("", width, bg), m.workTabs(width), fill("", width, bg)}
	}

	// Work gets up to half the body; the planner the rest.
	body := l.bodyHeight - len(lines)
	most := max(len(m.workItems(colThisWeek)), len(m.workItems(colPrevious)), 1)
	workHeight := min(2+most, max(3, body/2-1))
	gap := 3
	colWidth := (width - gap) / 2
	this := m.workColumn(colThisWeek, colWidth, workHeight)
	prev := m.workColumn(colPrevious, width-gap-colWidth, workHeight)
	spacer := st(ink, bg).Render(strings.Repeat(" ", gap))
	for i := range this {
		lines = append(lines, this[i]+spacer+prev[i])
	}

	// The planner: a week of days, wrapping when the terminal is narrow.
	lines = append(lines, fill("", width, bg))
	title := st(violet, bg).Bold(true).Render(" ") + st(ink, bg).Bold(true).Render("Plan") + st(ink, bg).Render(" ")
	help := hints(bg, "a", "add", "e", "edit", "d", "delete", "c", "copy to next week") + st(ink, bg).Render(" ")
	ruleWidth := max(0, width-lipgloss.Width(title)-lipgloss.Width(help)-2)
	head := title + gradient(strings.Repeat("─", ruleWidth), st(ink, bg), line, lipgloss.Darken(line, 0.6)) + st(ink, bg).Render("  ") + help
	if lipgloss.Width(head) > width {
		head = title
	}
	lines = append(lines, fill(head, width, bg), fill("", width, bg))

	cols := max(1, min(7, (width+2)/20))
	dayRows := (7 + cols - 1) / cols
	room := max(0, l.bodyHeight-len(lines))
	cellHeight := max(3, (room-(dayRows-1))/dayRows)
	cellWidth := (width - 2*(cols-1)) / cols
	for start := 0; start < 7 && len(lines) < l.bodyHeight; start += cols {
		var cells [][]string
		for d := start; d < min(7, start+cols); d++ {
			cells = append(cells, m.dayColumn(d, cellWidth, cellHeight))
		}
		for y := 0; y < cellHeight; y++ {
			row := ""
			for i, c := range cells {
				if i > 0 {
					row += st(ink, bg).Render("  ")
				}
				row += fill(c[y], cellWidth, bg)
			}
			lines = append(lines, fill(row, width, bg))
		}
		if start+cols < 7 {
			lines = append(lines, fill("", width, bg))
		}
	}
	for len(lines) < l.bodyHeight {
		lines = append(lines, fill("", width, bg))
	}
	return lines[:l.bodyHeight]
}

// popup is the quick add's dialog: when, then what.
func (m Model) popup(width int) []string {
	bg := surface
	inner := width - 4
	day := m.planDayDate()
	pad := func(s string) string { return fill(s, inner, bg) }
	label := func(s string, active bool) string {
		if active {
			return st(violet, bg).Bold(true).Render(fmt.Sprintf("%-6s", s))
		}
		return st(faint, bg).Italic(true).Render(fmt.Sprintf("%-6s", s))
	}
	title := pill(" + "+day.Format("Monday")+" ", brand...) + st(faint, bg).Italic(true).Render("  "+day.Format("02 January"))
	lines := []string{pad(title), pad("")}

	if m.mode == modePlanTime {
		field := inputField(m.input, m.inputBack, inner-6, ink, bg, violet)
		if m.input == "" {
			field = st(onAccent, violet).Render(" ") + st(faint, bg).Italic(true).Render(" 8 10 · 14h30 · or just type")
		}
		lines = append(lines,
			pad(label("When", true)+field),
			pad(label("What", false)+st(line, bg).Render("…")),
			pad(""),
			pad(hints(bg, "␣", "after the end time", "enter", "next", "esc", "cancel")))
	} else {
		when := st(faint, bg).Italic(true).Render("any time")
		if m.planStart != "" {
			when = st(ink, bg).Bold(true).Render(strings.ReplaceAll(m.planStart, ":", "h"))
			if m.planEnd != "" {
				when += st(faint, bg).Render(" → ") + st(ink, bg).Bold(true).Render(strings.ReplaceAll(m.planEnd, ":", "h"))
			}
		}
		field := inputField(m.input, m.inputBack, inner-6, ink, bg, violet)
		if word, _, _ := strings.Cut(m.input, " "); m.courseFor(word) >= 0 && m.inputBack <= len([]rune(m.input))-len([]rune(word)) {
			// Show the course the text starts with, as the plan will.
			field = st(courseColor(m.courseFor(word)), bg).Bold(true).Render(word) +
				inputField(strings.TrimPrefix(m.input, word), m.inputBack, inner-6-lipgloss.Width(word), ink, bg, violet)
		}
		lines = append(lines,
			pad(label("When", false)+when),
			pad(label("What", true)+field),
			pad(""),
			pad(hints(bg, "enter", "save", "⌫", "back to the time", "esc", "cancel")))
	}
	box := lipgloss.NewStyle().Width(width).Padding(0, 1).Background(bg).
		Border(lipgloss.RoundedBorder()).BorderForeground(violet).BorderBackground(base).
		Render(strings.Join(lines, "\n"))
	return strings.Split(box, "\n")
}

// overlay draws box over lines, centred horizontally, from line top.
func overlay(lines, box []string, top, width int) []string {
	if len(box) == 0 {
		return lines
	}
	bw := lipgloss.Width(box[0])
	left := max(0, (width-bw)/2)
	for i, b := range box {
		y := top + i
		if y < 0 || y >= len(lines) {
			continue
		}
		row := fill(lines[y], width, base)
		lines[y] = ansi.Truncate(row, left, "") + b + ansi.TruncateLeft(row, left+bw, "")
	}
	return lines
}
