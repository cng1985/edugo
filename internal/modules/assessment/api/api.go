package api

import (
	"context"
	"encoding/json"
	"time"
)

const (
	EventPublished = "AssessmentResultPublished.v1"
	EventRevised   = "AssessmentResultRevised.v1"
	EventInvalid   = "AssessmentResultInvalidated.v1"
)

type WrongItem struct {
	QuestionVersionID string `json:"questionVersionId"`
	Stem              string `json:"stem"`
}

type Facts struct {
	UserID       string      `json:"userId"`
	AttemptID    string      `json:"attemptId"`
	ExamID       string      `json:"examId"`
	Published    bool        `json:"published"`
	Voided       bool        `json:"voided"`
	Passed       bool        `json:"passed"`
	Score        float64     `json:"score"`
	ScoreVersion int         `json:"scoreVersion"`
	Wrong        []WrongItem `json:"wrong"`
}

type Reader interface {
	Facts(ctx context.Context, userID, attemptID string) (Facts, error)
}

type Paper struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Status        string     `json:"status"`
	Version       int64      `json:"version"`
	Total         float64    `json:"total"`
	Questions     []Question `json:"questions,omitempty"`
	PaperVersionID string    `json:"paperVersionId,omitempty"`
}

type Question struct {
	ID      string          `json:"id"`
	Stem    string          `json:"stem"`
	Type    string          `json:"type"`
	Options json.RawMessage `json:"options,omitempty"`
	Score   float64         `json:"score"`
	Answer  json.RawMessage `json:"answer,omitempty"`
}

type Exam struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Status        string     `json:"status"`
	PaperID       string     `json:"paperId"`
	DurationSec   int        `json:"durationSec"`
	MaxAttempts   int        `json:"maxAttempts"`
	PassScore     float64    `json:"passScore"`
	PublishMode   string     `json:"publishMode"`
	Version       int64      `json:"version"`
	StartAt       *time.Time `json:"startAt,omitempty"`
	EndAt         *time.Time `json:"endAt,omitempty"`
}

type Attempt struct {
	ID         string     `json:"id"`
	ExamID     string     `json:"examId"`
	Status     string     `json:"status"`
	AttemptNo  int        `json:"attemptNo"`
	DeadlineAt time.Time  `json:"deadlineAt"`
	Version    int64      `json:"version"`
	Score      *float64   `json:"score,omitempty"`
	Passed     *bool      `json:"passed,omitempty"`
	Questions  []Question `json:"questions,omitempty"`
	Answers    []Answer   `json:"answers,omitempty"`
}

type Answer struct {
	QuestionID      string          `json:"questionId"`
	Answer          json.RawMessage `json:"answer,omitempty"`
	Version         int64           `json:"version"`
	SavedAt         time.Time       `json:"savedAt"`
	Score           *float64        `json:"score,omitempty"`
}
