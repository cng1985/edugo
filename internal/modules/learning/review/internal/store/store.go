package store

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/eventbus"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"gorm.io/gorm"
)

type Card struct {
	ID              string `gorm:"primaryKey;size:36"`
	UserID          string `gorm:"size:36;not null;uniqueIndex:uk_card"`
	ContentKey      string `gorm:"size:80;not null;uniqueIndex:uk_card"`
	QuestionVersion string `gorm:"size:36"`
	Front           string `gorm:"type:text"`
	Back            string `gorm:"type:text"`
	EaseFactor      float64
	IntervalDays    int
	Repetitions     int
	SuccessStreak   int
	Status          string `gorm:"size:32;index"`
	NextReviewAt    time.Time `gorm:"index"`
	LastReviewAt    *time.Time
	Algorithm       string `gorm:"size:32"`
	SourceStatus    string `gorm:"size:32"`
	Version         int64 `gorm:"not null;default:1"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (Card) TableName() string { return "review_cards" }

type Log struct {
	ID           string `gorm:"primaryKey;size:36"`
	CardID       string `gorm:"size:36;not null;uniqueIndex:uk_review_sub"`
	SubmissionID string `gorm:"size:64;not null;uniqueIndex:uk_review_sub"`
	Quality      int
	BeforeState  string `gorm:"type:text"`
	AfterState   string `gorm:"type:text"`
	ReviewedAt   time.Time
}

func (Log) TableName() string { return "review_logs" }

func Models() []any { return []any{&Card{}, &Log{}} }

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) Create(card Card) error { return r.db.Create(&card).Error }

func (r *Repo) ByKey(userID, key string) (Card, error) {
	var row Card
	err := r.db.Where("user_id = ? AND content_key = ?", userID, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Card{}, apperr.NotFound("卡片不存在")
	}
	return row, err
}

func (r *Repo) Get(id, userID string) (Card, error) {
	var row Card
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Card{}, apperr.NotFound("卡片不存在")
	}
	return row, err
}

func (r *Repo) Due(userID string, now time.Time) ([]Card, error) {
	var rows []Card
	err := r.db.Where("user_id = ? AND next_review_at <= ? AND status NOT IN ?", userID, now, []string{"paused", "archived"}).Order("next_review_at asc").Find(&rows).Error
	return rows, err
}

func (r *Repo) Counts(userID string, now time.Time) (due, learning, review, mastered, paused int64, err error) {
	base := r.db.Model(&Card{}).Where("user_id = ?", userID)
	if err = base.Where("next_review_at <= ? AND status NOT IN ?", now, []string{"paused", "archived"}).Count(&due).Error; err != nil {
		return
	}
	r.db.Model(&Card{}).Where("user_id = ? AND status = ?", userID, "learning").Count(&learning)
	r.db.Model(&Card{}).Where("user_id = ? AND status = ?", userID, "review").Count(&review)
	r.db.Model(&Card{}).Where("user_id = ? AND status = ?", userID, "mastered").Count(&mastered)
	r.db.Model(&Card{}).Where("user_id = ? AND status = ?", userID, "paused").Count(&paused)
	return
}

func (r *Repo) FindLog(cardID, submissionID string) (Log, error) {
	var row Log
	err := r.db.Where("card_id = ? AND submission_id = ?", cardID, submissionID).First(&row).Error
	return row, err
}

func (r *Repo) Apply(card Card, expected int64, log Log, ev eventbus.Event) (Card, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Card{}).Where("id = ? AND version = ?", card.ID, expected).Updates(map[string]any{
			"ease_factor": card.EaseFactor, "interval_days": card.IntervalDays, "repetitions": card.Repetitions,
			"success_streak": card.SuccessStreak, "status": card.Status, "next_review_at": card.NextReviewAt,
			"last_review_at": card.LastReviewAt, "version": expected + 1, "updated_at": time.Now().UTC(),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return database.ErrVersionConflict
		}
		if err := tx.Create(&log).Error; err != nil {
			return err
		}
		return eventbus.Insert(tx, ev)
	})
	if err != nil {
		return Card{}, err
	}
	card.Version = expected + 1
	return card, nil
}

func (r *Repo) SetStatus(id, userID, status string, expected int64) error {
	res := r.db.Model(&Card{}).Where("id = ? AND user_id = ? AND version = ?", id, userID, expected).Updates(map[string]any{
		"status": status, "version": expected + 1, "updated_at": time.Now().UTC(),
	})
	if res.RowsAffected == 0 {
		return database.ErrVersionConflict
	}
	return res.Error
}

func StateJSON(card Card) string {
	b, _ := json.Marshal(map[string]any{"ef": card.EaseFactor, "reps": card.Repetitions, "interval": card.IntervalDays, "status": card.Status})
	return string(b)
}
