package main

import (
	"database/sql"
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

func press(m Model, code rune, text string) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: code, Text: text})
	return next.(Model)
}

func shiftTab(m Model) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	return next.(Model)
}

func typeKeys(m Model, s string) Model {
	for _, r := range s {
		m = press(m, r, string(r))
	}
	return m
}

func testDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1) // Every connection would get its own in-memory database.
	t.Cleanup(func() { db.Close() })
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func courses(n int) []Course {
	var out []Course
	for i := 0; i < n; i++ {
		out = append(out, Course{ID: i + 1, Position: i + 1, Name: fmt.Sprintf("Cours %03d — 数学 %s", i, strings.Repeat("long ", 20))})
	}
	return out
}

func TestAddInputRemainsVisible(t *testing.T) {
	for _, width := range []int{40, 100, 160} {
		for _, input := range []string{"M", "Mathematics", "数学", strings.Repeat("long ", 60) + "END"} {
			m := press(Model{section: sectionCourses, width: width, height: 24}, 'a', "a")
			m = typeKeys(m, input)
			view := ansi.Strip(m.View().Content)
			want := input
			if strings.HasSuffix(input, "END") {
				want = "END"
			}
			if !strings.Contains(view, want) {
				t.Fatalf("input %q invisible at width %d:\n%s", want, width, view)
			}
		}
	}
}

func TestSelectedRowBackgroundIsContinuous(t *testing.T) {
	m := Model{section: sectionCourses, width: 120, height: 32, courses: courses(3), cursor: 1}
	m.clamp()
	l := m.layout()
	lines := uv.NewStyledString(m.View().Content).Lines(ansi.GraphemeWidth)
	checked := 0
	for _, row := range lines {
		if len(row) <= l.margin || row[l.margin].Content != "▌" {
			continue
		}
		// Wide characters take a single entry, so track columns by cell width.
		for i, x := 0, 0; i < len(row) && x < l.margin+l.listWidth; x, i = x+max(1, row[i].Width), i+1 {
			if x < l.margin {
				continue
			}
			checked += max(1, row[i].Width)
			bg := row[i].Style.Bg
			if bg == nil || color.RGBAModel.Convert(bg) != color.RGBAModel.Convert(selection) {
				t.Fatalf("wrong background at column %d: %v", x, bg)
			}
		}
	}
	if checked != 2*l.listWidth {
		t.Fatalf("expected two highlighted lines, checked %d cells", checked)
	}
}

func TestViewFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {20, 10}, {40, 14}, {60, 24}, {80, 24}, {100, 32}, {160, 50}, {200, 15}} {
		for _, count := range []int{0, 1, 7, 100} {
			for _, mode := range []string{modeNormal, modeAdd, modeDelete, modeFilter} {
				for _, help := range []bool{false, true} {
					if mode == modeDelete && count == 0 {
						continue
					}
					t.Run(fmt.Sprintf("%dx%d/%d/%s/%v", size[0], size[1], count, mode, help), func(t *testing.T) {
						m := Model{section: sectionCourses, width: size[0], height: size[1], mode: mode, help: help, input: strings.Repeat("数学 très long ", 30)}
						m.courses = courses(count)
						m.cursor = count - 1
						m.clamp()
						view := m.View().Content
						if w, h := lipgloss.Width(view), lipgloss.Height(view); w > size[0] || h > size[1] {
							t.Fatalf("view %dx%d exceeds terminal %dx%d\n%s", w, h, size[0], size[1], ansi.Strip(view))
						}
						if count > 0 && !m.layout().tiny && !strings.Contains(ansi.Strip(view), fmt.Sprintf("Course %02d", count)) {
							t.Fatalf("selected course missing\n%s", ansi.Strip(view))
						}
					})
				}
			}
		}
	}
}

func TestHelpToggles(t *testing.T) {
	m := Model{width: 120, height: 40}
	if strings.Contains(ansi.Strip(m.View().Content), "toggle help") {
		t.Fatal("help shown before ?")
	}
	m = press(m, '?', "?")
	if !strings.Contains(ansi.Strip(m.View().Content), "toggle help") {
		t.Fatal("help hidden after ?")
	}
	m = press(m, tea.KeyEscape, "")
	if m.help {
		t.Fatal("esc should close help")
	}
}

func TestFilter(t *testing.T) {
	m := Model{section: sectionCourses, width: 120, height: 32, courses: []Course{{ID: 1, Name: "Algebra"}, {ID: 2, Name: "Physique"}, {ID: 3, Name: "Algèbre linéaire"}}}
	m = press(m, '/', "/")
	m = typeKeys(m, "alg")
	if got := len(m.visible()); got != 2 {
		t.Fatalf("visible = %d, want 2", got)
	}
	m = press(m, tea.KeyDown, "")
	m = press(m, tea.KeyEnter, "")
	if m.mode != modeNormal || m.selectedID() != 3 {
		t.Fatalf("enter should keep filter and selection: mode=%q id=%d", m.mode, m.selectedID())
	}
	m = press(m, tea.KeyEscape, "")
	if m.filter != "" || m.selectedID() != 3 || len(m.visible()) != 3 {
		t.Fatal("esc should clear the filter and keep the selection")
	}
	m = press(m, '/', "/")
	m = typeKeys(m, "zzz")
	if !strings.Contains(ansi.Strip(m.View().Content), "No course matches") {
		t.Fatal("empty filter result not explained")
	}
}

func TestCourseLifecycle(t *testing.T) {
	m := Model{section: sectionCourses, db: testDB(t), width: 100, height: 24}
	for i := 0; i < 12; i++ {
		m = press(m, 'a', "a")
		next, _ := m.Update(tea.PasteMsg{Content: fmt.Sprintf("Cours %d", i)})
		m = press(next.(Model), tea.KeyEnter, "")
	}
	if len(m.courses) != 12 || m.cursor != 11 || m.mode != modeNormal {
		t.Fatalf("add failed: %+v", m)
	}
	if m.courses[0].Added.IsZero() {
		t.Fatal("creation date not recorded")
	}
	if m.offset == 0 {
		t.Fatal("list should scroll to the new course")
	}
	m = press(m, tea.KeyHome, "")
	m = press(m, tea.KeyDown, "")
	if m.cursor != 1 {
		t.Fatal("down should move one row")
	}
	m = press(m, 'd', "d")
	m = press(m, tea.KeyEscape, "")
	if len(m.courses) != 12 {
		t.Fatal("cancel deleted a course")
	}

	// Reorder: Cours 1 moves before Cours 0, and stays selected.
	m = press(m, 'K', "K")
	if m.courses[0].Name != "Cours 1" || m.cursor != 0 {
		t.Fatalf("reorder failed: %q cursor=%d", m.courses[0].Name, m.cursor)
	}
	if err := m.load(); err != nil || m.courses[0].Name != "Cours 1" {
		t.Fatal("order not persisted")
	}

	m = press(m, 'e', "e")
	m.input = "Analyse"
	m = press(m, tea.KeyEnter, "")
	if m.courses[0].Name != "Analyse" || m.mode != modeNormal {
		t.Fatal("rename failed")
	}
	m = press(m, 'e', "e")
	m.input = "Cours 0"
	m = press(m, tea.KeyEnter, "")
	if m.mode != modeRename || !m.failed {
		t.Fatal("duplicate rename accepted")
	}
	m = press(m, tea.KeyEscape, "")

	m = press(m, tea.KeyEnd, "")
	m = press(m, 'd', "d")
	m = press(m, 'y', "y")
	if len(m.courses) != 11 || m.cursor != 10 {
		t.Fatal("delete should clamp selection")
	}
	m = press(m, 'a', "a")
	m = press(m, tea.KeyEnter, "")
	if m.mode != modeAdd || len(m.courses) != 11 {
		t.Fatal("empty name accepted")
	}
	m.input = "Cours 0"
	m = press(m, tea.KeyEnter, "")
	if m.mode != modeAdd || len(m.courses) != 11 {
		t.Fatal("duplicate name accepted")
	}
}

func TestMigrateKeepsExistingCourses(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("CREATE TABLE courses (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE); INSERT INTO courses (name) VALUES ('A'), ('B')"); err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatal("migration should be idempotent:", err)
	}
	m := Model{db: db}
	if err := m.load(); err != nil || len(m.courses) != 2 || m.courses[1].Name != "B" || !m.courses[0].Added.IsZero() {
		t.Fatalf("old courses lost: %v %+v", err, m.courses)
	}
}

func TestLectureLifecycle(t *testing.T) {
	m := Model{db: testDB(t), width: 160, height: 40}
	if m.section != sectionDashboard {
		t.Fatal("the dashboard should be the default section")
	}
	m = shiftTab(m) // Dashboard → Courses.
	m = press(m, 'a', "a")
	m = typeKeys(m, "QuaMech")
	m = press(m, tea.KeyEnter, "")
	m = shiftTab(m) // Courses → Lectures.
	if m.section != sectionLectures {
		t.Fatal("2 should open the lectures")
	}

	// The title is prefilled with the next number.
	m = press(m, 'a', "a")
	if m.mode != modeLectureAdd || m.input != "1. " {
		t.Fatalf("add lecture: mode=%q input=%q", m.mode, m.input)
	}
	m = typeKeys(m, "Linear Algebra")
	m = press(m, tea.KeyEnter, "")
	if len(m.lectures) != 1 || m.dashLecture() != 0 {
		t.Fatalf("lecture not added and selected: %+v", m.lectures)
	}
	first := m.lectures[0]
	if first.Title != "1. Linear Algebra" || !first.Date.Equal(today()) || first.Theory != "none" || first.Exo != "to do" {
		t.Fatalf("unexpected new lecture %+v", first)
	}

	m = press(m, 't', "t")
	m = press(m, 't', "t")
	m = press(m, 'x', "x")
	m = press(m, 'X', "X")
	m = press(m, 'X', "X")
	if l := m.lectures[0]; l.Theory != "done" || l.Exo != "no exo" {
		t.Fatalf("status cycling: %+v", l)
	}
	m = press(m, ']', "]")
	m = press(m, '>', ">")
	if want := today().AddDate(0, 0, 8); !m.lectures[0].Date.Equal(want) {
		t.Fatalf("date shift: %v, want %v", m.lectures[0].Date, want)
	}
	m = press(m, 'w', "w")
	m.input = "15/09/2026"
	m = press(m, tea.KeyEnter, "")
	if got := m.lectures[0].Date.Format(dateLayout); got != "2026-09-15" || m.mode != modeNormal {
		t.Fatalf("typed date: %s", got)
	}
	m = press(m, 'w', "w")
	m.input = "someday"
	m = press(m, tea.KeyEnter, "")
	if m.mode != modeLectureDate || !m.failed {
		t.Fatal("invalid date accepted")
	}
	m = press(m, tea.KeyEscape, "")

	// The next lecture comes a week after the last one, on the "+ New lecture" row.
	m = press(m, 'J', "J")
	if m.dashLecture() != -1 {
		t.Fatal("J should reach the new lecture row")
	}
	m = press(m, 'K', "K")
	if m.dashLecture() != 0 {
		t.Fatal("K should select the lecture above")
	}
	m = press(m, 'J', "J")
	m = press(m, tea.KeyEnter, "")
	if m.input != "2. " {
		t.Fatalf("enter on new row should add: %q", m.input)
	}
	m = typeKeys(m, "Postulates")
	m = press(m, tea.KeyEnter, "")
	if len(m.lectures) != 2 || m.lectures[1].Date.Format(dateLayout) != "2026-09-22" {
		t.Fatalf("second lecture: %+v", m.lectures)
	}
	m = press(m, 'e', "e")
	m.input = "2. Postulates (Measurement)"
	m = press(m, tea.KeyEnter, "")
	if m.lectures[1].Title != "2. Postulates (Measurement)" {
		t.Fatal("title edit failed")
	}
	m = press(m, 'd', "d")
	m = press(m, 'y', "y")
	if len(m.lectures) != 1 {
		t.Fatal("lecture not deleted")
	}

	// Deleting a course deletes its lectures.
	m = press(m, tea.KeyTab, "") // Lectures → Courses.
	m = press(m, 'd', "d")
	m = press(m, 'y', "y")
	var n int
	m.db.QueryRow("SELECT COUNT(*) FROM lectures").Scan(&n)
	if len(m.courses) != 0 || len(m.lectures) != 0 || n != 0 {
		t.Fatal("course lectures left behind")
	}
}

func TestParseDate(t *testing.T) {
	ref := time.Date(2026, 9, 15, 0, 0, 0, 0, time.Local)
	for in, want := range map[string]string{
		"22/09/2026": "2026-09-22", "1/10/2026": "2026-10-01", "22.09.2026": "2026-09-22",
		"2026-09-22": "2026-09-22", "22/09": "2026-09-22", "+7": "2026-09-22", "-1": "2026-09-14",
		"+1w": "2026-09-22", "-2s": "2026-09-01", "today": today().Format(dateLayout),
	} {
		got, err := parseDate(in, ref)
		if err != nil || got.Format(dateLayout) != want {
			t.Errorf("parseDate(%q) = %v, %v; want %s", in, got, err, want)
		}
	}
	if _, err := parseDate("31/02/2026", ref); err == nil {
		t.Error("impossible date accepted")
	}
}

func TestLecturesFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{40, 14}, {80, 24}, {120, 30}, {139, 40}, {140, 40}, {200, 15}} {
		for _, count := range []int{0, 1, 9} {
			for _, mode := range []string{modeNormal, modeLectureAdd, modeLectureDate, modeLectureDelete} {
				for _, help := range []bool{false, true} {
					if count == 0 && mode != modeNormal {
						continue
					}
					m := Model{width: size[0], height: size[1], section: sectionLectures, help: help, input: strings.Repeat("très long ", 30)}
					m.courses = courses(count)
					for i := 0; i < count*4; i++ {
						m.lectures = append(m.lectures, Lecture{ID: i + 1, CourseID: i%max(1, count) + 1, Title: strings.Repeat("Lecture ", 10),
							Date: today().AddDate(0, 0, i), Theory: theoryStates[i%3], Exo: exoStates[i%4]})
					}
					m.dashCourse, m.dashRow = count-1, 2
					m.clamp()
					if count > 0 && mode != modeNormal {
						m.mode = mode
					}
					view := m.View().Content
					if w, h := lipgloss.Width(view), lipgloss.Height(view); w > size[0] || h > size[1] {
						t.Fatalf("%v count=%d mode=%q: view %dx%d exceeds terminal\n%s", size, count, mode, w, h, ansi.Strip(view))
					}
					if count > 0 && !m.layout().tiny && m.layout().bodyHeight > 12 && !strings.Contains(ansi.Strip(view), "▌") {
						t.Fatalf("%v count=%d: selected lecture off screen\n%s", size, count, ansi.Strip(view))
					}
				}
			}
		}
	}
}

func TestLecturesCourseNavigation(t *testing.T) {
	m := Model{section: sectionLectures, width: 160, height: 40, courses: courses(5)}
	for i := 0; i < 10; i++ {
		m.lectures = append(m.lectures, Lecture{ID: i + 1, CourseID: i%5 + 1, Title: "L", Date: today(), Theory: "none", Exo: "to do"})
	}
	m.focusCourse(0)
	m.clamp()
	if m.dashLecture() != -1 {
		t.Fatal("start on + New lecture")
	}
	m = press(m, 'K', "K")
	if m.dashRow != 1 {
		t.Fatalf("K should select the last lecture, row %d", m.dashRow)
	}
	// Two columns: down skips to the block below, right to the next one.
	m = press(m, tea.KeyDown, "")
	if m.dashCourse != 2 || m.dashLecture() != -1 {
		t.Fatalf("down: course %d row %d", m.dashCourse, m.dashRow)
	}
	m = press(m, tea.KeyRight, "")
	m = press(m, tea.KeyUp, "")
	if m.dashCourse != 1 || m.dashLecture() != -1 {
		t.Fatalf("right then up: course %d", m.dashCourse)
	}
	m = press(m, tea.KeyLeft, "")
	m = press(m, tea.KeyLeft, "")
	if m.dashCourse != 0 {
		t.Fatal("left should stop at the first course")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	if m = next.(Model); m.dashRow != 1 {
		t.Fatalf("shift+up should select a lecture, row %d", m.dashRow)
	}
}

func TestWeekNumbersFollowTheLectures(t *testing.T) {
	d := func(s string) time.Time { t, _ := time.ParseInLocation(dateLayout, s, time.Local); return t }
	m := Model{}
	add := func(day time.Time) {
		m.lectures = append(m.lectures, Lecture{ID: len(m.lectures) + 1, CourseID: 1, Date: day, Theory: "none", Exo: "to do"})
	}
	// Autumn: a lecture every Thursday from 10 Sep to 17 Dec, and one on Mon 7 Sep.
	add(d("2026-09-07"))
	for day := d("2026-09-10"); !day.After(d("2026-12-17")); day = day.AddDate(0, 0, 7) {
		add(day)
	}
	// Spring starts after the winter break.
	add(d("2027-02-16"))
	add(d("2027-02-23"))
	for week, want := range map[string]int{
		"2026-08-31": 0, // Before any lecture.
		"2026-09-07": 1, // First week.
		"2026-09-14": 2,
		"2026-11-16": 11, // Weeks keep counting between lectures.
		"2026-12-14": 15,
		"2026-12-28": 17, // Two weeks after: still the semester's tail.
		"2027-01-04": 18, // The break counts on until the next lecture…
		"2027-02-15": 1,  // …which, after three empty weeks, starts week 1 again.
		"2027-02-22": 2,
	} {
		if got := m.weekNumber(d(week)); got != want {
			t.Errorf("week of %s: %d, want %d", week, got, want)
		}
	}
	// Previous weeks only reach back to the start of the semester.
	m.dashWeek = d("2027-02-22")
	if items := m.workItems(colPrevious); len(items) != 1 || m.lectures[items[0]].Date != d("2027-02-16") {
		t.Fatalf("previous weeks crossed the semester: %v", items)
	}
}

func TestParsePlan(t *testing.T) {
	for in, want := range map[string][4]string{
		"8-10 CompSec : S9":             {"1", "08:00", "10:00", "CompSec : S9"},
		"mar 08h00 - 10h00 : PhyMec":    {"1", "08:00", "10:00", "PhyMec"},
		"jeudi 14h Gym":                 {"3", "14:00", "", "Gym"},
		"sun 9:30-11 Read chapter 3":    {"6", "09:30", "11:00", "Read chapter 3"},
		"10h15 → 12h Assistanat":        {"1", "10:15", "12:00", "Assistanat"},
		"ven 2 problems of series 4":    {"4", "02:00", "", "problems of series 4"},
		"Mardi 9 Cours 3 relire 2 fois": {"1", "09:00", "", "Cours 3 relire 2 fois"},
	} {
		day, start, end, text, err := parsePlan(in, 1)
		if err != nil || [4]string{fmt.Sprint(day), start, end, text} != want {
			t.Errorf("parsePlan(%q) = %d %q %q %q %v, want %v", in, day, start, end, text, err, want)
		}
	}
	for _, bad := range []string{"", "mar", "25-26 Nope", "8-10", "Laundry", "jeu Gym"} {
		if _, _, _, _, err := parsePlan(bad, 0); err == nil {
			t.Errorf("parsePlan(%q) accepted", bad)
		}
	}
}

func TestDashboardWorkAndPlans(t *testing.T) {
	m := Model{db: testDB(t), width: 160, height: 44}
	if _, err := m.db.Exec(`INSERT INTO courses (name, position) VALUES ('Computer Security and Privacy', 1);
		INSERT INTO lectures (course_id, title, date, theory, exo) VALUES
		(1, '1. Intro', ?, 'done', 'to do'), (1, '2. Principles', ?, 'none', 'done')`,
		today().AddDate(0, 0, -14).Format(dateLayout), today().Format(dateLayout)); err != nil {
		t.Fatal(err)
	}
	if err := m.load(); err != nil {
		t.Fatal(err)
	}
	m.goToWeek(weekStart(time.Now()))
	if got := m.weekNumber(m.viewWeek()); got != 3 {
		t.Fatalf("week %d, want 3", got)
	}
	// Exercises: this week's lecture is done, the old one is late.
	if len(m.workItems(colThisWeek)) != 1 || len(m.workItems(colPrevious)) != 1 {
		t.Fatal("unexpected exercises")
	}
	m = key(m, tea.KeyRight, tea.ModCtrl) // To previous weeks.
	m = press(m, ' ', "space")
	if m.lectures[0].Exo != "done" || len(m.workItems(colPrevious)) != 0 {
		t.Fatal("late work marked done should leave previous weeks at once")
	}
	m = press(m, 'u', "u")
	if m.lectures[0].Exo != "to do" || len(m.workItems(colPrevious)) != 1 || m.selectedWork() != 0 {
		t.Fatal("u should bring it back, selected")
	}
	// This week keeps its done lectures (this one starts done), struck through.
	m = key(m, tea.KeyLeft, tea.ModCtrl)
	if m.lectures[1].Exo != "done" || len(m.workItems(colThisWeek)) != 1 {
		t.Fatal("this week should list its done lecture")
	}
	m = press(m, ' ', "space")
	m = press(m, ' ', "space")
	if m.lectures[1].Exo != "done" || len(m.workItems(colThisWeek)) != 1 || m.selectedWork() != 1 {
		t.Fatal("this week should keep a lecture marked done")
	}
	m = press(m, 't', "t")
	if len(m.workItems(colThisWeek)) != 1 || len(m.workItems(colPrevious)) != 0 {
		t.Fatal("theory tab")
	}

	// Plans: add from anywhere, edit, delete and undo.
	m = press(m, 'a', "a")
	m = typeKeys(m, "8-10 CompSec : S9")
	m = press(m, tea.KeyEnter, "")
	if len(m.plans) != 1 || !m.plans[0].Day.Equal(today()) || m.selectedPlan() != 0 {
		t.Fatalf("plan not added today: %+v", m.plans)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "CompSec : S9") {
		t.Fatal("plan not shown")
	}
	m = press(m, 'e', "e")
	if m.input != "08h00-10h00 CompSec : S9" {
		t.Fatalf("edit prefill %q", m.input)
	}
	m.input = "dim 9-10 CompSec : S10"
	m = press(m, tea.KeyEnter, "")
	if p := m.plans[0]; p.Text != "CompSec : S10" || p.Day.Weekday() != time.Sunday || m.dashDay != 6 {
		t.Fatalf("edit failed: %+v", p)
	}
	m = press(m, 'c', "c")
	if len(m.plans) != 2 || !m.plans[1].Day.Equal(m.plans[0].Day.AddDate(0, 0, 7)) {
		t.Fatal("copy to next week failed")
	}
	m = press(m, 'd', "d")
	if len(m.plans) != 1 {
		t.Fatal("delete failed")
	}
	m = press(m, 'u', "u")
	if len(m.plans) != 2 || m.selectedPlan() < 0 {
		t.Fatal("undo failed")
	}
	m = press(m, ']', "]")
	if m.weekNumber(m.viewWeek()) != 4 || len(m.plansOn(m.viewWeek().AddDate(0, 0, 6))) != 1 {
		t.Fatal("next week should hold the copy")
	}
	if m.courseFor("CompSec") != 0 || m.courseFor("PhyMec") != -1 || m.courseFor("CSP") != -1 || m.courseFor("SA") != -1 {
		t.Fatal("course abbreviations")
	}
}

func TestDashboardFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{40, 14}, {60, 20}, {80, 24}, {120, 30}, {160, 44}, {220, 60}, {200, 15}} {
		for _, n := range []int{0, 3, 40} {
			for _, mode := range []string{modeNormal, modePlanAdd} {
				for _, help := range []bool{false, true} {
					m := Model{width: size[0], height: size[1], help: help, mode: mode, input: strings.Repeat("très long ", 20)}
					m.courses = courses(3)
					for i := 0; i < n; i++ {
						m.lectures = append(m.lectures, Lecture{ID: i + 1, CourseID: i%3 + 1, Title: strings.Repeat("Lecture ", 8),
							Date: today().AddDate(0, 0, -i), Theory: "none", Exo: exoStates[i%3]})
						m.plans = append(m.plans, Plan{ID: i + 1, Day: weekStart(today()).AddDate(0, 0, i%7), Start: "08:00", End: "10:00", Text: strings.Repeat("CS long plan ", 5)})
					}
					m.goToWeek(weekStart(time.Now()))
					m.dashZone = n % 2
					m.clamp()
					view := m.View().Content
					if w, h := lipgloss.Width(view), lipgloss.Height(view); w > size[0] || h > size[1] {
						t.Fatalf("%v n=%d mode=%q help=%v: %dx%d\n%s", size, n, mode, help, w, h, ansi.Strip(view))
					}
				}
			}
		}
	}
}

func TestCourseNickname(t *testing.T) {
	m := Model{db: testDB(t), section: sectionCourses, width: 140, height: 40}
	for _, name := range []string{"Computer Security and Privacy", "Principles of Online Decision Making"} {
		m = press(m, 'a', "a")
		m = typeKeys(m, name)
		m = press(m, tea.KeyEnter, "")
	}
	m = press(m, tea.KeyHome, "")
	m = press(m, 'n', "n")
	m = typeKeys(m, "CompSec")
	m = press(m, tea.KeyEnter, "")
	if m.courses[0].Nick != "CompSec" || m.mode != modeNormal {
		t.Fatalf("nickname not saved: %+v", m.courses[0])
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "CompSec") {
		t.Fatal("nickname not shown")
	}
	// Nicknames are unique, filterable, and what plans and labels use.
	m = press(m, 'j', "j")
	m = press(m, 'n', "n")
	m = typeKeys(m, "compsec")
	m = press(m, tea.KeyEnter, "")
	if m.mode != modeNick || !m.failed {
		t.Fatal("duplicate nickname accepted")
	}
	m.input = "PODM"
	m = press(m, tea.KeyEnter, "")
	if m.courses[1].label() != "PODM" || m.courseFor("podm") != 1 || m.courseFor("CompSec") != 0 {
		t.Fatal("labels should use nicknames")
	}
	m = press(m, '/', "/")
	m = typeKeys(m, "podm")
	if v := m.visible(); len(v) != 1 || v[0] != 1 {
		t.Fatal("filter should match nicknames")
	}
	m = press(m, tea.KeyEscape, "")
	m = press(m, 'n', "n")
	m.input = ""
	m = press(m, tea.KeyEnter, "")
	if m.courses[1].Nick != "" {
		t.Fatal("empty nickname should remove it")
	}
}

func key(m Model, code rune, mod tea.KeyMod) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: code, Mod: mod})
	return next.(Model)
}

func TestInputCursor(t *testing.T) {
	m := Model{db: testDB(t), section: sectionCourses, width: 120, height: 30}
	m = press(m, 'a', "a")
	m = typeKeys(m, "omputer Security")
	// Forgot the first letter: go home and type it.
	m = press(m, tea.KeyHome, "")
	m = press(m, 'C', "C")
	if m.input != "Computer Security" {
		t.Fatalf("home + type: %q", m.input)
	}
	m = press(m, tea.KeyEnd, "")
	m = key(m, tea.KeyLeft, tea.ModCtrl) // Before "Security".
	m = typeKeys(m, "and ")
	m = key(m, tea.KeyLeft, 0)
	m = press(m, tea.KeyBackspace, "")
	if m.input != "Computer an Security" {
		t.Fatalf("word jump + backspace: %q", m.input)
	}
	m = press(m, 'd', "d")
	m = press(m, tea.KeyDelete, "") // Removes the space after the cursor.
	if m.input != "Computer andSecurity" {
		t.Fatalf("delete forward: %q", m.input)
	}
	m = typeKeys(m, " ")
	m = key(m, tea.KeyRight, 0)
	m = key(m, tea.KeyRight, 0)
	if m.input != "Computer and Security" || m.caret() != len("Computer and Se") {
		t.Fatalf("right: %q caret %d", m.input, m.caret())
	}
	m = key(m, 'w', tea.ModCtrl)
	if m.input != "Computer and curity" {
		t.Fatalf("ctrl+w: %q", m.input)
	}
	// The view shows the text around the cursor, even when long.
	m.input = strings.Repeat("x", 300) + "MIDDLE" + strings.Repeat("y", 300)
	m.inputBack = 303
	if !strings.Contains(ansi.Strip(m.View().Content), "MIDD") {
		t.Fatal("cursor area not visible")
	}
	m = press(m, tea.KeyEscape, "")
	m = press(m, 'a', "a")
	if m.input != "" || m.inputBack != 0 {
		t.Fatal("a new input should start empty, cursor at the end")
	}
	// The filter, typed live, follows edits in the middle too.
	m = press(m, tea.KeyEscape, "")
	m = press(m, '/', "/")
	m = typeKeys(m, "lgebra")
	m = press(m, tea.KeyHome, "")
	m = press(m, 'a', "a")
	if m.filter != "algebra" {
		t.Fatalf("filter %q", m.filter)
	}
}

func TestQuickPlan(t *testing.T) {
	m := Model{db: testDB(t), width: 160, height: 44}
	if _, err := m.db.Exec("INSERT INTO courses (name, nick, position) VALUES ('Computer Security and Privacy', 'CompSec', 1)"); err != nil {
		t.Fatal(err)
	}
	if err := m.load(); err != nil {
		t.Fatal(err)
	}
	m.goToWeek(weekStart(time.Now()))

	// 2, "8 10", space: the description pop-up opens for Tuesday.
	m = press(m, '2', "2")
	if m.mode != modePlanTime || m.dashDay != 1 || m.section != sectionDashboard {
		t.Fatalf("2 should plan Tuesday: mode %q day %d", m.mode, m.dashDay)
	}
	m = typeKeys(m, "8 10 ")
	if m.mode != modePlanText || m.planStart != "08:00" || m.planEnd != "10:00" {
		t.Fatalf("space after the end time: mode %q %q-%q", m.mode, m.planStart, m.planEnd)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Tuesday") || !strings.Contains(view, "08h00 → 10h00") {
		t.Fatalf("pop-up missing\n%s", view)
	}
	m = typeKeys(m, "CompSec : S9")
	m = press(m, tea.KeyEnter, "")
	if m.mode != modeNormal || len(m.plans) != 1 {
		t.Fatal("plan not saved")
	}
	p := m.plans[0]
	if p.Day.Weekday() != time.Tuesday || p.Start != "08:00" || p.End != "10:00" || p.Text != "CompSec : S9" {
		t.Fatalf("saved %+v", p)
	}

	// A plan needs a time: words straight away are refused.
	m = press(m, '5', "5")
	m = typeKeys(m, "G")
	if m.mode != modePlanTime || !m.failed || m.input != "" {
		t.Fatalf("plan without time accepted: %q %q", m.mode, m.input)
	}
	m = press(m, tea.KeyEnter, "")
	if m.mode != modePlanTime {
		t.Fatal("enter without time accepted")
	}
	m = typeKeys(m, "18h")
	m = typeKeys(m, "Gym")
	m = press(m, tea.KeyEnter, "")
	if p := m.plans[len(m.plans)-1]; p.Day.Weekday() != time.Friday || p.Start != "18:00" || p.Text != "Gym" {
		t.Fatalf("time then words %+v", p)
	}

	// One time then enter; backspace on an empty description goes back.
	m = press(m, '7', "7")
	m = typeKeys(m, "14h30")
	m = press(m, tea.KeyEnter, "")
	if m.mode != modePlanText || m.planStart != "14:30" || m.planEnd != "" {
		t.Fatalf("single time: %q %q", m.planStart, m.planEnd)
	}
	m = press(m, tea.KeyBackspace, "")
	if m.mode != modePlanTime || m.input != "14h30" {
		t.Fatalf("backspace should return to the time: %q %q", m.mode, m.input)
	}
	m = typeKeys(m, " 16 ")
	m = typeKeys(m, "Read")
	m = press(m, tea.KeyEnter, "")
	if p := m.plans[len(m.plans)-1]; p.Day.Weekday() != time.Sunday || p.Start != "14:30" || p.End != "16:00" {
		t.Fatalf("edited time %+v", p)
	}

	// A bad time is refused and the pop-up stays.
	m = press(m, '1', "1")
	m = typeKeys(m, "25 ")
	m = typeKeys(m, "x")
	if m.mode != modePlanTime || !m.failed {
		t.Fatal("bad time accepted")
	}
	m = press(m, tea.KeyEscape, "")
	if m.mode != modeNormal || len(m.plans) != 3 {
		t.Fatal("esc should cancel")
	}
	// Numbers no longer switch sections.
	if m.section != sectionDashboard {
		t.Fatal("left the dashboard")
	}
}

func TestWeekNumberCorrection(t *testing.T) {
	m := Model{db: testDB(t), width: 160, height: 44}
	if _, err := m.db.Exec(`INSERT INTO courses (name, position) VALUES ('A', 1);
		INSERT INTO lectures (course_id, title, date) VALUES (1, 'old', ?), (1, 'first', ?)`,
		weekStart(today()).AddDate(0, 0, -70).Format(dateLayout), weekStart(today()).AddDate(0, 0, -63).Format(dateLayout)); err != nil {
		t.Fatal(err)
	}
	if err := m.load(); err != nil {
		t.Fatal(err)
	}
	m.goToWeek(weekStart(time.Now()))
	if n := m.weekNumber(m.viewWeek()); n != 11 {
		t.Fatalf("detected week %d", n)
	}
	m = press(m, '#', "#")
	if m.input != "11" {
		t.Fatalf("prefill %q", m.input)
	}
	m = press(m, tea.KeyBackspace, "")
	m = press(m, tea.KeyBackspace, "")
	m = typeKeys(m, "10")
	m = press(m, tea.KeyEnter, "")
	if n := m.weekNumber(m.viewWeek()); n != 10 {
		t.Fatalf("corrected week %d", n)
	}
	// Persisted, and lectures before the new week 1 can still be late.
	m2 := Model{db: m.db}
	if err := m2.load(); err != nil || m2.weekNumber(weekStart(today())) != 10 {
		t.Fatal("correction not saved")
	}
	m2.goToWeek(weekStart(time.Now()))
	if len(m2.workItems(colPrevious)) != 2 {
		t.Fatal("the lecture before week 1 vanished from the late list")
	}
	// A later semester, after a break, starts at week 1 regardless.
	next := weekStart(today()).AddDate(0, 0, 7*12)
	m2.lectures = append(m2.lectures, Lecture{ID: 99, CourseID: 1, Date: next, Theory: "none", Exo: "to do"})
	if n := m2.weekNumber(next.AddDate(0, 0, 7)); n != 2 {
		t.Fatalf("next semester week %d", n)
	}
}

func TestPlannerTimelineAlignsTheWeek(t *testing.T) {
	mon := weekStart(today())
	m := Model{width: 200, height: 50, courses: []Course{{ID: 1, Name: "Software Entreprise", Nick: "SwEnt"}}}
	add := func(day int, start, end, text string) {
		m.plans = append(m.plans, Plan{ID: len(m.plans) + 1, Day: mon.AddDate(0, 0, day), Start: start, End: end, Text: text})
	}
	add(0, "13:00", "15:00", "Mon thirteen")
	add(2, "12:10", "12:30", "Wed short")
	add(2, "13:00", "15:00", "Wed thirteen")
	add(4, "11:30", "13:00", "SwEnt Fri")
	add(4, "12:00", "13:00", "Fri overlap")
	m.goToWeek(mon)
	m.clamp()
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	row := func(text string) int {
		for y, l := range lines {
			if strings.Contains(l, text) {
				return y
			}
		}
		t.Fatalf("%q not shown\n%s", text, strings.Join(lines, "\n"))
		return -1
	}
	// Same hour, same line, whatever the day.
	if row("Mon thirteen") != row("Wed thirteen") {
		t.Fatal("13h plans not aligned across days")
	}
	// Earlier plans sit higher; a 20-minute plan still shows time and text.
	if !(row("SwEnt Fri") < row("Wed short") && row("Wed short") < row("Wed thirteen")) {
		t.Fatal("plans not in time order down the week")
	}
	if row("Wed short") != row("12h10 → 12h30")+1 {
		t.Fatal("short plan lost its text")
	}
	// Overlapping plans share the day side by side, the later one lower.
	fri := lines[row("SwEnt Fri")]
	if strings.Contains(fri, "overlap") || row("overlap") <= row("SwEnt Fri") {
		t.Fatal("overlapping plans should sit side by side, the later lower")
	}
	if !strings.Contains(lines[row("overlap")], "▌            ▌") && !strings.Contains(lines[row("overlap")-1], "12h00") {
		t.Fatal("overlap should start at 12h next to the first plan")
	}
	// The gutter shows the hours.
	if !strings.Contains(lines[row("Mon thirteen")-1], "13h") {
		t.Fatalf("13h label missing\n%s", strings.Join(lines, "\n"))
	}
}

func TestPlannerUsesTheScreen(t *testing.T) {
	mon := weekStart(today())
	for _, height := range []int{36, 44, 52, 60} {
		m := Model{width: 190, height: height, courses: courses(2)}
		for i := 0; i < 25; i++ { // Plenty of late work, as in a real semester.
			m.lectures = append(m.lectures, Lecture{ID: i + 1, CourseID: 1, Title: "x.", Date: mon.AddDate(0, 0, -7*(i/2+1)), Theory: "none", Exo: "to do"})
		}
		for i, s := range [][2]string{{"08:00", "10:00"}, {"10:00", "12:00"}, {"12:10", "12:30"}, {"13:00", "15:00"}, {"17:00", "19:00"}, {"11:30", "13:00"}} {
			m.plans = append(m.plans, Plan{ID: i + 1, Day: mon.AddDate(0, 0, i%5), Start: s[0], End: s[1], Text: "Plan"})
		}
		m.goToWeek(mon)
		m.clamp()
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		if len(lines) != height {
			t.Fatalf("height %d: %d lines", height, len(lines))
		}
		// Above the status bar, at most the "add" row and a spare line stay empty.
		blank := 0
		for y := len(lines) - 2; y >= 0 && strings.TrimSpace(lines[y]) == ""; y-- {
			blank++
		}
		if blank > 2 {
			t.Errorf("height %d: %d empty lines under the planner\n%s", height, blank, strings.Join(lines, "\n"))
		}
		// The hours get two lines each once there is room for it.
		row := func(label string) int {
			for y, l := range lines {
				if strings.HasPrefix(strings.TrimSpace(l), label) {
					return y
				}
			}
			return -1
		}
		if height >= 52 && row("16h")-row("15h") != 2 {
			t.Errorf("height %d: an hour takes %d lines, want 2", height, row("16h")-row("15h"))
		}
	}
}

func TestLecturesScrollShowsTheWholeCourse(t *testing.T) {
	m := Model{section: sectionLectures, width: 190, height: 44}
	for i, nick := range []string{"QuaMech", "SHS", "PODM", "CompSec", "SwEnt"} {
		m.courses = append(m.courses, Course{ID: i + 1, Name: "Course " + nick, Nick: nick})
		for j := 0; j < 10; j++ {
			m.lectures = append(m.lectures, Lecture{ID: i*10 + j + 1, CourseID: i + 1, Title: fmt.Sprintf("%d.", j+1),
				Date: today().AddDate(0, 0, 7*j), Theory: "none", Exo: "to do"})
		}
	}
	m.focusCourse(0)
	m.clamp()
	visible := func(nick string) bool {
		for _, l := range strings.Split(ansi.Strip(m.View().Content), "\n") {
			if strings.Contains(l, "◆ "+nick+" ") {
				return true
			}
		}
		return false
	}
	order := []string{"QuaMech", "SHS", "PODM", "CompSec", "SwEnt"}
	// Down the grid then back up: the selected course always shows its title.
	for _, k := range []rune{tea.KeyDown, tea.KeyDown, tea.KeyRight, tea.KeyUp, tea.KeyLeft, tea.KeyUp, tea.KeyRight, tea.KeyLeft, tea.KeyLeft} {
		m = press(m, k, "")
		if !visible(order[m.dashCourse]) {
			t.Fatalf("course %s selected but its title is off screen (offset %d)\n%s",
				order[m.dashCourse], m.dashOffset, ansi.Strip(m.View().Content))
		}
	}
	if m.dashCourse != 0 || m.dashOffset != 0 {
		t.Fatalf("back on the first course, the grid should be at the top: course %d offset %d", m.dashCourse, m.dashOffset)
	}
}

func TestDashboardCursor(t *testing.T) {
	mon := weekStart(today())
	m := Model{width: 190, height: 40, courses: courses(1)}
	for i := 0; i < 30; i++ { // Late work, more than fits.
		m.lectures = append(m.lectures, Lecture{ID: i + 1, CourseID: 1, Title: fmt.Sprintf("Late %02d", i),
			Date: mon.AddDate(0, 0, -7-i), Theory: "none", Exo: "to do"})
	}
	m.lectures = append(m.lectures, Lecture{ID: 99, CourseID: 1, Title: "Due now", Date: mon, Theory: "none", Exo: "to do"})
	m.plans = []Plan{{ID: 1, Day: mon.AddDate(0, 0, 2), Start: "09:00", End: "10:00", Text: "Plan"}}
	m.goToWeek(mon)
	m.clamp()
	selected := func() string {
		for _, l := range strings.Split(ansi.Strip(m.View().Content), "\n") {
			if i := strings.Index(l, "▌"); i >= 0 && strings.Contains(l, "Late") {
				return l[i:]
			}
		}
		return ""
	}
	if m.dashCol != colThisWeek {
		t.Fatal("start on this week")
	}
	m = key(m, tea.KeyRight, tea.ModCtrl)
	rows := m.workRows(m.layout())
	// Down past the bottom: the selection stays in sight, the list scrolls.
	for i := 0; i < rows+3; i++ {
		m = press(m, tea.KeyDown, "")
		if want := fmt.Sprintf("Late %02d", i+1); !strings.Contains(selected(), want) {
			t.Fatalf("after %d downs, %q not selected and shown: %q", i+1, want, selected())
		}
	}
	top := m.workTop[colPrevious]
	if top != 4 {
		t.Fatalf("scrolled to %d, want 4", top)
	}
	// Up: the cursor climbs first, the list scrolls only from the top line.
	for i := 0; i < rows-1; i++ {
		m = press(m, tea.KeyUp, "")
		if m.workTop[colPrevious] != top {
			t.Fatalf("scrolled up after %d ups while the cursor could still climb", i+1)
		}
	}
	m = press(m, tea.KeyUp, "")
	if m.workTop[colPrevious] != top-1 || !strings.Contains(selected(), "Late 03") {
		t.Fatalf("at the top line, up should scroll: top %d, %q", m.workTop[colPrevious], selected())
	}
	// The scroll position shows in the header, not over a lecture.
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "↑3") {
		t.Fatalf("scroll position missing\n%s", view)
	}
	// Arrows stay in the part; ctrl+arrows move between parts, which keep their cursor.
	for i := 0; i < 40; i++ {
		m = press(m, tea.KeyDown, "")
	}
	if m.dashZone != zoneWork {
		t.Fatal("down left the work list")
	}
	item := m.dashItems[colPrevious]
	m = key(m, tea.KeyDown, tea.ModCtrl)
	m = press(m, tea.KeyRight, "")
	m = press(m, tea.KeyRight, "")
	if m.dashZone != zonePlanner || m.dashDay != daysBetween(mon, today())+2 && m.dashDay != 6 {
		t.Fatalf("planner: zone %d day %d", m.dashZone, m.dashDay)
	}
	m = press(m, tea.KeyUp, "")
	if m.dashZone != zonePlanner {
		t.Fatal("up left the planner")
	}
	m = key(m, tea.KeyLeft, tea.ModCtrl)
	if m.dashZone != zoneWork || m.dashCol != colThisWeek {
		t.Fatal("ctrl+left should go to this week")
	}
	m = key(m, tea.KeyRight, tea.ModCtrl)
	if m.dashItems[colPrevious] != item {
		t.Fatal("previous weeks lost its cursor")
	}
}

func TestDemo(t *testing.T) {
	m := Model{db: testDB(t), width: 190, height: 50}
	if err := seedDemo(m.db, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := m.load(); err != nil {
		t.Fatal(err)
	}
	m.goToWeek(weekStart(time.Now()))
	m.clamp()
	if n := m.weekNumber(m.viewWeek()); n != 10 {
		t.Fatalf("week %d, want 10", n)
	}
	if this, late := len(m.workItems(colThisWeek)), len(m.workItems(colPrevious)); this != 4 || late != 2 {
		t.Fatalf("exercises: %d this week, %d late; want 4 and 2", this, late)
	}
	m.dashTab = tabTheory
	if late := len(m.workItems(colPrevious)); late != 1 {
		t.Fatalf("theory late: %d, want 1", late)
	}
	if len(m.plansOn(m.viewWeek())) != 3 || len(m.plansOn(m.viewWeek().AddDate(0, 0, 2))) != 3 || m.courseFor("ML") < 0 {
		t.Fatal("demo plan")
	}
}

func notesModel(t *testing.T, width int) Model {
	db := testDB(t)
	if _, err := db.Exec(`INSERT INTO courses (id, name, position) VALUES (1, 'Quantum Mechanics', 1);
		INSERT INTO lectures (course_id, title, date) VALUES (1, '1. Postulates', '2026-09-15')`); err != nil {
		t.Fatal(err)
	}
	m := Model{db: db, section: sectionLectures, width: width, height: 30}
	if err := m.load(); err != nil {
		t.Fatal(err)
	}
	m.dashRow = 0
	return m
}

func TestNotesSaveOnEsc(t *testing.T) {
	m := notesModel(t, 140)
	m = press(m, tea.KeyEnter, "")
	if m.mode != modeNotes || m.notes == nil || !m.notes.editing {
		t.Fatalf("enter on a lecture without notes should open the editor, mode=%q", m.mode)
	}
	m = typeKeys(m, "# Spin")
	m = press(m, tea.KeyEnter, "")
	m = typeKeys(m, "- **up** & down q")
	if m.mode != modeNotes {
		t.Fatal("typing q in the notes should not quit or leave")
	}
	var stored string
	m.db.QueryRow("SELECT notes FROM lectures").Scan(&stored)
	if stored != "" {
		t.Fatalf("notes saved before esc: %q", stored)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "unsaved") {
		t.Fatal("unsaved notes should say so")
	}

	// esc saves and shows the notes rendered.
	m = press(m, tea.KeyEscape, "")
	m.db.QueryRow("SELECT notes FROM lectures").Scan(&stored)
	if want := "# Spin\n- **up** & down q"; stored != want || m.lectures[0].Notes != want {
		t.Fatalf("saved %q, want %q", stored, want)
	}
	if m.mode != modeNotes || m.notes.editing {
		t.Fatal("esc in the editor should show the rendered notes")
	}
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"N O T E S", "1. Postulates", "Spin", "• up & down q", "preview"} {
		if !strings.Contains(view, want) {
			t.Fatalf("notes pane misses %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "**up**") || strings.Contains(view, "# Spin") {
		t.Fatalf("Markdown not rendered:\n%s", view)
	}
	for i, line := range strings.Split(m.View().Content, "\n") {
		if w := lipgloss.Width(line); w != 140 {
			t.Fatalf("line %d is %d wide: %q", i, w, ansi.Strip(line))
		}
	}

	// enter edits again; esc twice saves and closes.
	m = press(m, tea.KeyEnter, "")
	if !m.notes.editing || m.notes.area.Value() != stored {
		t.Fatalf("editing again: %q", m.notes.area.Value())
	}
	m = press(m, tea.KeyEscape, "")
	m = press(m, tea.KeyEscape, "")
	if m.mode != modeNormal || m.notes != nil {
		t.Fatal("esc on the rendered notes should close them")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "● Tue 15/09") {
		t.Fatal("a lecture with notes should show a dot before its date")
	}
	m = press(m, tea.KeyEnter, "")
	if m.notes == nil || m.notes.editing {
		t.Fatal("notes that exist should open rendered")
	}
}

func TestNotesNarrowAndQuit(t *testing.T) {
	m := notesModel(t, 70)
	m = press(m, tea.KeyEnter, "")
	m = typeKeys(m, "draft")
	for i, line := range strings.Split(m.View().Content, "\n") {
		if w := lipgloss.Width(line); w != 70 {
			t.Fatalf("line %d is %d wide: %q", i, w, ansi.Strip(line))
		}
	}
	// Quitting straight away still keeps the notes.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(Model)
	var stored string
	m.db.QueryRow("SELECT notes FROM lectures").Scan(&stored)
	if stored != "draft" {
		t.Fatalf("ctrl+c lost the notes: %q", stored)
	}
}
