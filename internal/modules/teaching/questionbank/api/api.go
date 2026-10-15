package api

import (
	"context"
	"encoding/json"
)

type Snapshot struct {
	QuestionVersionID string          `json:"questionVersionId"`
	Type              string          `json:"type"`
	Stem              string          `json:"stem"`
	Options           json.RawMessage `json:"options"`
	Answer            json.RawMessage `json:"answer"`
	Explanation       string          `json:"explanation"`
	MaxScoreCenti     int             `json:"maxScoreCenti"`
	Difficulty        string          `json:"difficulty"`
}

type Question struct {
	ID            string          `json:"id"`
	VersionID     string          `json:"versionId"`
	VersionNo     int             `json:"versionNo"`
	Type          string          `json:"type"`
	Stem          string          `json:"stem"`
	Options       json.RawMessage `json:"options,omitempty"`
	Answer        json.RawMessage `json:"answer,omitempty"`
	Explanation   string          `json:"explanation,omitempty"`
	Difficulty    string          `json:"difficulty"`
	MaxScoreCenti int             `json:"maxScoreCenti"`
	ReviewStatus  string          `json:"reviewStatus"`
	OwnerID       string          `json:"ownerId"`
	Version       int64           `json:"version"`
}

type WriteRequest struct {
	QuestionID      string          `json:"questionId"`
	ExpectedVersion int64           `json:"expectedVersion"`
	Type            string          `json:"type"`
	Stem            string          `json:"stem"`
	Options         json.RawMessage `json:"options"`
	Answer          json.RawMessage `json:"answer"`
	Explanation     string          `json:"explanation"`
	Difficulty      string          `json:"difficulty"`
	MaxScoreCenti   int             `json:"maxScoreCenti"`
	Tags            []string        `json:"tags"`
	KnowledgeIDs    []string        `json:"knowledgeIds"`
}

type IDRequest struct {
	QuestionID      string `json:"questionId"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

type ListRequest struct {
	Page   int    `json:"page"`
	Size   int    `json:"size"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Stem   string `json:"stem"`
}

type GenerateRequest struct {
	CourseID   string `json:"courseId"`
	LessonID   string `json:"lessonId"`
	Type       string `json:"type"`
	Difficulty string `json:"difficulty"`
	Count      int    `json:"count"`
}

type Generation struct {
	ID     string     `json:"id"`
	Status string     `json:"status"`
	Error  string     `json:"error,omitempty"`
	Items  []Question `json:"items,omitempty"`
}

type Reader interface {
	ApprovedSnapshot(ctx context.Context, versionID string) (Snapshot, error)
}
