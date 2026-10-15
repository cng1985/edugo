package store

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/cng1985/edugo/internal/modules/assessment/internal/grading"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/eventbus"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/xid"
	"gorm.io/gorm"
)

type Paper struct {
	ID               string `gorm:"primaryKey;size:36"`
	OwnerID          string `gorm:"size:36;index;not null"`
	OrgID            string `gorm:"size:36;index;not null"`
	Title            string `gorm:"size:200;not null"`
	Status           string `gorm:"size:32;index;not null"`
	CurrentVersionID string `gorm:"size:36"`
	Version          int64  `gorm:"not null;default:1"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (Paper) TableName() string { return "papers" }

type PaperVersion struct {
	ID             string `gorm:"primaryKey;size:36"`
	PaperID        string `gorm:"size:36;not null;uniqueIndex:uk_paper_ver"`
	VersionNo      int    `gorm:"not null;uniqueIndex:uk_paper_ver"`
	Status         string `gorm:"size:32;not null"`
	TotalScoreCenti int
	Rules          string `gorm:"type:text"`
	CreatedAt      time.Time
}

func (PaperVersion) TableName() string { return "paper_versions" }

type PaperQuestion struct {
	ID                string `gorm:"primaryKey;size:36"`
	PaperVersionID    string `gorm:"size:36;index;not null"`
	QuestionVersionID string `gorm:"size:36;index;not null"`
	SortOrder         int
	ScoreCenti        int
	Snapshot          string `gorm:"type:text"`
}

func (PaperQuestion) TableName() string { return "paper_questions" }

type Exam struct {
	ID              string `gorm:"primaryKey;size:36"`
	OwnerID         string `gorm:"size:36;index;not null"`
	OrgID           string `gorm:"size:36;index;not null"`
	PaperID         string `gorm:"size:36;index"`
	PaperVersionID  string `gorm:"size:36"`
	Title           string `gorm:"size:200;not null"`
	Status          string `gorm:"size:32;index;not null"`
	StartAt         *time.Time
	EndAt           *time.Time
	DurationSec     int
	MaxAttempts     int
	PassScoreCenti  int
	SubmitGraceSec  int
	PublishMode     string `gorm:"size:32"`
	Version         int64  `gorm:"not null;default:1"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (Exam) TableName() string { return "exams" }

type Eligibility struct {
	ID     string `gorm:"primaryKey;size:36"`
	ExamID string `gorm:"size:36;index;not null"`
	UserID string `gorm:"size:36;index;not null"`
}

func (Eligibility) TableName() string { return "exam_eligibility" }

type Attempt struct {
	ID             string `gorm:"primaryKey;size:36"`
	ExamID         string `gorm:"size:36;not null;uniqueIndex:uk_attempt"`
	UserID         string `gorm:"size:36;not null;uniqueIndex:uk_attempt"`
	AttemptNo      int    `gorm:"not null;uniqueIndex:uk_attempt"`
	Status         string `gorm:"size:32;index;not null"`
	StartedAt      time.Time
	DeadlineAt     time.Time
	SubmittedAt    *time.Time
	ScoreCenti     *int
	ScoreVersion   int
	Passed         *bool
	PaperVersionID string `gorm:"size:36"`
	StartKey       string `gorm:"size:128;uniqueIndex"`
	Version        int64  `gorm:"not null;default:1"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (Attempt) TableName() string { return "exam_attempts" }

type Answer struct {
	ID                 string `gorm:"primaryKey;size:36"`
	AttemptID          string `gorm:"size:36;not null;uniqueIndex:uk_attempt_q"`
	QuestionSnapshotID string `gorm:"size:36;not null;uniqueIndex:uk_attempt_q"`
	Answer             string `gorm:"type:text"`
	Version            int64  `gorm:"not null"`
	SavedAt            time.Time
	ScoreCenti         *int
	Correct            *bool
}

func (Answer) TableName() string { return "exam_answers" }

type GradingTask struct {
	ID        string `gorm:"primaryKey;size:36"`
	AttemptID string `gorm:"size:36;uniqueIndex"`
	Status    string `gorm:"size:32;index"`
	Error     string `gorm:"size:512"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (GradingTask) TableName() string { return "grading_tasks" }

type GradingResult struct {
	ID                 string `gorm:"primaryKey;size:36"`
	AttemptID          string `gorm:"size:36;index"`
	QuestionSnapshotID string `gorm:"size:36;index"`
	CandidateCenti     *int
	Feedback           string `gorm:"type:text"`
	Reason             string `gorm:"size:512"`
	Status             string `gorm:"size:32"`
	CreatedAt          time.Time
}

func (GradingResult) TableName() string { return "grading_results" }

type ScoreRevision struct {
	ID          string `gorm:"primaryKey;size:36"`
	AttemptID   string `gorm:"size:36;index"`
	FromVersion int
	ToVersion   int
	BeforeCenti int
	AfterCenti  int
	Reason      string `gorm:"size:512"`
	ActorID     string `gorm:"size:36"`
	CreatedAt   time.Time
}

func (ScoreRevision) TableName() string { return "score_revisions" }

func Models() []any {
	return []any{&Paper{}, &PaperVersion{}, &PaperQuestion{}, &Exam{}, &Eligibility{}, &Attempt{}, &Answer{}, &GradingTask{}, &GradingResult{}, &ScoreRevision{}}
}

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) CreatePaper(p Paper, v PaperVersion) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		p.CurrentVersionID = v.ID
		return tx.Create(&p).Error
	})
}

func (r *Repo) Paper(id string) (Paper, error) {
	var row Paper
	err := r.db.First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Paper{}, apperr.NotFound("试卷不存在")
	}
	return row, err
}

func (r *Repo) PaperVersion(id string) (PaperVersion, error) {
	var row PaperVersion
	err := r.db.First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return PaperVersion{}, apperr.NotFound("试卷版本不存在")
	}
	return row, err
}

func (r *Repo) Questions(versionID string) ([]PaperQuestion, error) {
	var rows []PaperQuestion
	err := r.db.Where("paper_version_id = ?", versionID).Order("sort_order asc").Find(&rows).Error
	return rows, err
}

func (r *Repo) ReplaceQuestions(paper Paper, version PaperVersion, questions []PaperQuestion, expected int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("paper_version_id = ?", version.ID).Delete(&PaperQuestion{}).Error; err != nil {
			return err
		}
		if len(questions) > 0 {
			if err := tx.Create(&questions).Error; err != nil {
				return err
			}
		}
		if err := tx.Save(&version).Error; err != nil {
			return err
		}
		res := tx.Model(&Paper{}).Where("id = ? AND version = ?", paper.ID, expected).Updates(map[string]any{
			"title": paper.Title, "current_version_id": version.ID, "version": expected + 1, "updated_at": time.Now().UTC(),
		})
		if res.RowsAffected == 0 {
			return database.ErrVersionConflict
		}
		return res.Error
	})
}

func (r *Repo) PublishPaper(paper Paper, version PaperVersion, expected int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&PaperVersion{}).Where("id = ?", version.ID).Updates(map[string]any{"status": "published", "total_score_centi": version.TotalScoreCenti}).Error; err != nil {
			return err
		}
		res := tx.Model(&Paper{}).Where("id = ? AND version = ?", paper.ID, expected).Updates(map[string]any{
			"status": "published", "version": expected + 1, "updated_at": time.Now().UTC(),
		})
		if res.RowsAffected == 0 {
			return database.ErrVersionConflict
		}
		return res.Error
	})
}

func (r *Repo) SaveExam(row Exam) error { return r.db.Create(&row).Error }
func (r *Repo) UpdateExam(row Exam, expected int64) error {
	res := r.db.Model(&Exam{}).Where("id = ? AND version = ?", row.ID, expected).Updates(map[string]any{
		"title": row.Title, "paper_id": row.PaperID, "duration_sec": row.DurationSec, "max_attempts": row.MaxAttempts,
		"pass_score_centi": row.PassScoreCenti, "publish_mode": row.PublishMode, "start_at": row.StartAt, "end_at": row.EndAt,
		"status": row.Status, "paper_version_id": row.PaperVersionID, "version": expected + 1, "updated_at": time.Now().UTC(),
	})
	if res.RowsAffected == 0 {
		return database.ErrVersionConflict
	}
	return res.Error
}

func (r *Repo) Exam(id string) (Exam, error) {
	var row Exam
	err := r.db.First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Exam{}, apperr.NotFound("考试不存在")
	}
	return row, err
}

func (r *Repo) ListExams(owner, org, status string) ([]Exam, error) {
	q := r.db.Model(&Exam{})
	if owner != "" {
		q = q.Where("owner_id = ? OR status = ?", owner, "published")
	}
	if org != "" {
		q = q.Where("org_id = ?", org)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var rows []Exam
	err := q.Order("created_at desc").Find(&rows).Error
	return rows, err
}

func (r *Repo) FindAttemptByKey(examID, userID, key string) (Attempt, error) {
	var row Attempt
	err := r.db.Where("exam_id = ? AND user_id = ? AND start_key = ?", examID, userID, key).First(&row).Error
	return row, err
}

func (r *Repo) CountAttempts(examID, userID string) (int64, error) {
	var n int64
	err := r.db.Model(&Attempt{}).Where("exam_id = ? AND user_id = ?", examID, userID).Count(&n).Error
	return n, err
}

func (r *Repo) CreateAttempt(row Attempt) error { return r.db.Create(&row).Error }

func (r *Repo) Attempt(id string) (Attempt, error) {
	var row Attempt
	err := r.db.First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Attempt{}, apperr.NotFound("答卷不存在")
	}
	return row, err
}

func (r *Repo) Answers(attemptID string) ([]Answer, error) {
	var rows []Answer
	err := r.db.Where("attempt_id = ?", attemptID).Find(&rows).Error
	return rows, err
}

func (r *Repo) SaveAnswer(row Answer, expected int64) (Answer, error) {
	var current Answer
	err := r.db.Where("attempt_id = ? AND question_snapshot_id = ?", row.AttemptID, row.QuestionSnapshotID).First(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if expected != 0 {
			return Answer{}, apperr.Conflict("ATTEMPT_VERSION_CONFLICT", "答卷已发生变化，请刷新后重试")
		}
		row.ID = xid.New()
		row.Version = 1
		row.SavedAt = time.Now().UTC()
		if err := r.db.Create(&row).Error; err != nil {
			return Answer{}, apperr.Conflict("ATTEMPT_VERSION_CONFLICT", "答卷已发生变化，请刷新后重试")
		}
		return row, nil
	}
	if err != nil {
		return Answer{}, err
	}
	if current.Version != expected {
		return Answer{}, apperr.Conflict("ATTEMPT_VERSION_CONFLICT", "答卷已发生变化，请刷新后重试")
	}
	res := r.db.Model(&Answer{}).Where("id = ? AND version = ?", current.ID, expected).Updates(map[string]any{
		"answer": row.Answer, "version": expected + 1, "saved_at": time.Now().UTC(),
	})
	if res.RowsAffected == 0 {
		return Answer{}, apperr.Conflict("ATTEMPT_VERSION_CONFLICT", "答卷已发生变化，请刷新后重试")
	}
	current.Answer = row.Answer
	current.Version = expected + 1
	current.SavedAt = time.Now().UTC()
	return current, res.Error
}

type GradeLine struct {
	Question PaperQuestion
	Answer   string
	Score    int
	Correct  bool
}

func (r *Repo) SubmitObjective(attemptID string, lines []GradeLine, passLine int, publish bool, ev *eventbus.Event) (Attempt, error) {
	var saved Attempt
	err := r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		res := tx.Model(&Attempt{}).Where("id = ? AND status = ?", attemptID, "in_progress").Updates(map[string]any{
			"status": "submitted", "submitted_at": now, "updated_at": now,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errAlready
		}
		total := 0
		for _, line := range lines {
			total += line.Score
			score := line.Score
			correct := line.Correct
			tx.Model(&Answer{}).Where("attempt_id = ? AND question_snapshot_id = ?", attemptID, line.Question.ID).Updates(map[string]any{
				"score_centi": score, "correct": correct,
			})
		}
		passed := total >= passLine
		status := "ready"
		var scoreVersion int
		if publish {
			status = "published"
			scoreVersion = 1
		}
		if err := tx.Model(&Attempt{}).Where("id = ?", attemptID).Updates(map[string]any{
			"status": status, "score_centi": total, "passed": passed, "score_version": scoreVersion, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.First(&saved, "id = ?", attemptID).Error; err != nil {
			return err
		}
		if publish && ev != nil {
			return eventbus.Insert(tx, *ev)
		}
		return nil
	})
	return saved, err
}

func (r *Repo) MarkPending(attemptID, reason string) (Attempt, error) {
	var saved Attempt
	err := r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		res := tx.Model(&Attempt{}).Where("id = ? AND status = ?", attemptID, "in_progress").Updates(map[string]any{
			"status": "pending_review", "submitted_at": now, "updated_at": now,
		})
		if res.RowsAffected == 0 {
			return errAlready
		}
		if err := tx.Create(&GradingResult{ID: xid.New(), AttemptID: attemptID, Status: "exception", Reason: reason, CreatedAt: now}).Error; err != nil {
			return err
		}
		return tx.First(&saved, "id = ?", attemptID).Error
	})
	return saved, err
}

var errAlready = errors.New("already submitted")

func IsAlready(err error) bool { return errors.Is(err, errAlready) }

func (r *Repo) PublishResult(attempt Attempt, actor string, ev eventbus.Event) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Attempt{}).Where("id = ? AND status = ?", attempt.ID, "ready").Updates(map[string]any{
			"status": "published", "score_version": 1, "updated_at": time.Now().UTC(),
		})
		if res.RowsAffected == 0 {
			return apperr.Conflict("ATTEMPT_STATE", "答卷状态不允许发布")
		}
		return eventbus.Insert(tx, ev)
	})
}

func (r *Repo) DueAttempts(now time.Time) ([]Attempt, error) {
	var rows []Attempt
	err := r.db.Where("status = ? AND deadline_at <= ?", "in_progress", now).Find(&rows).Error
	return rows, err
}

func (r *Repo) QuestionMap(versionID string) (map[string]PaperQuestion, error) {
	rows, err := r.Questions(versionID)
	if err != nil {
		return nil, err
	}
	out := map[string]PaperQuestion{}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

func SnapshotOf(raw string) (grading.Snapshot, map[string]any, error) {
	var body struct {
		Type       string          `json:"type"`
		Stem       string          `json:"stem"`
		Answer     json.RawMessage `json:"answer"`
		ScoreCenti int             `json:"scoreCenti"`
		Options    json.RawMessage `json:"options"`
		Explanation string         `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return grading.Snapshot{}, nil, err
	}
	return grading.Snapshot{Type: body.Type, ScoreCenti: body.ScoreCenti, Answer: body.Answer, Normalize: true}, map[string]any{
		"type": body.Type, "stem": body.Stem, "options": body.Options, "explanation": body.Explanation, "answer": body.Answer,
	}, nil
}
