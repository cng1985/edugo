package api

import (
	"context"
	"time"
)

const (
	EventLessonCompleted = "LessonCompleted.v1"
	EventCourseCompleted = "CourseCompleted.v1"
	EventStudyActivity   = "StudyActivityRecorded.v1"
)

type Enrollment struct {
	ID              string    `json:"id"`
	CourseID        string    `json:"courseId"`
	CourseVersionID string    `json:"courseVersionId"`
	Status          string    `json:"status"`
	JoinedAt        time.Time `json:"joinedAt"`
}

type Session struct {
	ID           string    `json:"id"`
	LessonID     string    `json:"lessonId"`
	EnrollmentID string    `json:"enrollmentId"`
	StartedAt    time.Time `json:"startedAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type RecordResult struct {
	SessionID     string   `json:"sessionId"`
	Sequence      int      `json:"sequence"`
	EffectiveSec  int      `json:"effectiveSec"`
	Duplicate     bool     `json:"duplicate"`
	LessonDone    bool     `json:"lessonDone"`
	CourseRate    *float64 `json:"courseRate"`
	RequiredDone  int      `json:"requiredDone"`
	RequiredTotal int      `json:"requiredTotal"`
	ServerTime    string   `json:"serverTime"`
}

type Progress struct {
	CourseID      string   `json:"courseId"`
	EnrollmentID  string   `json:"enrollmentId"`
	RequiredDone  int      `json:"requiredDone"`
	RequiredTotal int      `json:"requiredTotal"`
	ElectiveDone  int      `json:"electiveDone"`
	ElectiveTotal int      `json:"electiveTotal"`
	Rate          *float64 `json:"rate"`
	Completed     bool     `json:"completed"`
}

type Reader interface {
	IsEnrolled(ctx context.Context, userID, courseID string) (bool, string, error)
	Completion(ctx context.Context, userID, courseID string) (bool, string, error)
}
