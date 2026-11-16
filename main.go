package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	_ "modernc.org/sqlite"
)

type Course struct {
	ID       int
	Name     string
	Position int
	Nick     string    // Short name such as CompSec; empty when unset.
	Added    time.Time // Zero for courses created before the date was recorded.
}

// Modes of the command box at the top of the list.
const (
	modeNormal = ""
	modeFilter = "filter"
	modeAdd    = "add"
	modeRename = "rename"
	modeNick   = "nick"
	modeDelete = "delete"
)

type Model struct {
	db        *sql.DB
	courses   []Course
	cursor    int // Index into visible().
	offset    int // First visible row of the list.
	mode      string
	input     string
	inputBack int // Runes between the text cursor and the end of input.
	filter    string
	message   string
	failed    bool // The message reports an error.
	help      bool
	noDetail  bool // The user hid the detail pane.
	section   int

	lectures   []Lecture
	dashCourse int // Index into courses of the focused block.
	dashRow    int // Lecture row in that block; one past the end is "+ New lecture".
	dashOffset int // First visible line of the lectures grid.

	plans    []Plan
	dashWeek time.Time // Monday of the week shown by the dashboard.
	dashTab  int       // Exercises or theory.
	dashZone int       // Work or planner.
	dashCol  int       // This week or previous weeks.
	dashItem int       // Selected lecture of the work column.
	dashDay  int       // Selected day of the planner, 0 is Monday.
	dashPlan int       // Selected plan of that day; one past the end is "add".
	justDone map[int]bool
	undo     *Plan // The last deleted plan.

	planTimes          string // The quick add's times, as typed.
	planStart, planEnd string
	weekAnchor         time.Time // Monday of week 1, when corrected by hand.
	width              int
	height             int
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS courses (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL UNIQUE
	)`); err != nil {
		return err
	}
	rows, err := db.Query("SELECT name FROM pragma_table_info('courses')")
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	rows.Close()
	if !columns["position"] {
		if _, err := db.Exec("ALTER TABLE courses ADD COLUMN position INTEGER"); err != nil {
			return err
		}
	}
	if !columns["nick"] {
		if _, err := db.Exec("ALTER TABLE courses ADD COLUMN nick TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	if !columns["created_at"] {
		if _, err := db.Exec("ALTER TABLE courses ADD COLUMN created_at INTEGER"); err != nil {
			return err
		}
	}
	if _, err := db.Exec("UPDATE courses SET position = id WHERE position IS NULL"); err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS lectures (
		id INTEGER PRIMARY KEY,
		course_id INTEGER NOT NULL REFERENCES courses(id),
		title TEXT NOT NULL,
		date TEXT NOT NULL,
		theory TEXT NOT NULL DEFAULT 'none',
		exo TEXT NOT NULL DEFAULT 'to do'
	);
	CREATE INDEX IF NOT EXISTS lectures_course ON lectures(course_id, date)`)
	if err != nil {
		return err
	}
	return migratePlans(db)
}

func (m *Model) load() error {
	rows, err := m.db.Query("SELECT id, name, nick, position, COALESCE(created_at, 0) FROM courses ORDER BY position, id")
	if err != nil {
		return err
	}
	defer rows.Close()

	m.courses = nil
	for rows.Next() {
		var c Course
		var added int64
		if err := rows.Scan(&c.ID, &c.Name, &c.Nick, &c.Position, &added); err != nil {
			return err
		}
		if added > 0 {
			c.Added = time.Unix(added, 0)
		}
		m.courses = append(m.courses, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	if err := m.loadLectures(); err != nil {
		return err
	}
	if err := m.loadSettings(); err != nil {
		return err
	}
	err = m.loadPlans()
	m.clamp()
	return err
}

// visible returns the indices of the courses matching the filter, in order.
func (m Model) visible() []int {
	words := strings.Fields(strings.ToLower(m.filter))
	var out []int
outer:
	for i, c := range m.courses {
		name := strings.ToLower(c.Name + " " + c.Nick)
		for _, w := range words {
			if !strings.Contains(name, w) {
				continue outer
			}
		}
		out = append(out, i)
	}
	return out
}

// selected returns the index of the selected course in m.courses, or -1.
func (m Model) selected() int {
	v := m.visible()
	if m.cursor < 0 || m.cursor >= len(v) {
		return -1
	}
	return v[m.cursor]
}

// selectID moves the cursor to the course with the given id, if visible.
func (m *Model) selectID(id int) {
	for i, c := range m.visible() {
		if m.courses[c].ID == id {
			m.cursor = i
			return
		}
	}
}

// clamp keeps the cursors on existing items and those items on screen.
func (m *Model) clamp() {
	m.clampDash()
	m.clampDashboard()
	n := len(m.visible())
	m.cursor = max(0, min(n-1, m.cursor))
	page := m.layout().pageSize
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+page {
		m.offset = m.cursor - page + 1
	}
	m.offset = max(0, min(m.offset, n-page))
}

func (m *Model) move(delta int) {
	m.cursor += delta
	m.clamp()
}

func (m *Model) notify(text string, err error) {
	m.message, m.failed = text, err != nil
	if err != nil {
		m.message = err.Error()
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	from := m.mode
	next, cmd := m.update(msg)
	// Any change of mode puts the text cursor back at the end of the input.
	if n := next.(Model); n.mode != from {
		n.inputBack = 0
		return n, cmd
	}
	return next, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clamp()
		return m, nil
	case tea.PasteMsg:
		if m.editing() {
			m.typeText(msg.Content)
		}
		return m, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	k := key.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	var cmd tea.Cmd
	if m.editing() && !m.inPopup() && m.editKey(k) {
		m.clamp()
		return m, nil
	}
	switch m.mode {
	case modeFilter:
		m.updateFilter(key)
	case modeAdd, modeRename, modeNick:
		m.updateInput(key)
	case modeLectureAdd, modeLectureTitle, modeLectureDate:
		m.updateLectureInput(k, key.Key().Text)
	case modePlanAdd, modePlanEdit:
		m.updatePlanInput(k, key.Key().Text)
	case modePlanTime, modePlanText:
		m.updatePopup(k, key.Key().Text)
	case modeWeek:
		m.updateWeekInput(k, key.Key().Text)
	case modeDelete, modeLectureDelete:
		switch {
		case k != "y":
			m.notify("Deletion cancelled.", nil)
		case m.mode == modeDelete:
			m.deleteSelected()
		default:
			m.deleteLecture()
		}
		m.mode = modeNormal
	default:
		m.message = ""
		switch k {
		case "q":
			return m, tea.Quit
		case "?":
			m.help = !m.help
		case "tab":
			m.section = (m.section + 1) % sectionCount
		case "shift+tab":
			m.section = (m.section + sectionCount - 1) % sectionCount
		case "esc":
			if m.help {
				m.help = false
				break
			}
			fallthrough
		default:
			switch m.section {
			case sectionDashboard:
				m.updateDashboard(k)
			case sectionLectures:
				m.updateLectures(k)
			default:
				cmd = m.updateNormal(k)
			}
		}
	}
	m.clamp()
	return m, cmd
}

func (m Model) editing() bool {
	switch m.mode {
	case modeFilter, modeAdd, modeRename, modeNick, modeLectureAdd, modeLectureTitle, modeLectureDate, modePlanAdd, modePlanEdit, modePlanTime, modePlanText, modeWeek:
		return true
	}
	return false
}

func (m *Model) updateNormal(k string) tea.Cmd {
	page := m.layout().pageSize
	switch k {
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.visible()) - 1
	case "ctrl+d", "pgdown":
		m.move(page)
	case "ctrl+u", "pgup":
		m.move(-page)
	case "/":
		m.mode, m.input = modeFilter, m.filter
	case "esc":
		if m.filter != "" {
			id := m.selectedID()
			m.filter = ""
			m.selectID(id)
		}
	case "p":
		m.noDetail = !m.noDetail
	case "a":
		m.mode, m.input = modeAdd, ""
	case "e", "r":
		if i := m.selected(); i >= 0 {
			m.mode, m.input = modeRename, m.courses[i].Name
		}
	case "n":
		if i := m.selected(); i >= 0 {
			m.mode, m.input = modeNick, m.courses[i].Nick
		}
	case "d", "x":
		if m.selected() >= 0 {
			m.mode = modeDelete
		}
	case "y":
		if i := m.selected(); i >= 0 {
			m.notify("Copied “"+m.courses[i].Name+"” to the clipboard.", nil)
			return tea.SetClipboard(m.courses[i].Name)
		}
	case "K", "shift+up":
		m.reorder(-1)
	case "J", "shift+down":
		m.reorder(1)
	}
	return nil
}

func (m Model) selectedID() int {
	if i := m.selected(); i >= 0 {
		return m.courses[i].ID
	}
	return 0
}

func (m *Model) updateFilter(key tea.KeyPressMsg) {
	switch key.String() {
	case "esc":
		id := m.selectedID()
		m.mode, m.filter, m.input = modeNormal, "", ""
		m.selectID(id)
	case "enter":
		m.mode = modeNormal
	case "backspace":
		m.backspace()
	case "down", "ctrl+n", "ctrl+j":
		m.move(1)
	case "up", "ctrl+p", "ctrl+k":
		m.move(-1)
	default:
		m.typeText(key.Key().Text)
	}
}

func (m *Model) updateInput(key tea.KeyPressMsg) {
	switch key.String() {
	case "esc":
		m.mode = modeNormal
	case "backspace":
		m.backspace()
	case "enter":
		name := strings.TrimSpace(m.input)
		if m.mode == modeNick {
			if err := m.setNick(name); err != nil {
				m.notify("", err)
				return
			}
			m.mode = modeNormal
			return
		}
		if name == "" {
			m.notify("", errString("Enter a course name."))
			return
		}
		var err error
		if m.mode == modeAdd {
			err = m.add(name)
		} else {
			err = m.rename(name)
		}
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				err = errString("A course with this name already exists.")
			}
			m.notify("", err)
			return
		}
		if m.mode == modeAdd {
			m.notify("Course added.", nil)
		} else {
			m.notify("Course renamed.", nil)
		}
		m.mode = modeNormal
	default:
		m.typeText(key.Key().Text)
	}
}

// setNick gives the selected course a nickname; an empty one removes it.
func (m *Model) setNick(nick string) error {
	id := m.selectedID()
	for _, c := range m.courses {
		if nick != "" && c.ID != id && strings.EqualFold(c.Nick, nick) {
			return errString("“" + c.Name + "” already goes by " + c.Nick + ".")
		}
	}
	if _, err := m.db.Exec("UPDATE courses SET nick = ? WHERE id = ?", nick, id); err != nil {
		return err
	}
	if err := m.load(); err != nil {
		return err
	}
	m.selectID(id)
	if nick == "" {
		m.notify("Nickname removed.", nil)
	} else {
		m.notify("Now known as "+nick+".", nil)
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }

func (m *Model) add(name string) error {
	res, err := m.db.Exec(`INSERT INTO courses (name, position, created_at)
		VALUES (?, (SELECT COALESCE(MAX(position), 0) + 1 FROM courses), ?)`, name, time.Now().Unix())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	if err := m.load(); err != nil {
		return err
	}
	// Keep the new course in sight even if it does not match the filter.
	m.selectID(int(id))
	if m.selectedID() != int(id) {
		m.filter = ""
		m.selectID(int(id))
	}
	return nil
}

func (m *Model) rename(name string) error {
	id := m.selectedID()
	if _, err := m.db.Exec("UPDATE courses SET name = ? WHERE id = ?", name, id); err != nil {
		return err
	}
	if err := m.load(); err != nil {
		return err
	}
	m.selectID(id)
	return nil
}

func (m *Model) deleteSelected() {
	i := m.selected()
	if i < 0 {
		return
	}
	id := m.courses[i].ID
	tx, err := m.db.Begin()
	if err == nil {
		_, err = tx.Exec("DELETE FROM lectures WHERE course_id = ?", id)
		if err == nil {
			_, err = tx.Exec("DELETE FROM courses WHERE id = ?", id)
		}
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
	}
	if err == nil {
		err = m.load()
	}
	m.notify("Course deleted.", err)
}

// reorder swaps the selected course with its neighbour in the full list.
func (m *Model) reorder(delta int) {
	if m.filter != "" {
		m.notify("", errString("Clear the filter (esc) to reorder courses."))
		return
	}
	i := m.selected()
	j := i + delta
	if i < 0 || j < 0 || j >= len(m.courses) {
		return
	}
	a, b := m.courses[i], m.courses[j]
	tx, err := m.db.Begin()
	if err == nil {
		_, err = tx.Exec("UPDATE courses SET position = ? WHERE id = ?", b.Position, a.ID)
		if err == nil {
			_, err = tx.Exec("UPDATE courses SET position = ? WHERE id = ?", a.Position, b.ID)
		}
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
	}
	if err == nil {
		err = m.load()
	}
	if err != nil {
		m.notify("", err)
		return
	}
	m.selectID(a.ID)
}

func cleanName(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// dbPath is where the data lives, the same from any directory: $STASH_DB,
// or stash/stash.db in the user's data directory (~/.local/share).
func dbPath() (string, error) {
	if p := os.Getenv("STASH_DB"); p != "" {
		return p, nil
	}
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	dir = filepath.Join(dir, "stash")
	return filepath.Join(dir, "stash.db"), os.MkdirAll(dir, 0o755)
}

func main() {
	path, err := dbPath()
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := migrate(db); err != nil {
		log.Fatal(err)
	}

	m := Model{db: db}
	if err := m.load(); err != nil {
		log.Fatal(err)
	}
	m.focusCourse(0)
	m.goToWeek(weekStart(time.Now()))

	if _, err := tea.NewProgram(m).Run(); err != nil {
		log.Fatal(err)
	}
}
