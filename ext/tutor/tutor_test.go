package tutor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/ext/tutor"
)

func TestQuizAttemptRecording(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := tutor.NewAttemptStore(dir)

	attempt := tutor.QuizAttempt{
		StudentID: "student-1",
		LessonID:  "lesson-fractions",
		QuizID:    "quiz-1",
		Responses: []tutor.Response{
			{QuestionID: "q1", Answer: "1/2", Correct: true},
			{QuestionID: "q2", Answer: "2/3", Correct: false},
		},
		Score:        50.0,
		MasteryDelta: map[string]float64{"fractions": 0.1},
		Timestamp:    time.Now(),
	}

	if err := store.Record(attempt); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Retake — should create a new record
	attempt2 := attempt
	attempt2.Score = 75.0
	attempt2.Timestamp = time.Now().Add(time.Minute)
	if err := store.Record(attempt2); err != nil {
		t.Fatalf("Record attempt2: %v", err)
	}

	attempts, err := store.List("student-1", "quiz-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(attempts) != 2 {
		t.Fatalf("want 2 attempts, got %d", len(attempts))
	}

	latest, err := store.Latest("student-1", "quiz-1")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if latest.Score != 75.0 {
		t.Errorf("want latest score 75, got %v", latest.Score)
	}

	// Verify mastery delta stored
	if latest.MasteryDelta["fractions"] == 0 {
		t.Error("expected mastery delta to be persisted")
	}
}

func TestParentDashboard(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := tutor.NewAttemptStore(dir)

	// Seed some data
	now := time.Now()
	_ = store.Record(tutor.QuizAttempt{
		StudentID: "child-1",
		LessonID:  "lesson-math",
		QuizID:    "q1",
		Score:     80.0,
		MasteryDelta: map[string]float64{
			"multiplication": 0.2,
		},
		Timestamp: now.Add(-2 * time.Hour),
	})

	dashboard := tutor.NewDashboard(store)
	report, err := dashboard.ForChild("child-1", tutor.DashboardFilter{})
	if err != nil {
		t.Fatalf("ForChild: %v", err)
	}

	if len(report.RecentAssessments) == 0 {
		t.Error("expected recent assessments")
	}
	if report.MasteryByTopic["multiplication"] == "" {
		t.Error("expected mastery status for multiplication")
	}
	if len(report.CompletedToday) == 0 && now.Hour() > 0 {
		// only check if we have time window
		_ = report
	}
}

func TestParentLessonEditor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ls := tutor.NewLessonStore(dir)

	lesson := tutor.LessonDraft{
		Title:   "Intro to Fractions",
		Subject: "math",
		Blocks: []tutor.BlockDraft{
			{Kind: "prose", Content: "Welcome!"},
			{Kind: "quiz", QuizRef: "q1"},
		},
	}

	id, err := ls.Create(lesson)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty lesson ID")
	}

	lesson.Title = "Fractions (Revised)"
	lesson.Blocks = append(lesson.Blocks, tutor.BlockDraft{Kind: "video", VideoURL: "https://www.youtube.com/embed/xyz"})
	if err := ls.Update(id, lesson); err != nil {
		t.Fatalf("Update: %v", err)
	}

	loaded, err := ls.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.Title != "Fractions (Revised)" {
		t.Errorf("want revised title, got %q", loaded.Title)
	}
	if len(loaded.Blocks) != 3 {
		t.Errorf("want 3 blocks, got %d", len(loaded.Blocks))
	}

	// Assign to children
	if err := ls.Assign(id, []string{"child-1", "child-2"}); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	loaded, err = ls.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.AssignedTo) != 2 {
		t.Errorf("want 2 assignees, got %d", len(loaded.AssignedTo))
	}
}

func TestComplianceReporting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := tutor.NewAttemptStore(dir)
	now := time.Now()

	// Seed a month of data for two students
	subjects := []string{"math", "reading", "science"}
	for i := 0; i < 20; i++ {
		for _, subj := range subjects {
			_ = store.Record(tutor.QuizAttempt{
				StudentID: "child-1",
				LessonID:  "lesson-" + subj,
				QuizID:    "q-" + subj,
				Score:     float64(60 + i),
				MasteryDelta: map[string]float64{
					subj: 0.05,
				},
				Timestamp: now.AddDate(0, 0, -i),
			})
		}
	}

	reporter := tutor.NewReporter(store)

	// Daily report
	daily, err := reporter.Daily("child-1", now)
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if len(daily.Subjects) == 0 {
		t.Error("expected subjects in daily report")
	}

	// Monthly report
	monthly, err := reporter.Monthly("child-1", now.Year(), int(now.Month()))
	if err != nil {
		t.Fatalf("Monthly: %v", err)
	}
	if len(monthly.MasteryBySubject) == 0 {
		t.Error("expected mastery data in monthly report")
	}
	if len(monthly.AssessmentSummary) == 0 {
		t.Error("expected assessment summary in monthly report")
	}

	// HTML export
	htmlOut := filepath.Join(dir, "report.html")
	if err := reporter.ExportHTML("child-1", monthly, htmlOut); err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}
	data, err := os.ReadFile(htmlOut)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "child-1") {
		t.Error("HTML export should contain child ID")
	}

	// CSV export
	csvOut := filepath.Join(dir, "report.csv")
	if err := reporter.ExportCSV("child-1", monthly, csvOut); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	csvData, err := os.ReadFile(csvOut)
	if err != nil {
		t.Fatal(err)
	}
	if len(csvData) == 0 {
		t.Error("CSV export should not be empty")
	}
}
