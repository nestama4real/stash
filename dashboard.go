package main

import (
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// A gap of at least this many weeks without any lecture (winter or summer
// holidays) ends a semester: the next lecture starts week 1 again. Week
// numbers thus follow the lectures in the database, with nothing to set up.
const semesterGapWeeks = 3

// Tabs of the dashboard's lecture view.
const (
	tabExercises = iota
	tabTheory
)

// Zones of the dashboard that hold the cursor.
const (
	zoneWork    = iota // This week / previous weeks.
	zonePlanner        // The week's plan.
)

// Columns of the work zone.
const (
	colThisWeek = iota
	colPrevious
)

const (
	modePlanAdd  = "plan-add"  // A whole plan typed on one line.
	modePlanEdit = "plan-edit" // The same, to change a plan.
	modePlanTime = "plan-time" // Quick add, first the time…
	modePlanText = "plan-text" // …then what to do, in a pop-up.
	modeWeek     = "week"      // Correcting the week number.
)

// Plan is a quick note of something to do on a day, optionally at a time.
type Plan struct {
	ID         int
	Day        time.Time
	Start, End string // "15:04", or empty.
	Text       string
}

func migratePlans(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS plans (
		id INTEGER PRIMARY KEY,
		day TEXT NOT NULL,
		start TEXT NOT NULL DEFAULT '',
		end TEXT NOT NULL DEFAULT '',
		text TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS plans_day ON plans(day)`)
	return err
}

// The week number correction is stored as the Monday of week 1.
const anchorSetting = "week1"

func (m *Model) loadSettings() error {
	var value string
	err := m.db.QueryRow("SELECT value FROM settings WHERE key = ?", anchorSetting).Scan(&value)
	if err == sql.ErrNoRows {
		m.weekAnchor = time.Time{}
		return nil
	}
	if err != nil {
		return err
	}
	m.weekAnchor, _ = time.ParseInLocation(dateLayout, value, time.Local)
	return nil
}

// setWeekNumber makes the shown week number n, by moving week 1.
func (m *Model) setWeekNumber(n int) error {
	anchor := m.viewWeek().AddDate(0, 0, -7*(n-1))
	if _, err := m.db.Exec("INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		anchorSetting, anchor.Format(dateLayout)); err != nil {
		return err
	}
	return m.loadSettings()
}

func (m *Model) loadPlans() error {
	rows, err := m.db.Query(`SELECT id, day, start, end, text FROM plans
		ORDER BY day, start = '', start, end, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	m.plans = nil
	for rows.Next() {
		var p Plan
		var day string
		if err := rows.Scan(&p.ID, &day, &p.Start, &p.End, &p.Text); err != nil {
			return err
		}
		p.Day, _ = time.ParseInLocation(dateLayout, day, time.Local)
		m.plans = append(m.plans, p)
	}
	return rows.Err()
}

// ---------------------------------------------------------------- weeks

func daysBetween(a, b time.Time) int {
	return int(math.Round(b.Sub(a).Hours() / 24))
}

// viewWeek is the Monday of the week the dashboard shows.
func (m Model) viewWeek() time.Time {
	if m.dashWeek.IsZero() {
		return weekStart(time.Now())
	}
	return m.dashWeek
}

// semester finds the first week of the semester containing week, from the
// lecture dates alone: the first lecture after a break of at least
// semesterGapWeeks starts a new one. ok is false before any lecture.
func (m Model) semester(week time.Time) (start time.Time, ok bool) {
	start, ok = m.detectedSemester(week)
	// A correction made during this semester (its week 1 within a few weeks
	// of the detected one) wins; the next semester starts afresh.
	if a := m.weekAnchor; ok && !a.IsZero() && abs(daysBetween(a, start)) <= 7*semesterGapWeeks {
		start = a
	}
	return start, ok
}

func abs(n int) int { return max(n, -n) }

// detectedSemester finds the semester's first week from the lectures alone.
func (m Model) detectedSemester(week time.Time) (start time.Time, ok bool) {
	var weeks []time.Time
	for _, l := range m.lectures {
		if w := weekStart(l.Date); !w.After(week) {
			weeks = append(weeks, w)
		}
	}
	if len(weeks) == 0 {
		return time.Time{}, false
	}
	sort.Slice(weeks, func(i, j int) bool { return weeks[i].Before(weeks[j]) })
	start = weeks[0]
	for i := 1; i < len(weeks); i++ {
		if daysBetween(weeks[i-1], weeks[i]) >= 7*semesterGapWeeks {
			start = weeks[i]
		}
	}
	return start, true
}

// weekNumber is the week's number in its semester, or 0 before any lecture.
func (m Model) weekNumber(week time.Time) int {
	start, ok := m.semester(week)
	if !ok {
		return 0
	}
	return daysBetween(start, week)/7 + 1
}

// ---------------------------------------------------------------- work

// workField returns the status the current tab tracks for a lecture.
func (m Model) workField(l Lecture) string {
	if m.dashTab == tabTheory {
		return l.Theory
	}
	return l.Exo
}

// workItems lists the lectures of a column, as indices into m.lectures:
// every lecture of the week, or the unfinished ones of earlier weeks.
func (m Model) workItems(col int) []int {
	week := m.viewWeek()
	start, inSemester := m.semester(week)
	if detected, _ := m.detectedSemester(week); detected.Before(start) {
		start = detected // Lectures before a corrected week 1 can still be late.
	}
	var out []int
	for i, l := range m.lectures {
		status := m.workField(l)
		if status == "no exo" {
			continue
		}
		w := weekStart(l.Date)
		switch col {
		case colThisWeek:
			if w.Equal(week) {
				out = append(out, i)
			}
		case colPrevious:
			if w.Before(week) && (!inSemester || !w.Before(start)) && (status != "done" || m.justDone[l.ID]) {
				out = append(out, i)
			}
		}
	}
	return out
}

func (m Model) selectedWork() int {
	items := m.workItems(m.dashCol)
	if m.dashZone != zoneWork || m.dashItem < 0 || m.dashItem >= len(items) {
		return -1
	}
	return items[m.dashItem]
}

// setWork sets the tracked status of the selected lecture.
func (m *Model) setWork(status string) {
	li := m.selectedWork()
	if li < 0 {
		return
	}
	l := m.lectures[li]
	field := "exo"
	if m.dashTab == tabTheory {
		field = "theory"
	}
	if m.workField(l) == status {
		status = "to do"
		if field == "theory" {
			status = "none"
		}
	}
	if _, err := m.db.Exec("UPDATE lectures SET "+field+" = ? WHERE id = ?", status, l.ID); err != nil {
		m.notify("", err)
		return
	}
	// Keep finished lectures in sight until the dashboard is reloaded,
	// so a slip of the key is one press away from being undone.
	if m.justDone == nil {
		m.justDone = map[int]bool{}
	}
	m.justDone[l.ID] = true
	if err := m.loadLectures(); err != nil {
		m.notify("", err)
	}
}

// ---------------------------------------------------------------- planner

// plansOn returns the indices in m.plans of a day's plans, by time.
func (m Model) plansOn(day time.Time) []int {
	var out []int
	for i, p := range m.plans {
		if p.Day.Equal(day) {
			out = append(out, i)
		}
	}
	return out
}

func (m Model) planDayDate() time.Time {
	return m.viewWeek().AddDate(0, 0, m.dashDay)
}

func (m Model) selectedPlan() int {
	ps := m.plansOn(m.planDayDate())
	if m.dashZone != zonePlanner || m.dashPlan < 0 || m.dashPlan >= len(ps) {
		return -1
	}
	return ps[m.dashPlan]
}

func (m *Model) selectPlan(id int) {
	for i, p := range m.plans {
		if p.ID != id {
			continue
		}
		m.dashWeek = weekStart(p.Day)
		m.dashDay = daysBetween(m.dashWeek, p.Day)
		m.dashZone = zonePlanner
		for r, j := range m.plansOn(p.Day) {
			if j == i {
				m.dashPlan = r
			}
		}
	}
}

// goToWeek shows another week, on today when it is the current one.
func (m *Model) goToWeek(week time.Time) {
	m.dashWeek = week
	m.dashDay = 0
	if week.Equal(weekStart(time.Now())) {
		m.dashDay = daysBetween(week, today())
	}
	m.dashItem, m.dashPlan = 0, 0
	m.justDone = nil
	// Nothing due this week: start on what is late.
	m.dashCol = colThisWeek
	if len(m.workItems(colThisWeek)) == 0 {
		m.dashCol = colPrevious
	}
}

var dayNames = map[string]int{
	"lun": 0, "lundi": 0, "mon": 0, "monday": 0,
	"mar": 1, "mardi": 1, "tue": 1, "tuesday": 1,
	"mer": 2, "mercredi": 2, "wed": 2, "wednesday": 2,
	"jeu": 3, "jeudi": 3, "thu": 3, "thursday": 3,
	"ven": 4, "vendredi": 4, "fri": 4, "friday": 4,
	"sam": 5, "samedi": 5, "sat": 5, "saturday": 5,
	"dim": 6, "dimanche": 6, "sun": 6, "sunday": 6,
}

// A time range such as 8-10, 8h-10h, 08h00 - 10h00, 8:30-10, 14h or 14:15.
var timeRange = regexp.MustCompile(`^(\d{1,2})(?:[h:.](\d{2})?)?(?:\s*(?:-|–|→|à|to)\s*(\d{1,2})(?:[h:.](\d{2})?)?)?(?:\s*:\s+|\s+|$)`)

func clock(h, mm string) (string, bool) {
	hour, _ := strconv.Atoi(h)
	minute := 0
	if mm != "" {
		minute, _ = strconv.Atoi(mm)
	}
	if hour > 23 || minute > 59 {
		return "", false
	}
	return fmt.Sprintf("%02d:%02d", hour, minute), true
}

// parsePlan reads "[day] [time range] text", e.g. "mar 8-10 CompSec : S9".
// Without a day, the plan goes on defaultDay (0 is Monday).
func parsePlan(s string, defaultDay int) (day int, start, end, text string, err error) {
	s = strings.TrimSpace(cleanName(s))
	day = defaultDay
	if word, rest, _ := strings.Cut(s, " "); word != "" {
		if d, ok := dayNames[strings.TrimSuffix(strings.ToLower(word), ".")]; ok {
			day, s = d, strings.TrimSpace(rest)
		}
	}
	if g := timeRange.FindStringSubmatch(s); g != nil {
		var ok bool
		if start, ok = clock(g[1], g[2]); !ok {
			return 0, "", "", "", errString("Unknown time “" + strings.TrimSpace(g[0]) + "”.")
		}
		if g[3] != "" {
			if end, ok = clock(g[3], g[4]); !ok {
				return 0, "", "", "", errString("Unknown time “" + strings.TrimSpace(g[0]) + "”.")
			}
		}
		s = s[len(g[0]):]
	}
	text = strings.TrimSpace(strings.TrimLeft(s, ":"))
	if text == "" {
		return 0, "", "", "", errString("Write what to do, e.g. “mar 8-10 CompSec : S9”.")
	}
	return day, start, end, text, nil
}

// planInput is how a plan reads back in the input box.
func planInput(p Plan) string {
	when := strings.ReplaceAll(p.Start, ":", "h")
	if p.End != "" {
		when += "-" + strings.ReplaceAll(p.End, ":", "h")
	}
	if when != "" {
		when += " "
	}
	return when + p.Text
}

func (m *Model) savePlan() error {
	day, start, end, text, err := parsePlan(m.input, m.dashDay)
	if err != nil {
		return err
	}
	return m.storePlan(day, start, end, text)
}

// storePlan saves a plan on a day of the shown week: the selected plan when
// editing one, a new plan otherwise.
func (m *Model) storePlan(day int, start, end, text string) error {
	date := m.viewWeek().AddDate(0, 0, day).Format(dateLayout)
	var id int64
	var err error
	if m.mode == modePlanEdit {
		p := m.plans[m.selectedPlan()]
		id = int64(p.ID)
		_, err = m.db.Exec("UPDATE plans SET day = ?, start = ?, end = ?, text = ? WHERE id = ?", date, start, end, text, id)
	} else {
		var res sql.Result
		if res, err = m.db.Exec("INSERT INTO plans (day, start, end, text) VALUES (?, ?, ?, ?)", date, start, end, text); err == nil {
			id, _ = res.LastInsertId()
		}
	}
	if err != nil {
		return err
	}
	if err := m.loadPlans(); err != nil {
		return err
	}
	m.selectPlan(int(id))
	return nil
}

func (m *Model) deletePlan() {
	pi := m.selectedPlan()
	if pi < 0 {
		return
	}
	p := m.plans[pi]
	_, err := m.db.Exec("DELETE FROM plans WHERE id = ?", p.ID)
	if err == nil {
		err = m.loadPlans()
	}
	if err == nil {
		m.undo = &p
	}
	m.notify("Deleted “"+p.Text+"” · u to undo.", err)
}

func (m *Model) undoDelete() {
	if m.undo == nil {
		return
	}
	p := *m.undo
	_, err := m.db.Exec("INSERT INTO plans (id, day, start, end, text) VALUES (?, ?, ?, ?, ?)",
		p.ID, p.Day.Format(dateLayout), p.Start, p.End, p.Text)
	if err == nil {
		err = m.loadPlans()
	}
	if err == nil {
		m.undo = nil
		m.selectPlan(p.ID)
	}
	m.notify("Restored “"+p.Text+"”.", err)
}

// copyPlan repeats the selected plan a week later, for weekly habits.
func (m *Model) copyPlan() {
	pi := m.selectedPlan()
	if pi < 0 {
		return
	}
	p := m.plans[pi]
	day := p.Day.AddDate(0, 0, 7)
	_, err := m.db.Exec("INSERT INTO plans (day, start, end, text) VALUES (?, ?, ?, ?)",
		day.Format(dateLayout), p.Start, p.End, p.Text)
	if err == nil {
		err = m.loadPlans()
	}
	m.notify("Copied to "+day.Format("Mon 02/01")+".", err)
}

// ---------------------------------------------------------------- keys

func (m *Model) clampDashboard() {
	m.dashDay = max(0, min(6, m.dashDay))
	m.dashItem = max(0, min(len(m.workItems(m.dashCol))-1, m.dashItem))
	m.dashPlan = max(0, min(len(m.plansOn(m.planDayDate())), m.dashPlan))
}

func (m *Model) updateDashboard(k string) {
	switch k {
	case "1", "2", "3", "4", "5", "6", "7":
		// Straight to planning that day: type the time, then what to do.
		m.dashZone, m.dashDay = zonePlanner, int(k[0]-'1')
		m.dashPlan = len(m.plansOn(m.planDayDate()))
		m.mode, m.input = modePlanTime, ""
		return
	case "#":
		m.mode, m.input = modeWeek, ""
		if n := m.weekNumber(m.viewWeek()); n > 0 {
			m.input = strconv.Itoa(n)
		}
		return
	case "x":
		m.dashTab, m.dashItem, m.justDone = tabExercises, 0, nil
		return
	case "t":
		m.dashTab, m.dashItem, m.justDone = tabTheory, 0, nil
		return
	case "[", "<":
		m.goToWeek(m.viewWeek().AddDate(0, 0, -7))
		return
	case "]", ">":
		m.goToWeek(m.viewWeek().AddDate(0, 0, 7))
		return
	case "w":
		m.goToWeek(weekStart(time.Now()))
		return
	case "a", "n":
		if m.dashZone == zoneWork {
			m.dashDay = 0
			if m.viewWeek().Equal(weekStart(time.Now())) {
				m.dashDay = daysBetween(m.viewWeek(), today())
			}
		}
		m.mode, m.input = modePlanAdd, ""
		return
	case "u":
		m.undoDelete()
		return
	}
	if m.dashZone == zoneWork {
		m.updateWork(k)
	} else {
		m.updatePlanner(k)
	}
}

func (m *Model) updateWork(k string) {
	switch k {
	case "down", "j":
		if m.dashItem+1 < len(m.workItems(m.dashCol)) {
			m.dashItem++
		} else {
			m.dashZone, m.dashPlan = zonePlanner, 0
		}
	case "up", "k":
		m.dashItem = max(0, m.dashItem-1)
	case "left", "h":
		m.dashCol, m.dashItem = colThisWeek, 0
	case "right", "l":
		m.dashCol, m.dashItem = colPrevious, 0
	case "space", "enter":
		m.setWork("done")
	case "o":
		m.setWork("ongoing")
	}
}

func (m *Model) updatePlanner(k string) {
	switch k {
	case "down", "j":
		m.dashPlan++
	case "up", "k":
		if m.dashPlan == 0 {
			m.dashZone = zoneWork
			m.dashItem = len(m.workItems(m.dashCol)) - 1
		} else {
			m.dashPlan--
		}
	case "left", "h":
		m.dashDay = max(0, m.dashDay-1)
	case "right", "l":
		m.dashDay = min(6, m.dashDay+1)
	case "enter", "e":
		if pi := m.selectedPlan(); pi >= 0 {
			m.mode, m.input = modePlanEdit, planInput(m.plans[pi])
		} else if k == "enter" {
			m.mode, m.input = modePlanAdd, ""
		}
	case "d", "backspace", "delete":
		m.deletePlan()
	case "c":
		m.copyPlan()
	}
}

func (m *Model) updatePlanInput(key, text string) {
	switch key {
	case "esc":
		m.mode = modeNormal
	case "backspace":
		m.backspace()
	case "enter":
		editing := m.mode == modePlanEdit
		if err := m.savePlan(); err != nil {
			m.notify("", err)
			return
		}
		m.mode = modeNormal
		if editing {
			m.notify("Plan updated.", nil)
		} else {
			m.notify("Planned for "+m.planDayDate().Format("Monday 02/01")+".", nil)
		}
	default:
		m.typeText(text)
	}
}

// ---------------------------------------------------------------- courses in text

// label is how a course shows where space is short: its nickname, or else
// a short name derived from its full name.
func (c Course) label() string {
	if c.Nick != "" {
		return c.Nick
	}
	return shortName(c.Name)
}

// shortName gives a compact course label: the name itself when short,
// otherwise its initials (Principles of Online Decision Making → PODM).
func shortName(name string) string {
	if len([]rune(name)) <= 10 {
		return name
	}
	var b strings.Builder
	for _, w := range strings.Fields(name) {
		r := []rune(w)
		switch {
		case len(r) > 1 && strings.ToUpper(w) == w:
			b.WriteString(w)
		case unicode.IsUpper(r[0]):
			b.WriteRune(r[0])
		}
	}
	if b.Len() < 2 {
		return string([]rune(name)[:9]) + "…"
	}
	return b.String()
}

// courseFor finds the course a word abbreviates, such as CompSec for
// Computer Security and Privacy: its letters appear in order in the name.
func (m Model) courseFor(word string) int {
	w := []rune(strings.ToLower(strings.TrimFunc(word, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })))
	if len(w) < 2 {
		return -1
	}
	for ci, c := range m.courses {
		if c.Nick != "" && strings.EqualFold(c.Nick, string(w)) {
			return ci
		}
	}
	for ci, c := range m.courses {
		name := []rune(lower(c.Name))
		if len(name) == 0 || name[0] != w[0] {
			continue
		}
		i := 0
		for _, r := range name {
			if i < len(w) && r == w[i] {
				i++
			}
		}
		if i == len(w) {
			return ci
		}
	}
	return -1
}

// ---------------------------------------------------------------- quick add

// A time as typed in the quick add: 8, 8h, 8h30, 8:30, 08.30.
var clockWord = regexp.MustCompile(`^(\d{1,2})(?:[h:.](\d{2})?)?$`)

// parseTimes reads "8 10", "8-10", "8h30 10h" or "14": up to two times.
// n is how many times were found.
func parseTimes(s string) (start, end string, n int, err error) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune("-–→", r) })
	if len(fields) > 2 {
		return "", "", 0, errString("Two times at most, like “8 10”.")
	}
	var times []string
	for _, f := range fields {
		g := clockWord.FindStringSubmatch(strings.ToLower(f))
		if g == nil {
			return "", "", 0, errString("Unknown time “" + f + "”: try 8, 8h30 or 14:15.")
		}
		t, ok := clock(g[1], g[2])
		if !ok {
			return "", "", 0, errString("Unknown time “" + f + "”.")
		}
		times = append(times, t)
	}
	switch len(times) {
	case 2:
		return times[0], times[1], 2, nil
	case 1:
		return times[0], "", 1, nil
	}
	return "", "", 0, nil
}

func (m Model) inPopup() bool { return m.mode == modePlanTime || m.mode == modePlanText }

// toText moves the quick add on to describing the plan.
func (m *Model) toText(first string) bool {
	start, end, _, err := parseTimes(m.input)
	if err != nil {
		m.notify("", err)
		return false
	}
	m.planTimes, m.planStart, m.planEnd = m.input, start, end
	m.mode, m.input = modePlanText, first
	m.message = ""
	return true
}

func (m *Model) updatePopup(k, text string) {
	if k == "esc" {
		m.mode = modeNormal
		return
	}
	if m.mode == modePlanTime {
		switch {
		case k == "enter" || k == "tab":
			m.toText("")
		case k == "space":
			// The space after the second time opens the description.
			if _, _, n, err := parseTimes(m.input); err == nil && n == 2 {
				m.toText("")
			} else if m.trimmed() != "" {
				m.typeText(" ")
			}
		case m.editKey(k):
		case text != "" && strings.ContainsAny(text, "0123456789:.-–"),
			text == "h" && m.input != "" && unicode.IsDigit([]rune(m.input)[len([]rune(m.input))-1]):
			m.typeText(text)
		case text != "":
			// Typing words straight away: no time, or the time so far.
			m.toText(text)
		}
		return
	}
	switch {
	case k == "enter":
		if m.trimmed() == "" {
			m.notify("", errString("Write what to do."))
			return
		}
		if err := m.storePlan(m.dashDay, m.planStart, m.planEnd, m.trimmed()); err != nil {
			m.notify("", err)
			return
		}
		m.mode = modeNormal
		m.notify("Planned for "+m.planDayDate().Format("Monday 02/01")+".", nil)
	case (k == "backspace" || k == "shift+tab") && (m.input == "" || k == "shift+tab"):
		// Back to the time, as typed.
		m.mode, m.input = modePlanTime, m.planTimes
	case m.editKey(k):
	default:
		m.typeText(text)
	}
}

func (m *Model) updateWeekInput(k, text string) {
	switch k {
	case "esc":
		m.mode = modeNormal
	case "enter":
		n, err := strconv.Atoi(m.trimmed())
		if err != nil || n < 1 || n > 60 {
			m.notify("", errString("Enter the number of this week, like 10."))
			return
		}
		if err := m.setWeekNumber(n); err != nil {
			m.notify("", err)
			return
		}
		m.mode = modeNormal
		m.notify(fmt.Sprintf("This is week %d. New semesters still start at week 1 on their own.", n), nil)
	default:
		if text != "" && unicode.IsDigit([]rune(text)[0]) {
			m.typeText(text)
		}
	}
}
