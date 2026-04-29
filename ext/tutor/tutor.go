// Package tutor implements quiz attempt recording, mastery tracking, lesson editing, and reporting.
package tutor

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── Quiz attempts ─────────────────────────────────────────────────────────

// Response is a single question-level response within a quiz attempt.
type Response struct {
	QuestionID string `json:"question_id"`
	Answer     string `json:"answer"`
	Correct    bool   `json:"correct"`
}

// QuizAttempt records one student attempt at a quiz.
type QuizAttempt struct {
	StudentID    string             `json:"student_id"`
	LessonID     string             `json:"lesson_id"`
	QuizID       string             `json:"quiz_id"`
	Responses    []Response         `json:"responses,omitempty"`
	Score        float64            `json:"score"`
	MasteryDelta map[string]float64 `json:"mastery_delta,omitempty"`
	Timestamp    time.Time          `json:"timestamp"`
}

// AttemptStore records and retrieves quiz attempts using JSONL files.
type AttemptStore struct {
	dir string
	mu  sync.Mutex
}

// NewAttemptStore creates an AttemptStore backed by the given directory.
func NewAttemptStore(dir string) *AttemptStore {
	return &AttemptStore{dir: dir}
}

// Record appends a quiz attempt. Each attempt creates a new record (no overwrite).
func (s *AttemptStore) Record(a QuizAttempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.dir, "attempts.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(a)
}

func (s *AttemptStore) loadAll() ([]QuizAttempt, error) {
	path := filepath.Join(s.dir, "attempts.jsonl")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []QuizAttempt
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var a QuizAttempt
		if err := json.Unmarshal([]byte(line), &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// List returns all attempts for a given student and quiz.
func (s *AttemptStore) List(studentID, quizID string) ([]QuizAttempt, error) {
	all, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	var out []QuizAttempt
	for _, a := range all {
		if a.StudentID == studentID && a.QuizID == quizID {
			out = append(out, a)
		}
	}
	return out, nil
}

// Latest returns the most recent attempt for a student+quiz pair.
func (s *AttemptStore) Latest(studentID, quizID string) (QuizAttempt, error) {
	attempts, err := s.List(studentID, quizID)
	if err != nil {
		return QuizAttempt{}, err
	}
	if len(attempts) == 0 {
		return QuizAttempt{}, fmt.Errorf("no attempts for student=%q quiz=%q", studentID, quizID)
	}
	sort.Slice(attempts, func(i, j int) bool {
		return attempts[i].Timestamp.After(attempts[j].Timestamp)
	})
	return attempts[0], nil
}

// ForStudent returns all attempts for a student.
func (s *AttemptStore) ForStudent(studentID string) ([]QuizAttempt, error) {
	all, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	var out []QuizAttempt
	for _, a := range all {
		if a.StudentID == studentID {
			out = append(out, a)
		}
	}
	return out, nil
}

// ─── Dashboard ─────────────────────────────────────────────────────────────

// DashboardFilter restricts what the dashboard returns.
type DashboardFilter struct {
	DateFrom time.Time
	DateTo   time.Time
	Subject  string
}

// DashboardReport is the per-child dashboard output.
type DashboardReport struct {
	ChildID           string
	MasteryByTopic    map[string]string // topic → "not_started|in_progress|mastered|needs_review"
	RecentAssessments []QuizAttempt
	CompletedToday    []string // lesson IDs completed today
}

// Dashboard generates per-child dashboard reports.
type Dashboard struct {
	store *AttemptStore
}

// NewDashboard creates a Dashboard backed by the given AttemptStore.
func NewDashboard(store *AttemptStore) *Dashboard {
	return &Dashboard{store: store}
}

// ForChild builds a DashboardReport for the given child.
func (d *Dashboard) ForChild(childID string, filter DashboardFilter) (DashboardReport, error) {
	attempts, err := d.store.ForStudent(childID)
	if err != nil {
		return DashboardReport{}, err
	}

	report := DashboardReport{
		ChildID:        childID,
		MasteryByTopic: make(map[string]string),
	}

	// Accumulate mastery by topic
	masteryScore := make(map[string]float64)
	for _, a := range attempts {
		for topic, delta := range a.MasteryDelta {
			masteryScore[topic] += delta
		}
		report.RecentAssessments = append(report.RecentAssessments, a)
	}

	// Convert scores to labels
	for topic, score := range masteryScore {
		switch {
		case score == 0:
			report.MasteryByTopic[topic] = "not_started"
		case score < 0.3:
			report.MasteryByTopic[topic] = "in_progress"
		case score < 0.8:
			report.MasteryByTopic[topic] = "in_progress"
		default:
			report.MasteryByTopic[topic] = "mastered"
		}
	}

	// Find lessons completed today
	today := time.Now().Truncate(24 * time.Hour)
	seen := map[string]bool{}
	for _, a := range attempts {
		if a.Timestamp.After(today) && !seen[a.LessonID] {
			report.CompletedToday = append(report.CompletedToday, a.LessonID)
			seen[a.LessonID] = true
		}
	}

	// Keep only the 10 most recent assessments
	sort.Slice(report.RecentAssessments, func(i, j int) bool {
		return report.RecentAssessments[i].Timestamp.After(report.RecentAssessments[j].Timestamp)
	})
	if len(report.RecentAssessments) > 10 {
		report.RecentAssessments = report.RecentAssessments[:10]
	}

	return report, nil
}

// ─── Lesson store ──────────────────────────────────────────────────────────

// BlockDraft is a draft block within a lesson being authored.
type BlockDraft struct {
	Kind      string `json:"kind"`
	Content   string `json:"content,omitempty"`
	VideoURL  string `json:"video_url,omitempty"`
	QuizRef   string `json:"quiz_ref,omitempty"`
	AssetPath string `json:"asset_path,omitempty"`
	Caption   string `json:"caption,omitempty"`
}

// LessonDraft is a lesson being authored by a parent.
type LessonDraft struct {
	ID         string       `json:"id,omitempty"`
	Title      string       `json:"title"`
	Subject    string       `json:"subject"`
	Blocks     []BlockDraft `json:"blocks"`
	AssignedTo []string     `json:"assigned_to,omitempty"`
}

// LessonStore persists lesson drafts to disk.
type LessonStore struct {
	dir string
	mu  sync.Mutex
}

// NewLessonStore creates a LessonStore in the given directory.
func NewLessonStore(dir string) *LessonStore {
	return &LessonStore{dir: dir}
}

func (ls *LessonStore) path(id string) string {
	return filepath.Join(ls.dir, "lesson-"+id+".json")
}

// Create saves a new lesson and returns its generated ID.
func (ls *LessonStore) Create(draft LessonDraft) (string, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	draft.ID = id
	data, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		return "", err
	}
	return id, os.WriteFile(ls.path(id), data, 0644)
}

// Update overwrites an existing lesson.
func (ls *LessonStore) Update(id string, draft LessonDraft) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	draft.ID = id
	// Preserve assigned_to if not set
	existing, err := ls.get(id)
	if err == nil && len(draft.AssignedTo) == 0 {
		draft.AssignedTo = existing.AssignedTo
	}
	data, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ls.path(id), data, 0644)
}

func (ls *LessonStore) get(id string) (LessonDraft, error) {
	data, err := os.ReadFile(ls.path(id))
	if err != nil {
		return LessonDraft{}, err
	}
	var d LessonDraft
	return d, json.Unmarshal(data, &d)
}

// Get retrieves a lesson by ID.
func (ls *LessonStore) Get(id string) (LessonDraft, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.get(id)
}

// Assign sets the children assigned to a lesson.
func (ls *LessonStore) Assign(id string, children []string) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	d, err := ls.get(id)
	if err != nil {
		return err
	}
	d.AssignedTo = children
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ls.path(id), data, 0644)
}

// ─── Reporting ─────────────────────────────────────────────────────────────

// DailyReport summarizes one student's activity on a given day.
type DailyReport struct {
	ChildID  string
	Date     time.Time
	Subjects []string
	Lessons  []string
}

// MonthlyReport summarizes one student's activity over a month.
type MonthlyReport struct {
	ChildID           string
	Year              int
	Month             int
	MasteryBySubject  map[string]string
	AssessmentSummary []AssessmentSummaryRow
}

// AssessmentSummaryRow is one row in the assessment summary.
type AssessmentSummaryRow struct {
	LessonID  string
	QuizID    string
	BestScore float64
	Attempts  int
}

// Reporter generates compliance reports.
type Reporter struct {
	store *AttemptStore
}

// NewReporter creates a Reporter backed by the given AttemptStore.
func NewReporter(store *AttemptStore) *Reporter {
	return &Reporter{store: store}
}

// Daily returns a daily summary for the given child.
func (r *Reporter) Daily(childID string, day time.Time) (DailyReport, error) {
	attempts, err := r.store.ForStudent(childID)
	if err != nil {
		return DailyReport{}, err
	}
	start := day.Truncate(24 * time.Hour)
	end := start.Add(24 * time.Hour)

	report := DailyReport{ChildID: childID, Date: start}
	subjSet := map[string]bool{}
	lessonSet := map[string]bool{}
	for _, a := range attempts {
		if a.Timestamp.Before(start) || !a.Timestamp.Before(end) {
			continue
		}
		lessonSet[a.LessonID] = true
		for topic := range a.MasteryDelta {
			subjSet[topic] = true
		}
	}
	for s := range subjSet {
		report.Subjects = append(report.Subjects, s)
	}
	for l := range lessonSet {
		report.Lessons = append(report.Lessons, l)
	}
	return report, nil
}

// Monthly returns a monthly summary for the given child.
func (r *Reporter) Monthly(childID string, year, month int) (MonthlyReport, error) {
	attempts, err := r.store.ForStudent(childID)
	if err != nil {
		return MonthlyReport{}, err
	}

	report := MonthlyReport{
		ChildID:          childID,
		Year:             year,
		Month:            month,
		MasteryBySubject: make(map[string]string),
	}

	quizBest := make(map[string]float64)
	quizCount := make(map[string]int)
	masteryScore := make(map[string]float64)

	for _, a := range attempts {
		if a.Timestamp.Year() != year || int(a.Timestamp.Month()) != month {
			continue
		}
		key := a.LessonID + "|" + a.QuizID
		if a.Score > quizBest[key] {
			quizBest[key] = a.Score
		}
		quizCount[key]++
		for topic, delta := range a.MasteryDelta {
			masteryScore[topic] += delta
		}
	}

	for topic, score := range masteryScore {
		switch {
		case score >= 0.8:
			report.MasteryBySubject[topic] = "mastered"
		case score > 0:
			report.MasteryBySubject[topic] = "in_progress"
		default:
			report.MasteryBySubject[topic] = "not_started"
		}
	}

	for key, best := range quizBest {
		parts := strings.SplitN(key, "|", 2)
		lessonID, quizID := parts[0], parts[1]
		report.AssessmentSummary = append(report.AssessmentSummary, AssessmentSummaryRow{
			LessonID:  lessonID,
			QuizID:    quizID,
			BestScore: best,
			Attempts:  quizCount[key],
		})
	}

	return report, nil
}

var htmlReportTmpl = template.Must(template.New("monthly").Parse(`<!DOCTYPE html>
<html><head><title>Monthly Report — {{.ChildID}}</title></head><body>
<h1>Monthly Report: {{.ChildID}} ({{.Year}}-{{printf "%02d" .Month}})</h1>
<h2>Mastery by Subject</h2>
<table border="1"><tr><th>Subject</th><th>Status</th></tr>
{{range $k,$v := .MasteryBySubject}}<tr><td>{{$k}}</td><td>{{$v}}</td></tr>{{end}}
</table>
<h2>Assessment Summary</h2>
<table border="1"><tr><th>Lesson</th><th>Quiz</th><th>Best Score</th><th>Attempts</th></tr>
{{range .AssessmentSummary}}<tr><td>{{.LessonID}}</td><td>{{.QuizID}}</td><td>{{.BestScore}}</td><td>{{.Attempts}}</td></tr>{{end}}
</table>
</body></html>`))

// ExportHTML writes an HTML report to the given path.
func (r *Reporter) ExportHTML(childID string, report MonthlyReport, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return htmlReportTmpl.Execute(f, report)
}

// ExportCSV writes a CSV report to the given path.
func (r *Reporter) ExportCSV(childID string, report MonthlyReport, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"child_id", "lesson_id", "quiz_id", "best_score", "attempts"})
	for _, row := range report.AssessmentSummary {
		w.Write([]string{
			childID,
			row.LessonID,
			row.QuizID,
			fmt.Sprintf("%.2f", row.BestScore),
			fmt.Sprintf("%d", row.Attempts),
		})
	}
	w.Flush()
	return w.Error()
}
