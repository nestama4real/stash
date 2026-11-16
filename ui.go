package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// A neutral, transparent base (the terminal and its wallpaper show through)
// lit by an electric cyan → violet → pink accent.
var (
	base      color.Color = lipgloss.NoColor{}
	surface               = lipgloss.Color("#1A1A1E") // Status bar, title block.
	selection             = lipgloss.Color("#2A2A31") // Selected row.
	keyBg                 = lipgloss.Color("#323238") // Key caps.
	ink                   = lipgloss.Color("#F4F4F5")
	muted                 = lipgloss.Color("#A1A1AA")
	faint                 = lipgloss.Color("#71717A")
	line                  = lipgloss.Color("#3F3F46")
	onAccent              = lipgloss.Color("#0B0B0E") // Text on accent backgrounds.

	cyan   = lipgloss.Color("#00E1FF")
	violet = lipgloss.Color("#A855F7")
	pink   = lipgloss.Color("#FF2E97")
	lime   = lipgloss.Color("#B4FF39")
	amber  = lipgloss.Color("#FFC21A")
	red    = lipgloss.Color("#FF4D6D")

	brand = []color.Color{cyan, violet, pink}
)

// Below this size the screen only shows a short notice.
const minWidth, minHeight = 40, 14

// Lines used by the list pane around its rows: command box, column titles, gap.
const listChrome = 5

// rowHeight is the height of a course row, including the gap below it.
const rowHeight = 3

type layout struct {
	tiny                   bool
	margin, width, height  int // Content area, inside the side margins.
	listWidth, detailWidth int // detailWidth is 0 when the detail pane is hidden.
	bodyHeight, pageSize   int
	help                   []string
}

func (m Model) layout() layout {
	w, h := m.width, m.height
	if w == 0 {
		w = 100
	}
	if h == 0 {
		h = 32
	}
	l := layout{tiny: w < minWidth || h < minHeight, height: h}
	if w >= 60 {
		l.margin = 1
	}
	l.width = w - 2*l.margin
	if m.help {
		l.help = helpPanel(l.width, m.helpGroups())
		// The list keeps at least a few rows; the help panel gives way.
		l.help = l.help[:min(len(l.help), max(0, h-3-listChrome-rowHeight))]
	}
	l.bodyHeight = max(0, h-3-len(l.help))
	l.listWidth = l.width
	if m.section == sectionCourses && !m.noDetail && l.width >= 90 {
		l.detailWidth = max(36, min(64, l.width*2/5))
		l.listWidth = l.width - l.detailWidth - 1
	}
	l.pageSize = max(1, (l.bodyHeight-listChrome+1)/rowHeight)
	return l
}

func st(fg, bg color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(fg).Background(bg)
}

func fitted(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

// fill truncates a styled string to width and pads it with the background.
func fill(s string, width int, bg color.Color) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "")
	return s + st(ink, bg).Render(strings.Repeat(" ", width-lipgloss.Width(s)))
}

// spread places left and right on one line, dropping right when space is short.
func spread(left, right string, width int, bg color.Color) string {
	if lipgloss.Width(left)+lipgloss.Width(right)+1 > width {
		return fill(left, width, bg)
	}
	return fill(left, width-lipgloss.Width(right), bg) + right
}

func lower(s string) string { return strings.Map(unicode.ToLower, s) }

// highlight renders name within width, emphasising the parts matching words.
func highlight(name string, words []string, width int, base, match lipgloss.Style) string {
	text := []rune(fitted(name, width))
	low := []rune(lower(name))
	marked := make([]bool, len(text))
	for _, w := range words {
		w := []rune(w)
		for i := 0; i+len(w) <= len(low); i++ {
			if string(low[i:i+len(w)]) == string(w) {
				for j := i; j < i+len(w) && j < len(marked); j++ {
					marked[j] = true
				}
			}
		}
	}
	var b strings.Builder
	for i := 0; i < len(text); {
		j := i
		for j < len(text) && marked[j] == marked[i] {
			j++
		}
		style := base
		if marked[i] {
			style = match
		}
		b.WriteString(style.Render(string(text[i:j])))
		i = j
	}
	return b.String()
}

func ago(t, now time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/24/7))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
	}
	return fmt.Sprintf("%dy", int(d.Hours()/24/365))
}

func agoLong(t, now time.Time) string {
	short := ago(t, now)
	units := map[string]string{"m": "minute", "h": "hour", "d": "day", "w": "week", "mo": "month", "y": "year"}
	n, unit := 0, ""
	fmt.Sscanf(short, "%d%s", &n, &unit)
	switch {
	case short == "now":
		return "just now"
	case units[unit] == "":
		return short
	case n == 1:
		return "1 " + units[unit] + " ago"
	}
	return fmt.Sprintf("%d %ss ago", n, units[unit])
}

// ---------------------------------------------------------------- flourishes

// gradient colours each character of s along the given colour stops.
func gradient(s string, base lipgloss.Style, stops ...color.Color) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	colors := lipgloss.Blend1D(max(2, len(runes)), stops...)
	var b strings.Builder
	for i, r := range runes {
		b.WriteString(base.Foreground(colors[i]).Render(string(r)))
	}
	return b.String()
}

// pill renders s on a gradient background, as a glowing label.
func pill(s string, stops ...color.Color) string {
	runes := []rune(s)
	colors := lipgloss.Blend1D(max(2, len(runes)), stops...)
	var b strings.Builder
	for i, r := range runes {
		b.WriteString(lipgloss.NewStyle().Foreground(onAccent).Background(colors[i]).Bold(true).Render(string(r)))
	}
	return b.String()
}

// keycap renders a key as a small raised cap.
func keycap(k string, _ color.Color) string {
	return st(ink, keyBg).Bold(true).Render(" " + k + " ")
}

// hints renders "key action · key action" with quiet italic actions.
func hints(bg color.Color, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, st(muted, bg).Bold(true).Render(pairs[i])+st(faint, bg).Italic(true).Render(" "+pairs[i+1]))
	}
	return strings.Join(parts, st(line, bg).Render("  ·  "))
}

// ageColor makes fresh courses glow and old ones fade.
func ageColor(t, now time.Time) color.Color {
	switch d := now.Sub(t); {
	case t.IsZero():
		return faint
	case d < 24*time.Hour:
		return lime
	case d < 7*24*time.Hour:
		return cyan
	case d < 30*24*time.Hour:
		return muted
	}
	return faint
}

// ---------------------------------------------------------------- header

func (m Model) header(l layout) []string {
	bg := base
	var title, count, key, action string
	switch m.section {
	case sectionDashboard:
		title, key, action = "Dashboard", "a", " plan something"
		count = m.weekLabel(bg)
	case sectionLectures:
		title, key, action = "Lectures", "a", " new lecture"
		count = st(muted, bg).Render(fmt.Sprintf("  %d", len(m.lectures))) + st(faint, bg).Italic(true).Render(" lectures")
	default:
		title, key, action = "Courses", "a", " new course"
		count = st(muted, bg).Render(fmt.Sprintf("  %d", len(m.courses)))
		if m.filter != "" {
			count += st(faint, bg).Italic(true).Render(fmt.Sprintf(" · %d shown", len(m.visible())))
		}
	}
	icon := sectionIcons[m.section]
	left := st(ink, bg).Render(" ") + gradient(icon, st(ink, bg), brand...) + st(ink, bg).Render(" ") +
		st(ink, bg).Bold(true).Render(title) + count + st(ink, bg).Render("  ")
	right := keycap(key, bg) + st(faint, bg).Italic(true).Render(action) +
		st(line, bg).Render("   ┃   ") + pill(" S T A S H ", brand...) + st(ink, bg).Render(" ")
	underline := min(l.width, lipgloss.Width(left))
	rule := gradient(strings.Repeat("━", underline), st(ink, bg), brand...) +
		gradient(strings.Repeat("─", l.width-underline), st(ink, bg), line, lipgloss.Darken(line, 0.5))
	return []string{spread(left, right, l.width, bg), rule}
}

// ---------------------------------------------------------------- list pane

func (m Model) commandBox(width int) []string {
	bg := base
	inner := max(1, width-4)
	border := line
	accent := muted
	var prompt, value, hint string
	cursor := false
	switch m.mode {
	case modeFilter:
		border, accent, value, cursor = cyan, cyan, m.input, true
		prompt = st(cyan, bg).Bold(true).Render("\uf002 ")
		hint = hints(bg, "enter", "apply", "esc", "clear")
	case modeAdd:
		border, accent, value, cursor = lime, lime, m.input, true
		prompt = st(lime, bg).Bold(true).Render("+ New course ") + st(line, bg).Render("› ")
		hint = hints(bg, "enter", "save", "esc", "cancel")
	case modeRename:
		border, accent, value, cursor = violet, violet, m.input, true
		prompt = st(violet, bg).Bold(true).Render(" Rename ") + st(line, bg).Render("› ")
		hint = hints(bg, "enter", "save", "esc", "cancel")
	case modePlanAdd, modePlanEdit:
		border, accent, value, cursor = violet, violet, m.input, true
		verb := "+ Plan "
		if m.mode == modePlanEdit {
			verb = "\uf040 Plan "
		}
		// The day follows what is typed, so "jeu …" shows Thursday at once.
		day := m.dashDay
		if word, _, _ := strings.Cut(strings.TrimSpace(m.input), " "); word != "" {
			if d, ok := dayNames[strings.TrimSuffix(strings.ToLower(word), ".")]; ok {
				day = d
			}
		}
		prompt = st(violet, bg).Bold(true).Render(verb) +
			st(ink, bg).Bold(true).Italic(true).Render(m.viewWeek().AddDate(0, 0, day).Format("Monday")) + st(line, bg).Render(" › ")
		hint = st(faint, bg).Italic(true).Render("[day] [8-10] what") + st(line, bg).Render("  ·  ") + hints(bg, "enter", "save", "esc", "cancel")
	case modeWeek:
		border, accent, value, cursor = cyan, cyan, m.input, true
		prompt = st(cyan, bg).Bold(true).Render("# This is week ") + st(line, bg).Render("› ")
		hint = st(faint, bg).Italic(true).Render("week 1 moves to match") + st(line, bg).Render("  ·  ") + hints(bg, "enter", "save", "esc", "cancel")
	case modeNick:
		border, accent, value, cursor = amber, amber, m.input, true
		prompt = st(amber, bg).Bold(true).Render("\uf02b Nickname ") +
			st(muted, bg).Italic(true).Render(fitted(m.courses[m.selected()].Name, 24)) + st(line, bg).Render(" › ")
		hint = hints(bg, "enter", "save", "empty", "remove", "esc", "cancel")
	case modeLectureAdd:
		border, accent, value, cursor = lime, lime, m.input, true
		prompt = st(lime, bg).Bold(true).Render("+ New lecture ") +
			st(courseColor(m.dashCourse), bg).Italic(true).Render(m.courses[m.dashCourse].label()) + st(line, bg).Render(" › ")
		hint = hints(bg, "enter", "add on "+m.nextDate(m.courses[m.dashCourse].ID).Format("Mon 02/01"), "esc", "cancel")
	case modeLectureTitle:
		border, accent, value, cursor = violet, violet, m.input, true
		prompt = st(violet, bg).Bold(true).Render("\uf040 Title ") + st(line, bg).Render("› ")
		hint = hints(bg, "enter", "save", "esc", "cancel")
	case modeLectureDate:
		border, accent, value, cursor = cyan, cyan, m.input, true
		prompt = st(cyan, bg).Bold(true).Render("\uf073 Date ") + st(line, bg).Render("› ")
		hint = st(faint, bg).Italic(true).Render("15/09 · today · +7 · -1w") + st(line, bg).Render("  ·  ") + hints(bg, "enter", "save")
	case modeLectureDelete:
		border = red
		value = m.lectures[m.dashLecture()].Title
		prompt = st(red, bg).Bold(true).Render("✕ Delete lecture ")
		hint = hints(bg, "y", "confirm", "any key", "cancel")
	case modeDelete:
		border = red
		value = m.courses[m.selected()].Name
		if n := len(m.lecturesOf(m.courses[m.selected()].ID)); n > 0 {
			hint = st(red, bg).Italic(true).Render(fmt.Sprintf("and its %d lecture%s", n, plural(n))) + st(line, bg).Render("  ·  ")
		}
		prompt = st(red, bg).Bold(true).Render("✕ Delete ")
		hint += hints(bg, "y", "confirm", "any key", "cancel")
	default:
		prompt = st(faint, bg).Render("\uf002 ")
		if m.filter == "" {
			prompt += st(faint, bg).Italic(true).Render("Filter courses…")
			hint = keycap("/", bg)
		} else {
			prompt = st(amber, bg).Render("\uf002 ")
			value = m.filter
			hint = hints(bg, "esc", "clear")
		}
	}
	space := inner - lipgloss.Width(prompt)
	if lipgloss.Width(hint)+12 <= space {
		space -= lipgloss.Width(hint) + 2
	} else {
		hint = ""
	}
	text := st(ink, bg)
	var body string
	switch {
	case cursor:
		body = inputField(value, m.inputBack, space, ink, bg, accent)
	case m.mode == modeDelete || m.mode == modeLectureDelete:
		body = text.Bold(true).Italic(true).Render(fitted("“"+value+"”", space-1)) + st(red, bg).Bold(true).Render("?")
	default:
		body = st(amber, bg).Italic(true).Render(fitted(value, space))
	}
	content := spread(prompt+body, hint, inner, bg)
	box := lipgloss.NewStyle().Width(width).Padding(0, 1).
		Border(lipgloss.RoundedBorder()).BorderForeground(border).BorderBackground(bg).
		Background(bg).Render(content)
	return strings.Split(box, "\n")
}

func (m Model) row(index, position int, width int, now time.Time) []string {
	c := m.courses[index]
	selected := position == m.cursor
	bg := base
	marker := [2]string{st(ink, bg).Render("  "), st(ink, bg).Render("  ")}
	icon := st(line, bg).Render("◇ ")
	if selected {
		bg = selection
		// The marker runs from cyan to pink, top to bottom.
		marker = [2]string{st(cyan, bg).Render("▌") + st(ink, bg).Render(" "), st(pink, bg).Render("▌") + st(ink, bg).Render(" ")}
		icon = gradient("◆", st(ink, bg), brand...) + st(ink, bg).Render(" ")
	}
	const ageWidth = 6
	nameWidth := max(1, width-4-ageWidth-1)
	meta := st(muted, bg).Render(fmt.Sprintf("Course %02d", index+1))
	if selected {
		meta = st(ink, bg).Bold(true).Render(fmt.Sprintf("Course %02d", index+1))
	}
	if !c.Added.IsZero() {
		meta += st(line, bg).Render(" · ") + st(faint, bg).Italic(true).Render(c.Added.Format("02 Jan 2006"))
	}
	age := st(ageColor(c.Added, now), bg).Italic(true).Render(fmt.Sprintf("%*s", ageWidth, ago(c.Added, now)))
	nameStyle := st(muted, bg)
	if selected {
		nameStyle = st(ink, bg).Bold(true)
	}
	words := strings.Fields(lower(m.filter))
	match := nameStyle.Foreground(amber).Bold(true).Underline(true)
	name := highlight(c.Name, words, nameWidth, nameStyle, match)
	if c.Nick != "" {
		// The nickname leads, in the course's colour; the full name follows.
		nick := st(courseColor(index), bg).Bold(true)
		name = highlight(c.Nick, words, nameWidth, nick, match.Foreground(amber))
		if rest := nameWidth - lipgloss.Width(c.Nick) - 2; rest > 3 {
			name += st(ink, bg).Render("  ") + highlight(c.Name, words, rest, nameStyle.Italic(true), match)
		}
	}
	tail := st(ink, bg).Render(" ")
	return []string{
		marker[0] + icon + fill(meta, nameWidth, bg) + age + tail,
		marker[1] + st(ink, bg).Render("  ") + fill(name, nameWidth, bg) + fill("", ageWidth+1, bg),
	}
}

func (m Model) listPane(l layout) []string {
	width := l.listWidth
	lines := m.commandBox(width)
	bg := base
	label := st(faint, bg).Bold(true)
	titles := st(ink, bg).Render("    ") + label.Render("C O U R S E")
	lines = append(lines, spread(titles, label.Render("A D D E D")+st(ink, bg).Render(" "), width, bg), fill("", width, bg))

	visible := m.visible()
	rowsHeight := max(0, l.bodyHeight-len(lines))
	if len(visible) == 0 {
		var title, hint string
		if len(m.courses) == 0 {
			title = "It starts with a course."
			hint = st(faint, bg).Italic(true).Render("Press ") + keycap("a", bg) + st(faint, bg).Italic(true).Render(" to add your first one.")
		} else {
			title = "No course matches “" + m.filter + "”."
			hint = st(faint, bg).Italic(true).Render("Press ") + keycap("esc", bg) + st(faint, bg).Italic(true).Render(" to clear the filter.")
		}
		empty := []string{
			gradient("◇  ◆  ◇", st(ink, bg), brand...),
			"",
			gradient(fitted(title, width-2), st(ink, bg).Bold(true).Italic(true), brand...),
			"",
			fitted(hint, width-2),
		}
		top := max(0, rowsHeight/3-2)
		for i := 0; i < top; i++ {
			lines = append(lines, fill("", width, bg))
		}
		for _, e := range empty {
			pad := max(0, (width-lipgloss.Width(e))/2)
			lines = append(lines, fill(st(ink, bg).Render(strings.Repeat(" ", pad))+e, width, bg))
		}
	} else {
		now := time.Now()
		end := min(len(visible), m.offset+l.pageSize)
		for p := m.offset; p < end; p++ {
			lines = append(lines, m.row(visible[p], p, width, now)...)
			lines = append(lines, fill("", width, bg))
		}
	}
	for len(lines) < l.bodyHeight {
		lines = append(lines, fill("", width, bg))
	}
	lines = lines[:l.bodyHeight]
	// Scroll hint on the last line when rows are hidden below.
	if len(visible) > 0 && l.bodyHeight > 0 && m.offset+l.pageSize < len(visible) {
		more := st(faint, bg).Italic(true).Render(fmt.Sprintf("%d more ", len(visible)-m.offset-l.pageSize)) + st(cyan, bg).Render("↓")
		lines[l.bodyHeight-1] = spread("", more+st(ink, bg).Render(" "), width, bg)
	}
	return lines
}

// scrollbar renders the column between the panes, with a thumb when the list overflows.
func (m Model) scrollbar(l layout) []string {
	n := len(m.visible())
	out := make([]string, l.bodyHeight)
	thumbStart, thumbSize := -1, 0
	if n > l.pageSize && l.bodyHeight > 0 {
		thumbSize = max(1, l.bodyHeight*l.pageSize/n)
		thumbStart = (l.bodyHeight - thumbSize) * m.offset / max(1, n-l.pageSize)
	}
	colors := lipgloss.Blend1D(max(2, thumbSize), brand...)
	for y := range out {
		if y >= thumbStart && y < thumbStart+thumbSize {
			out[y] = st(colors[y-thumbStart], base).Render("┃")
		} else {
			out[y] = st(line, base).Render("│")
		}
	}
	return out
}

// ---------------------------------------------------------------- detail pane

func (m Model) detailPane(l layout) []string {
	width := l.detailWidth
	bg := base
	inner := width - 3
	pad := func(s string, bg color.Color) string {
		return st(ink, bg).Render("  ") + fill(s, inner, bg) + st(ink, bg).Render(" ")
	}
	var lines []string
	i := m.selected()
	if i < 0 {
		for len(lines) < l.bodyHeight/2-1 {
			lines = append(lines, fill("", width, bg))
		}
		lines = append(lines, pad(gradient("◇", st(ink, bg), brand...)+st(faint, bg).Italic(true).Render("  Nothing selected."), bg))
	} else {
		c := m.courses[i]
		now := time.Now()
		// Title block.
		head := surface
		lines = append(lines,
			pad(st(faint, head).Bold(true).Render("C O U R S E")+st(line, head).Render("  ·  ")+
				st(muted, head).Italic(true).Render(fmt.Sprintf("%02d of %02d", i+1, len(m.courses))), head),
			pad("", head))
		wrapped := strings.Split(lipgloss.NewStyle().Width(inner).Render(c.Name), "\n")
		if len(wrapped) > 3 {
			wrapped = append(wrapped[:2], fitted(strings.Join(wrapped[2:], " "), inner))
		}
		for _, w := range wrapped {
			lines = append(lines, pad(gradient(strings.TrimRight(w, " "), st(ink, head).Bold(true), brand...), head))
		}
		lines = append(lines, pad("", head), fill("", width, bg))

		lines = append(lines, pad(pill(" ◆ COURSE ", brand...)+
			st(muted, bg).Italic(true).Render(fmt.Sprintf("  position %d", i+1))+
			st(line, bg).Render(" · ")+st(faint, bg).Italic(true).Render(fmt.Sprintf("id #%d", c.ID)), bg),
			fill("", width, bg))
		added := st(faint, bg).Italic(true).Render("added date unknown")
		if !c.Added.IsZero() {
			added = st(faint, bg).Italic(true).Render("added ") +
				st(ageColor(c.Added, now), bg).Bold(true).Render(agoLong(c.Added, now)) +
				st(line, bg).Render(" · ") + st(faint, bg).Italic(true).Render(c.Added.Format("Mon 02 Jan 2006, 15:04"))
		}
		lines = append(lines, pad(added, bg), fill("", width, bg))

		section := func(icon string, accent color.Color, title string) {
			head := st(accent, bg).Bold(true).Render(icon+" ") + st(ink, bg).Bold(true).Render(title) + st(ink, bg).Render(" ")
			rule := gradient(strings.Repeat("─", max(0, inner-lipgloss.Width(head))), st(ink, bg), line, lipgloss.Darken(line, 0.6))
			lines = append(lines, fill("", width, bg), pad(head+rule, bg), fill("", width, bg))
		}
		kv := func(k, v string) {
			lines = append(lines, pad(st(faint, bg).Italic(true).Render(fmt.Sprintf("%-10s", k))+v, bg))
		}
		section("≡", cyan, "Details")
		if c.Nick != "" {
			kv("Nickname", st(courseColor(i), bg).Bold(true).Render(c.Nick))
		} else {
			kv("Nickname", st(faint, bg).Italic(true).Render("none · ")+keycap("n", bg)+st(faint, bg).Italic(true).Render(" to add one"))
		}
		kv("Name", st(ink, bg).Render(fmt.Sprintf("%d", len([]rune(c.Name))))+st(faint, bg).Italic(true).Render(" characters"))
		if ls := m.lecturesOf(c.ID); len(ls) > 0 {
			theory, exo := 0, 0
			for _, li := range ls {
				if m.lectures[li].Theory == "done" {
					theory++
				}
				if e := m.lectures[li].Exo; e == "done" || e == "no exo" {
					exo++
				}
			}
			kv("Lectures", st(ink, bg).Bold(true).Render(fmt.Sprint(len(ls)))+st(line, bg).Render(" · ")+
				st(faint, bg).Italic(true).Render("theory ")+st(lime, bg).Render(fmt.Sprintf("%d/%d", theory, len(ls)))+st(line, bg).Render(" · ")+
				st(faint, bg).Italic(true).Render("exos ")+st(lime, bg).Render(fmt.Sprintf("%d/%d", exo, len(ls))))
		}
		// Position in the course list as a glowing progress bar.
		barWidth := max(4, min(24, inner-20))
		filled := max(1, barWidth*(i+1)/len(m.courses))
		bar := gradient(strings.Repeat("━", filled), st(ink, bg), lipgloss.Blend1D(len(m.courses)+1, brand...)[:i+2]...) +
			st(line, bg).Render(strings.Repeat("─", barWidth-filled))
		kv("Order", bar+st(ink, bg).Bold(true).Render(fmt.Sprintf("  %d", i+1))+st(faint, bg).Render(fmt.Sprintf("/%d", len(m.courses))))
		if i > 0 {
			kv("Before", st(cyan, bg).Render("↑ ")+st(muted, bg).Italic(true).Render(fitted(m.courses[i-1].Name, inner-12)))
		}
		if i+1 < len(m.courses) {
			kv("After", st(pink, bg).Render("↓ ")+st(muted, bg).Italic(true).Render(fitted(m.courses[i+1].Name, inner-12)))
		}

		section("▸", pink, "Actions")
		// Actions as a grid of key caps, two per line.
		actions := [][2]string{{"e", "rename"}, {"n", "nickname"}, {"d", "delete"}, {"y", "copy name"}, {"J/K", "reorder"}, {"p", "hide details"}}
		cell := func(a [2]string) string {
			return fill(keycap(a[0], bg)+st(muted, bg).Italic(true).Render(" "+a[1]), 18, bg)
		}
		for j := 0; j < len(actions); j += 2 {
			lines = append(lines, pad(cell(actions[j])+cell(actions[j+1]), bg), fill("", width, bg))
		}
	}
	for len(lines) < l.bodyHeight {
		lines = append(lines, fill("", width, bg))
	}
	return lines[:l.bodyHeight]
}

// ---------------------------------------------------------------- status bar

func (m Model) statusBar(l layout) string {
	bg := surface
	sep := st(line, bg).Render("  │  ")
	// Sections, like gh-dash's bottom bar: the current one glows.
	left := ""
	for i, name := range sectionNames {
		label := " " + sectionIcons[i] + " " + strings.ToUpper(name) + " "
		if i == m.section {
			left += pill(label, brand...)
		} else {
			left += st(faint, bg).Render(label)
		}
	}
	modes := map[string]struct {
		label  string
		accent color.Color
	}{
		modeFilter: {" \uf002 FILTER ", cyan}, modeAdd: {" + NEW ", lime},
		modeRename: {" \uf040 RENAME ", violet}, modeNick: {" \uf02b NICKNAME ", amber}, modeDelete: {" ✕ DELETE ", red},
		modeLectureAdd: {" + NEW ", lime}, modeLectureTitle: {" \uf040 TITLE ", violet},
		modeLectureDate: {" \uf073 DATE ", cyan}, modeLectureDelete: {" ✕ DELETE ", red},
		modePlanAdd: {" + PLAN ", violet}, modePlanEdit: {" \uf040 PLAN ", violet},
		modePlanTime: {" + PLAN ", violet}, modePlanText: {" + PLAN ", violet}, modeWeek: {" # WEEK ", cyan},
	}
	if md, ok := modes[m.mode]; ok {
		left += st(onAccent, md.accent).Bold(true).Render(md.label)
	}
	left += st(ink, bg).Render("  ")
	switch {
	case m.message != "" && m.failed:
		left += st(red, bg).Bold(true).Render("✕ ") + st(ink, bg).Italic(true).Render(m.message)
	case m.message != "":
		left += st(lime, bg).Bold(true).Render("✓ ") + st(ink, bg).Italic(true).Render(m.message)
	case m.section == sectionDashboard:
		where := st(faint, bg).Italic(true).Render([]string{"exercises", "theory"}[m.dashTab])
		if m.dashZone == zonePlanner {
			where = st(violet, bg).Render("\uf073 ") + st(muted, bg).Italic(true).Render(m.planDayDate().Format("Monday 02/01"))
		}
		left += st(muted, bg).Render("\uf1c0 ") + st(muted, bg).Italic(true).Render("stash.db") + sep + where
	case m.section == sectionLectures:
		where := st(faint, bg).Italic(true).Render("no course yet")
		if len(m.courses) > 0 {
			c := m.courses[m.dashCourse]
			n := len(m.lecturesOf(c.ID))
			where = st(courseColor(m.dashCourse), bg).Render("◆ ") + st(muted, bg).Italic(true).Render(c.label()) + sep
			if m.dashRow < n {
				where += st(faint, bg).Italic(true).Render("lecture ") + st(ink, bg).Bold(true).Render(fmt.Sprint(m.dashRow+1)) +
					st(faint, bg).Render(fmt.Sprintf("/%d", n))
			} else {
				where += st(faint, bg).Italic(true).Render("new lecture")
			}
		}
		left += st(muted, bg).Render("\uf1c0 ") + st(muted, bg).Italic(true).Render("stash.db") + sep + where
	default:
		n := len(m.visible())
		where := st(faint, bg).Italic(true).Render("no course yet")
		if n > 0 {
			where = st(faint, bg).Italic(true).Render("course ") + st(ink, bg).Bold(true).Render(fmt.Sprint(m.cursor+1)) +
				st(faint, bg).Render(fmt.Sprintf("/%d", n))
		}
		left += st(muted, bg).Render("\uf1c0 ") + st(muted, bg).Italic(true).Render("stash.db") + sep + where
		if m.filter != "" {
			left += sep + st(amber, bg).Render("\uf002 ") + st(amber, bg).Italic(true).Render(m.filter)
		}
	}
	help := st(muted, keyBg).Render(" ? ") + st(muted, keyBg).Italic(true).Render("help ")
	if m.help {
		help = pill(" ? help ", brand...)
	}
	return spread(left, help, l.width, bg)
}

// ---------------------------------------------------------------- help panel

type binding struct{ keys, desc string }

type helpGroup struct {
	title    string
	bindings []binding
}

var (
	sectionsHelp = helpGroup{"Sections", []binding{{"tab", "next section"}, {"shift+tab", "previous section"}}}
	editingHelp  = helpGroup{"Typing", []binding{
		{"←→", "move the cursor"}, {"ctrl+←→", "by word"}, {"home/end", "start/end"},
		{"ctrl+w", "delete word"}, {"ctrl+u", "delete to start"},
	}}
	generalHelp = helpGroup{"General", []binding{{"?", "toggle help"}, {"q", "quit"}}}

	coursesHelp = []helpGroup{
		{"Navigation", []binding{
			{"↑/k", "move up"}, {"↓/j", "move down"}, {"g/home", "first course"},
			{"G/end", "last course"}, {"ctrl+d", "page down"}, {"ctrl+u", "page up"},
		}},
		{"Courses", []binding{
			{"a", "add"}, {"e", "rename"}, {"n", "nickname"}, {"d", "delete"}, {"y", "copy name"},
			{"K/shift+↑", "move earlier"}, {"J/shift+↓", "move later"},
		}},
		{"View", []binding{{"/", "filter"}, {"esc", "clear filter"}, {"p", "toggle details"}}},
		{"Editing", []binding{{"enter", "confirm"}, {"esc", "cancel"}, {"↑/↓", "browse matches"}}},
		editingHelp, sectionsHelp, generalHelp,
	}

	dashboardHelp = []helpGroup{
		{"Plan a day", []binding{
			{"1…7", "Monday…Sunday"}, {"then 8 10␣", "from 8h to 10h"}, {"then words", "what to do"},
			{"enter", "save"}, {"⌫ (empty)", "back to the time"},
		}},
		{"Move", []binding{
			{"↑↓←→", "move around"}, {"hjkl", "move around"}, {"[ ] / < >", "previous/next week"},
			{"w", "back to this week"}, {"#", "fix the week number"},
		}},
		{"Work", []binding{
			{"x", "exercises"}, {"t", "theory"}, {"space/enter", "done / undone"}, {"o", "ongoing"},
		}},
		{"Plan", []binding{
			{"a", "add (anywhere)"}, {"enter", "add / edit"}, {"e", "edit"}, {"d", "delete"},
			{"u", "undo delete"}, {"c", "copy to next week"},
		}},
		{"One-line plan (a)", []binding{
			{"8-10 CompSec S9", "today, 8h–10h"}, {"mar 14h Gym", "Tuesday, 14h"}, {"jeu Laundry", "Thursday"},
		}},
		editingHelp, sectionsHelp, generalHelp,
	}

	lecturesHelp = []helpGroup{
		{"Courses", []binding{
			{"↑↓←→", "move between courses"}, {"hjkl", "move between courses"},
			{"g/G", "first/last course"},
		}},
		{"Lectures", []binding{
			{"shift+↑↓", "select a lecture"}, {"K/J", "select a lecture"},
		}},
		{"Edit", []binding{
			{"a", "add to course"}, {"enter", "add / edit title"}, {"e", "edit title"}, {"d", "delete"},
		}},
		{"Status", []binding{
			{"t/T", "cycle theory"}, {"x/X", "cycle exo"}, {"space", "cycle exo"},
		}},
		{"Date", []binding{
			{"w", "type a date"}, {"[ ]", "∓ one day"}, {"< >", "∓ one week"},
		}},
		editingHelp, sectionsHelp, generalHelp,
	}
)

func (m Model) helpGroups() []helpGroup {
	switch m.section {
	case sectionDashboard:
		return dashboardHelp
	case sectionLectures:
		return lecturesHelp
	}
	return coursesHelp
}

// helpPanel lays the key groups out in columns, wrapping them to the width.
func helpPanel(width int, helpGroups []helpGroup) []string {
	bg := base
	const gap = 5
	accents := lipgloss.Blend1D(len(helpGroups), brand...)
	var columns [][]string
	var widths []int
	for gi, g := range helpGroups {
		keyWidth := 0
		for _, b := range g.bindings {
			keyWidth = max(keyWidth, lipgloss.Width(b.keys))
		}
		title := st(accents[gi], bg).Render("▍") + st(accents[gi], bg).Bold(true).Render(strings.ToUpper(g.title))
		col := []string{title, ""}
		w := lipgloss.Width(title)
		for _, b := range g.bindings {
			entry := st(ink, bg).Bold(true).Render(fmt.Sprintf(" %-*s", keyWidth+2, b.keys)) + st(muted, bg).Italic(true).Render(b.desc)
			col = append(col, entry)
			w = max(w, lipgloss.Width(entry))
		}
		columns, widths = append(columns, col), append(widths, w)
	}
	lines := []string{fill("", width, bg)}
	for start := 0; start < len(columns); {
		end, used := start, 0
		for end < len(columns) && (end == start || used+gap+widths[end] <= width-2) {
			if end > start {
				used += gap
			}
			used += widths[end]
			end++
		}
		height := 0
		for _, c := range columns[start:end] {
			height = max(height, len(c))
		}
		for y := 0; y < height; y++ {
			row := st(ink, bg).Render("  ")
			for i := start; i < end; i++ {
				cell := ""
				if y < len(columns[i]) {
					cell = columns[i][y]
				}
				if i > start {
					row += st(ink, bg).Render(strings.Repeat(" ", gap))
				}
				row += fill(cell, widths[i], bg)
			}
			lines = append(lines, fill(row, width, bg))
		}
		if end < len(columns) {
			lines = append(lines, fill("", width, bg))
		}
		start = end
	}
	return lines
}

// ---------------------------------------------------------------- view

func (m Model) View() tea.View {
	l := m.layout()
	var content string
	if l.tiny {
		w := max(1, m.width)
		notice := pill(" S T A S H ", brand...) + "\n\n" +
			st(muted, base).Italic(true).Render("Enlarge the terminal\nto see your courses.") + "\n" +
			st(faint, base).Render("q quit")
		content = lipgloss.NewStyle().Width(w).MaxWidth(w).MaxHeight(max(1, m.height)).Render(notice)
	} else {
		lines := m.header(l)
		var list []string
		switch m.section {
		case sectionDashboard:
			list = m.dashboardPane(l)
		case sectionLectures:
			list = m.lecturesPane(l)
		default:
			list = m.listPane(l)
		}
		if m.section == sectionCourses && l.detailWidth > 0 {
			detail := m.detailPane(l)
			bar := m.scrollbar(l)
			for i := range list {
				list[i] += bar[i] + detail[i]
			}
		}
		if m.inPopup() && len(list) > 0 {
			width := min(l.width-2, 70)
			popup := m.popup(width)
			list = overlay(list, popup, max(0, (len(list)-len(popup))/3), l.width)
		}
		lines = append(lines, list...)
		lines = append(lines, m.statusBar(l))
		lines = append(lines, l.help...)
		margin := st(ink, base).Render(strings.Repeat(" ", l.margin))
		for i := range lines {
			lines[i] = margin + lines[i] + margin
		}
		content = strings.Join(lines, "\n")
	}
	v := tea.NewView(content)
	v.AltScreen = true
	// No background: the terminal's own (and its wallpaper) shows through.
	v.WindowTitle = "Stash · " + sectionNames[m.section]
	return v
}
