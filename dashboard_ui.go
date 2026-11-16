package main

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strconv"
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
	rows := height - 2
	first := max(0, min(m.workTop[col], len(items)-rows))
	// Where the list stands when it scrolls, in the header: no line lost.
	scroll := ""
	if above, below := first, len(items)-first-rows; above > 0 || below > 0 {
		if above > 0 {
			scroll += st(cyan, bg).Render("↑") + st(faint, bg).Italic(true).Render(fmt.Sprintf("%d ", above))
		}
		if below > 0 {
			scroll += st(cyan, bg).Render(" ↓") + st(faint, bg).Italic(true).Render(fmt.Sprint(below))
		}
	}
	head := spread(titleStyle.Render(title)+count, scroll+st(ink, bg).Render(" "), width, bg)
	lines := []string{fill(head, width, bg), rule.Render(strings.Repeat("─", max(0, width)))}

	if len(items) == 0 {
		msg := "Nothing this week."
		if col == colPrevious {
			msg = "No delay. Well done."
		}
		lines = append(lines, fill(st(lime, bg).Render("✓ ")+st(faint, bg).Italic(true).Render(msg), width, bg))
	}
	now := time.Now()
	for r := first; r < len(items) && r < first+rows; r++ {
		lec := m.lectures[items[r]]
		ci := m.courseIndex(lec.CourseID)
		status := m.workField(lec)
		selected := focused && r == m.dashItems[col]
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

// ---------------------------------------------------------------- timeline

// gridColor draws the faint hour lines of the planner.
var gridColor = lipgloss.Color("#2C2C33")

func minutes(hhmm string) int {
	h, mm, _ := strings.Cut(hhmm, ":")
	hour, _ := strconv.Atoi(h)
	minute, _ := strconv.Atoi(mm)
	return hour*60 + minute
}

// span is when a plan takes place, in minutes; without an end, an hour.
func span(p Plan) (start, end int) {
	start = minutes(p.Start)
	end = start + 60
	if p.End != "" && minutes(p.End) > start {
		end = minutes(p.End)
	}
	return start, min(end, 24*60)
}

func clockLabel(minute int) string {
	if minute%60 == 0 {
		return fmt.Sprintf("%02dh", minute/60)
	}
	return fmt.Sprintf("%02dh%02d", minute/60, minute%60)
}

// timeline is the week's time axis, shared by every day so that the same
// hour sits on the same line all week. Band i runs from bounds[i] to
// bounds[i+1] and takes heights[i] lines: roughly in proportion to its
// length, but never so little that a plan cannot show its time and text.
type timeline struct {
	bounds, heights []int
}

func (t timeline) total() int {
	n := 0
	for _, h := range t.heights {
		n += h
	}
	return n
}

// line returns the first line of the band starting at minute.
func (t timeline) line(minute int) int {
	n := 0
	for i, b := range t.bounds[:len(t.bounds)-1] {
		if b >= minute {
			break
		}
		n += t.heights[i]
	}
	return n
}

// timeline fits the shown week's plans into room lines.
func (m Model) timeline(room int) timeline {
	week := m.viewWeek()
	var spans [][2]int
	for _, p := range m.plans {
		if p.Start != "" && !p.Day.Before(week) && p.Day.Before(week.AddDate(0, 0, 7)) {
			s, e := span(p)
			spans = append(spans, [2]int{s, e})
		}
	}
	if len(spans) == 0 {
		return timeline{}
	}
	lo, hi := 24*60, 0
	for _, s := range spans {
		lo, hi = min(lo, s[0]), max(hi, s[1])
	}
	lo, hi = lo/60*60, (hi+59)/60*60
	build := func(hours bool, scale float64) timeline {
		set := map[int]bool{lo: true, hi: true}
		for _, s := range spans {
			set[s[0]], set[s[1]] = true, true
		}
		if hours {
			for h := lo; h < hi; h += 60 {
				set[h] = true
			}
		}
		var t timeline
		for b := range set {
			t.bounds = append(t.bounds, b)
		}
		sort.Ints(t.bounds)
		for i := 0; i+1 < len(t.bounds); i++ {
			lines := int(math.Round(float64(t.bounds[i+1]-t.bounds[i]) / 60 * scale))
			t.heights = append(t.heights, max(1, lines))
		}
		// Every plan gets at least two lines: its time, then what.
		for _, s := range spans {
			first, last, sum := -1, -1, 0
			for i := range t.heights {
				if t.bounds[i] >= s[0] && t.bounds[i+1] <= s[1] {
					if first < 0 {
						first = i
					}
					last, sum = i, sum+t.heights[i]
				}
			}
			if last >= 0 && sum < 2 {
				t.heights[last] += 2 - sum
			}
		}
		return t
	}
	var t timeline
	for _, hours := range []bool{true, false} {
		// At most two lines an hour: half-hours read well, and the rest of
		// the screen is better spent on the work lists.
		for _, scale := range []float64{2, 1.5, 1, 0.5, 0} {
			if t = build(hours, scale); t.total() <= room {
				return t
			}
		}
	}
	return t // Too tall anyway: the bottom gets cut.
}

// card renders the lines of a plan, height lines tall and width wide.
func (m Model) card(p Plan, width, height int, selected bool) []string {
	accent := violet
	word, _, _ := strings.Cut(p.Text, " ")
	if ci := m.courseFor(word); ci >= 0 {
		accent = courseColor(ci)
	}
	tint := lipgloss.Darken(accent, 0.84)
	textColor := muted
	if selected {
		tint, textColor = lipgloss.Darken(accent, 0.62), ink
	}
	bar := st(accent, tint).Render("▌")
	inner := max(1, width-2)
	when := strings.ReplaceAll(p.Start, ":", "h")
	if p.End != "" {
		when += " → " + strings.ReplaceAll(p.End, ":", "h")
	}
	if lipgloss.Width(when) > inner {
		when = strings.ReplaceAll(p.Start, ":", "h") // Narrow: the start says enough.
	}
	timeStyle := st(lipgloss.Lighten(tint, 0.45), tint).Italic(true)
	if selected {
		timeStyle = st(ink, tint).Italic(true).Bold(true)
	}
	row := func(s string) string { return bar + st(ink, tint).Render(" ") + fill(s, inner, tint) }
	if height == 1 {
		return []string{row(timeStyle.Render(when) + st(ink, tint).Render(" ") + m.planText(p.Text, inner, tint, selected))}
	}
	lines := []string{row(timeStyle.Render(when))}
	wrapped := strings.Split(lipgloss.NewStyle().Width(inner).Render(p.Text), "\n")
	for i := 0; len(lines) < height; i++ {
		switch {
		case i == 0:
			text := strings.TrimRight(wrapped[0], " ")
			if len(wrapped) > height-1 && height == 2 {
				text = p.Text // One line only: let it end in an ellipsis.
			}
			lines = append(lines, row(m.planText(text, inner, tint, selected)))
		case i < len(wrapped):
			text := strings.TrimRight(wrapped[i], " ")
			if len(lines) == height-1 && i < len(wrapped)-1 {
				text = strings.Join(wrapped[i:], " ")
			}
			lines = append(lines, row(st(textColor, tint).Render(fitted(text, inner))))
		default:
			lines = append(lines, row(""))
		}
	}
	return lines
}

// dayColumn renders one day on the shared timeline: its header, the plans
// placed by time (side by side when they overlap), then the "add" row.
func (m Model) dayColumn(d, width int, t timeline) []string {
	bg := base
	day := m.viewWeek().AddDate(0, 0, d)
	focused := m.dashZone == zonePlanner && m.dashDay == d
	name := day.Format("Monday")
	header := st(muted, bg).Bold(true).Render(" " + name + " ")
	switch {
	case day.Equal(today()):
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
	if len(t.bounds) == 0 || len(ps) == 0 && t.total() == 0 {
		lines = append(lines, fill(st(line, bg).Italic(true).Render(" free"), width, bg))
	}
	if len(t.bounds) > 0 {
		// Lanes: overlapping plans sit side by side.
		lane := make([]int, len(ps))
		var laneEnds []int
		for i, pi := range ps {
			s, e := span(m.plans[pi])
			lane[i] = len(laneEnds)
			for l, end := range laneEnds {
				if end <= s {
					lane[i] = l
					break
				}
			}
			if lane[i] == len(laneEnds) {
				laneEnds = append(laneEnds, e)
			} else {
				laneEnds[lane[i]] = e
			}
		}
		lanes := max(1, len(laneEnds))
		laneWidth := max(1, (width-(lanes-1))/lanes)
		// The background: faint lines on the hours.
		grid := make([][]string, lanes)
		for l := range grid {
			for i, h := range t.heights {
				for y := 0; y < h; y++ {
					if y == 0 && t.bounds[i]%60 == 0 && i > 0 {
						grid[l] = append(grid[l], st(gridColor, bg).Render(strings.Repeat("┈", laneWidth)))
					} else {
						grid[l] = append(grid[l], fill("", laneWidth, bg))
					}
				}
			}
		}
		for i, pi := range ps {
			p := m.plans[pi]
			s, e := span(p)
			top, bottom := t.line(s), t.line(e)
			card := m.card(p, laneWidth, max(1, bottom-top), focused && i == m.dashPlan)
			for y, c := range card {
				if top+y < len(grid[lane[i]]) {
					grid[lane[i]][top+y] = c
				}
			}
		}
		for y := 0; y < t.total(); y++ {
			row := ""
			for l := range grid {
				if l > 0 {
					row += st(ink, bg).Render(" ")
				}
				row += grid[l][y]
			}
			lines = append(lines, fill(row, width, bg))
		}
	}

	add := ""
	if focused {
		add = st(faint, bg).Render(" + ") + st(faint, bg).Italic(true).Render("add")
		if m.dashPlan == len(ps) {
			add = st(violet, selection).Render("▌") + st(violet, selection).Bold(true).Render("+ ") +
				st(ink, selection).Italic(true).Render("add") + st(faint, selection).Italic(true).Render("  or press "+fmt.Sprint(d+1))
			add = fill(add, width, selection)
		}
	}
	return append(lines, fill(add, width, bg))
}

// gutter labels the timeline's lines with their times.
func (t timeline) gutter(width int) []string {
	bg := base
	lines := []string{fill("", width, bg), fill("", width, bg)}
	if len(t.bounds) == 0 {
		return append(lines, fill("", width, bg), fill("", width, bg))
	}
	for i, h := range t.heights {
		style := st(faint, bg).Italic(true)
		if t.bounds[i]%60 == 0 {
			style = st(muted, bg)
		}
		lines = append(lines, fill(style.Render(fmt.Sprintf("%*s ", width-1, clockLabel(t.bounds[i]))), width, bg))
		for y := 1; y < h; y++ {
			lines = append(lines, fill("", width, bg))
		}
	}
	end := t.bounds[len(t.bounds)-1]
	return append(lines, fill(st(muted, bg).Render(fmt.Sprintf("%*s ", width-1, clockLabel(end))), width, bg))
}

// dashBudget shares the dashboard's height between the work lists and the
// planner. The planner takes the room it needs to space its hours out, as
// long as the work lists keep a few lines; the work lists, which scroll,
// then get everything left, so no line goes to waste.
func (m Model) dashBudget(l layout) (workHeight int, t timeline, cols, dayRows, timelineHeight int) {
	const gutterWidth = 7
	cols = max(1, min(7, (l.width-gutterWidth+2)/18))
	dayRows = (7 + cols - 1) / cols
	plannerChrome := 3 + dayRows*3 + (dayRows - 1) // Title and gaps; per row header, rule, add row.
	body := l.bodyHeight - 3                       // Tabs, or the input box.
	most := max(len(m.workItems(colThisWeek)), len(m.workItems(colPrevious)), 1)
	workMin := min(2+most, 5)
	t = m.timeline(max(0, (body-workMin-plannerChrome)/dayRows))
	timelineHeight = max(1, t.total()) // An empty week still says "free".
	workHeight = max(workMin, min(2+most, body-plannerChrome-dayRows*timelineHeight))
	return workHeight, t, cols, dayRows, timelineHeight
}

// workRows is how many lectures a work column shows at once.
func (m Model) workRows(l layout) int {
	h, _, _, _, _ := m.dashBudget(l)
	return max(1, h-2)
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

	workHeight, t, cols, _, timelineHeight := m.dashBudget(l)
	const gutterWidth = 7
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

	rowHeight := 3 + timelineHeight
	cellWidth := (width - gutterWidth - 2*(cols-1)) / cols
	gutter := t.gutter(gutterWidth)
	for start := 0; start < 7 && len(lines) < l.bodyHeight; start += cols {
		cells := [][]string{gutter}
		for d := start; d < min(7, start+cols); d++ {
			cells = append(cells, m.dayColumn(d, cellWidth, t))
		}
		height := 0
		for _, c := range cells {
			height = max(height, len(c))
		}
		for y := 0; y < min(height, rowHeight); y++ {
			row := ""
			for i, c := range cells {
				w := cellWidth
				if i == 0 {
					w = gutterWidth
				} else if i > 1 {
					row += st(ink, bg).Render("  ")
				}
				cell := ""
				if y < len(c) {
					cell = c[y]
				}
				row += fill(cell, w, bg)
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
			field = st(onAccent, violet).Render(" ") + st(faint, bg).Italic(true).Render(" 8 10 · 14h30 16h · 9h")
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
