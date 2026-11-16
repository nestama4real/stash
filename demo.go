package main

import (
	"database/sql"
	"fmt"
	"time"
)

// seedDemo fills an empty database with a typical week of a semester,
// dated around now: week 10, a few lectures due this week, a little late
// work, and a reasonably busy plan. It backs `stash --demo`.
func seedDemo(db *sql.DB, now time.Time) error {
	mon := weekStart(now)
	courses := []struct {
		name, nick string
		day        int // Weekday of the lecture, 0 is Monday.
		exo        bool
		titles     []string
	}{
		{"Quantum Mechanics", "QuaMech", 0, true, []string{"Linear Algebra", "Postulates", "Measurement",
			"Harmonic Oscillator", "Angular Momentum", "Spin", "Hydrogen Atom", "Perturbation Theory",
			"Identical Particles", "Entanglement"}},
		{"Computer Security and Privacy", "CompSec", 1, true, []string{"Introduction", "Security Principles",
			"Cryptography Basics", "Authentication", "Access Control", "Software Security", "Web Security",
			"Network Security", "Privacy", "Anonymity"}},
		{"Machine Learning", "ML", 2, true, []string{"Linear Regression", "Optimization", "Classification",
			"Kernels", "Neural Networks", "Backpropagation", "Convolutional Networks", "Unsupervised Learning",
			"Generative Models", "Transformers"}},
		{"Économie de la Chine", "SHS", 3, false, []string{"Introduction", "Chine 101", "Les réformes de 1978",
			"Un modèle de croissance", "Made in China", "Urbanisation", "Énergie", "Commerce extérieur",
			"Technologie", "Démographie"}},
		{"Software Engineering", "SwEng", 4, true, []string{"Agile", "Scrum", "Testing", "Continuous Integration",
			"Code Review", "Architecture", "Design Patterns", "Refactoring", "Security", "Release"}},
	}
	// What is not done yet: a little late work, and this week (week 10).
	type status struct{ theory, exo string }
	pending := map[[2]int]status{
		{0, 8}: {"none", "done"},    // QuaMech, week 9: theory to catch up on.
		{1, 8}: {"done", "to do"},   // CompSec, week 9: exercises late.
		{2, 7}: {"done", "ongoing"}, // ML, week 8: exercises half done.
		{0, 9}: {"done", "to do"},
		{1, 9}: {"ongoing", "to do"},
		{2, 9}: {"none", "to do"},
		{3, 9}: {"none", "no exo"},
		{4, 9}: {"none", "ongoing"},
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for ci, c := range courses {
		res, err := tx.Exec("INSERT INTO courses (name, nick, position, created_at) VALUES (?, ?, ?, ?)",
			c.name, c.nick, ci+1, mon.AddDate(0, 0, -70).Unix())
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		for w, title := range c.titles {
			s, ok := pending[[2]int{ci, w}]
			if !ok {
				s = status{"done", "done"}
			}
			if !c.exo || w == 0 {
				s.exo = "no exo" // No exercises yet in the first week.
			}
			date := mon.AddDate(0, 0, 7*(w-9)+c.day)
			if _, err := tx.Exec("INSERT INTO lectures (course_id, title, date, theory, exo) VALUES (?, ?, ?, ?, ?)",
				id, fmt.Sprintf("%d. %s", w+1, title), date.Format(dateLayout), s.theory, s.exo); err != nil {
				return err
			}
		}
	}
	plans := []struct {
		day              int
		start, end, text string
	}{
		{0, "08:00", "10:00", "QuaMech : lecture"},
		{0, "10:15", "12:00", "ML : series 9"},
		{0, "14:00", "16:00", "CompSec : lab 9"},
		{1, "09:00", "11:00", "CompSec : lecture"},
		{1, "13:00", "15:00", "QuaMech : series 10"},
		{1, "18:00", "19:30", "Climbing"},
		{2, "08:00", "10:00", "ML : lecture"},
		{2, "12:10", "12:30", "Call with the TA"},
		{2, "14:00", "17:00", "ML : project"},
		{3, "13:30", "14:30", "Office hours"},
		{3, "10:00", "12:00", "SHS : lecture"},
		{3, "16:00", "18:00", "SHS : essay at the library"},
		{4, "09:00", "10:30", "SwEng : lecture"},
		{4, "11:30", "13:00", "SwEng : scrum meeting"},
		{4, "14:00", "16:00", "CompSec : series 10"},
		{5, "10:00", "12:00", "ML : revision"},
		{6, "17:00", "18:00", "Plan next week"},
	}
	for _, p := range plans {
		if _, err := tx.Exec("INSERT INTO plans (day, start, end, text) VALUES (?, ?, ?, ?)",
			mon.AddDate(0, 0, p.day).Format(dateLayout), p.start, p.end, p.text); err != nil {
			return err
		}
	}
	return tx.Commit()
}
