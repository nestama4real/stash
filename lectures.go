package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Sections of the app, cycled with tab.
const (
	sectionDashboard = iota // The default, first section.
	sectionLectures
	sectionCourses
	sectionCount
)

var (
	sectionNames = [sectionCount]string{"Dashboard", "Lectures", "Courses"}
	sectionIcons = [sectionCount]string{"\uf0e4", "\uf0ce", "◆"}
)

// Modes of the command box specific to the lectures.
const (
	modeLectureAdd    = "lecture-add"
	modeLectureTitle  = "lecture-title"
	modeLectureDate   = "lecture-date"
	modeLectureDelete = "lecture-delete"
)

// Values a lecture's theory and exo can take, in cycling order.
var (
	theoryStates = []string{"none", "ongoing", "done"}
	exoStates    = []string{"to do", "ongoing", "done", "no exo"}
)

const dateLayout = "2006-01-02"

type Lecture struct {
	ID       int
	CourseID int
	Title    string
	Date     time.Time
	Theory   string
	Exo      string
}

func (m *Model) loadLectures() error {
	rows, err := m.db.Query("SELECT id, course_id, title, date, theory, exo FROM lectures ORDER BY date, id")
	if err != nil {
		return err
	}
	defer rows.Close()
	m.lectures = nil
	for rows.Next() {
		var l Lecture
		var date string
		if err := rows.Scan(&l.ID, &l.CourseID, &l.Title, &date, &l.Theory, &l.Exo); err != nil {
			return err
		}
		l.Date, _ = time.ParseInLocation(dateLayout, date, time.Local)
		m.lectures = append(m.lectures, l)
	}
	return rows.Err()
}

// lecturesOf returns the indices in m.lectures of a course's lectures, by date.
func (m Model) lecturesOf(courseID int) []int {
	var out []int
	for i, l := range m.lectures {
		if l.CourseID == courseID {
			out = append(out, i)
		}
	}
	return out
}

// dashLecture returns the index in m.lectures of the selected lecture, or -1
// when the cursor is on a course's "new lecture" row or nothing is selected.
func (m Model) dashLecture() int {
	if m.dashCourse < 0 || m.dashCourse >= len(m.courses) {
		return -1
	}
	ls := m.lecturesOf(m.courses[m.dashCourse].ID)
	if m.dashRow < 0 || m.dashRow >= len(ls) {
		return -1
	}
	return ls[m.dashRow]
}

func (m *Model) selectLecture(id int) {
	for ci, c := range m.courses {
		for r, li := range m.lecturesOf(c.ID) {
			if m.lectures[li].ID == id {
				m.dashCourse, m.dashRow = ci, r
				return
			}
		}
	}
}

// dashColumns is the number of course blocks side by side.
func (m Model) dashColumns() int { return dashColumns(m.layout().width) }

func dashColumns(width int) int {
	if width >= 140 {
		return 2
	}
	return 1
}

func (m *Model) clampDash() {
	m.dashCourse = max(0, min(len(m.courses)-1, m.dashCourse))
	if len(m.courses) == 0 {
		m.dashRow, m.dashOffset = 0, 0
		return
	}
	// The last row of each block is "+ New lecture".
	m.dashRow = max(0, min(len(m.lecturesOf(m.courses[m.dashCourse].ID)), m.dashRow))
	if m.section != sectionLectures {
		return
	}
	_, cursorLine, top, bottom, total := m.dashboardLines(m.layout())
	height := m.dashBodyHeight(m.layout())
	if bottom-top <= height {
		// The whole focused course fits: show all of it, from its title down.
		if top < m.dashOffset {
			m.dashOffset = top
		}
		if bottom > m.dashOffset+height {
			m.dashOffset = bottom - height
		}
		m.dashOffset = max(0, min(m.dashOffset, total-height))
		return
	}
	// A course taller than the screen: follow the selected row.
	if cursorLine < m.dashOffset+1 {
		m.dashOffset = cursorLine - 1
	}
	if cursorLine > m.dashOffset+height-2 {
		m.dashOffset = cursorLine - height + 2
	}
	m.dashOffset = max(0, min(m.dashOffset, total-height))
}

// moveRow selects the previous or next row of the focused course.
func (m *Model) moveRow(delta int) {
	if len(m.courses) == 0 {
		return
	}
	rows := len(m.lecturesOf(m.courses[m.dashCourse].ID)) + 1
	m.dashRow = max(0, min(rows-1, m.dashRow+delta))
}

// focusCourse moves to another course, with "+ New lecture" selected.
func (m *Model) focusCourse(ci int) {
	if ci < 0 || ci >= len(m.courses) {
		return
	}
	m.dashCourse = ci
	m.dashRow = len(m.lecturesOf(m.courses[ci].ID))
}

func (m *Model) updateLectures(k string) {
	li := m.dashLecture()
	cols := m.dashColumns()
	switch k {
	// Between courses, through the grid.
	case "down", "j":
		m.focusCourse(m.dashCourse + cols)
	case "up", "k":
		m.focusCourse(m.dashCourse - cols)
	case "right", "l":
		m.focusCourse(m.dashCourse + 1)
	case "left", "h":
		m.focusCourse(m.dashCourse - 1)
	case "g", "home":
		m.focusCourse(0)
	case "G", "end":
		m.focusCourse(len(m.courses) - 1)
	// Within the focused course.
	case "shift+down", "J":
		m.moveRow(1)
	case "shift+up", "K":
		m.moveRow(-1)
	case "a", "enter", "n":
		if k == "enter" && li >= 0 {
			m.startLectureEdit(modeLectureTitle)
			return
		}
		if len(m.courses) == 0 {
			m.notify("", errString("Add a course first (tab to Courses)."))
			return
		}
		count := len(m.lecturesOf(m.courses[m.dashCourse].ID))
		m.mode, m.input = modeLectureAdd, fmt.Sprintf("%d. ", count+1)
	case "e", "r":
		m.startLectureEdit(modeLectureTitle)
	case "w", "D":
		m.startLectureEdit(modeLectureDate)
	case "d":
		if li >= 0 {
			m.mode = modeLectureDelete
		}
	case "t":
		m.cycleLecture("theory", 1)
	case "T":
		m.cycleLecture("theory", -1)
	case "x", "space":
		m.cycleLecture("exo", 1)
	case "X":
		m.cycleLecture("exo", -1)
	case "]":
		m.shiftLecture(1)
	case "[":
		m.shiftLecture(-1)
	case ">", "}":
		m.shiftLecture(7)
	case "<", "{":
		m.shiftLecture(-7)
	}
}

func (m *Model) startLectureEdit(mode string) {
	li := m.dashLecture()
	if li < 0 {
		return
	}
	m.mode = mode
	if mode == modeLectureDate {
		m.input = m.lectures[li].Date.Format("02/01/2006")
	} else {
		m.input = m.lectures[li].Title
	}
}

// nextDate proposes a date for a course's next lecture: a week after its
// latest one, since lectures come weekly, or today for a first lecture.
func (m Model) nextDate(courseID int) time.Time {
	ls := m.lecturesOf(courseID)
	if len(ls) == 0 {
		return today()
	}
	return m.lectures[ls[len(ls)-1]].Date.AddDate(0, 0, 7)
}

func today() time.Time {
	y, mo, d := time.Now().Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, time.Local)
}

// parseDate understands 15/09/2026, 15/09, 15.09.2026, 2026-09-15,
// "today", and offsets such as +7 or -1 (days) or +1w (weeks) from ref.
func parseDate(s string, ref time.Time) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "today", "auj", "aujourd'hui", "now":
		return today(), nil
	case "tomorrow", "demain":
		return today().AddDate(0, 0, 1), nil
	}
	if len(s) > 1 && (s[0] == '+' || s[0] == '-') {
		unit := 1
		num := s[1:]
		if strings.HasSuffix(num, "w") || strings.HasSuffix(num, "s") {
			unit, num = 7, num[:len(num)-1]
		}
		if n, err := strconv.Atoi(num); err == nil {
			if s[0] == '-' {
				n = -n
			}
			return ref.AddDate(0, 0, n*unit), nil
		}
	}
	for _, layout := range []string{"02/01/2006", "2/1/2006", "02.01.2006", "2.1.2006", dateLayout} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	for _, layout := range []string{"02/01", "2/1", "02.01", "2.1"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return time.Date(ref.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local), nil
		}
	}
	return time.Time{}, errString("Unknown date “" + s + "”: try 15/09/2026, 15/09, today or +7.")
}

func (m *Model) updateLectureInput(key string, text string) {
	switch key {
	case "esc":
		m.mode = modeNormal
		return
	case "backspace":
		m.backspace()
		return
	case "enter":
	default:
		m.typeText(text)
		return
	}
	value := strings.TrimSpace(m.input)
	var err error
	var done string
	switch m.mode {
	case modeLectureAdd:
		if value == "" {
			err = errString("Enter a lecture title.")
			break
		}
		course := m.courses[m.dashCourse]
		date := m.nextDate(course.ID)
		var id int64
		res, e := m.db.Exec("INSERT INTO lectures (course_id, title, date, theory, exo) VALUES (?, ?, ?, ?, ?)",
			course.ID, value, date.Format(dateLayout), theoryStates[0], exoStates[0])
		if err = e; err == nil {
			id, _ = res.LastInsertId()
			err = m.loadLectures()
			m.selectLecture(int(id))
			done = "Lecture added for " + date.Format("Mon 02/01") + " · [ ] shift the date"
		}
	case modeLectureTitle:
		if value == "" {
			err = errString("Enter a lecture title.")
			break
		}
		err = m.updateLecture("title", value)
		done = "Lecture renamed."
	case modeLectureDate:
		li := m.dashLecture()
		var date time.Time
		if date, err = parseDate(value, m.lectures[li].Date); err == nil {
			err = m.updateLecture("date", date.Format(dateLayout))
			done = "Date set to " + date.Format("Mon 02/01/2006") + "."
		}
	}
	if err != nil {
		m.notify("", err)
		return
	}
	m.notify(done, nil)
	m.mode = modeNormal
}

// updateLecture saves one field of the selected lecture and keeps it selected.
func (m *Model) updateLecture(field, value string) error {
	li := m.dashLecture()
	if li < 0 {
		return nil
	}
	id := m.lectures[li].ID
	// field is one of a fixed set of column names, never user input.
	if _, err := m.db.Exec("UPDATE lectures SET "+field+" = ? WHERE id = ?", value, id); err != nil {
		return err
	}
	if err := m.loadLectures(); err != nil {
		return err
	}
	m.selectLecture(id)
	return nil
}

func (m *Model) cycleLecture(field string, delta int) {
	li := m.dashLecture()
	if li < 0 {
		return
	}
	states, current := theoryStates, m.lectures[li].Theory
	if field == "exo" {
		states, current = exoStates, m.lectures[li].Exo
	}
	next := 0
	for i, s := range states {
		if s == current {
			next = (i + delta + len(states)) % len(states)
		}
	}
	if err := m.updateLecture(field, states[next]); err != nil {
		m.notify("", err)
	}
}

func (m *Model) shiftLecture(days int) {
	li := m.dashLecture()
	if li < 0 {
		return
	}
	date := m.lectures[li].Date.AddDate(0, 0, days)
	if err := m.updateLecture("date", date.Format(dateLayout)); err != nil {
		m.notify("", err)
		return
	}
	m.notify("Moved to "+date.Format("Mon 02/01/2006")+".", nil)
}

func (m *Model) deleteLecture() {
	li := m.dashLecture()
	if li < 0 {
		return
	}
	_, err := m.db.Exec("DELETE FROM lectures WHERE id = ?", m.lectures[li].ID)
	if err == nil {
		err = m.loadLectures()
	}
	m.notify("Lecture deleted.", err)
}
