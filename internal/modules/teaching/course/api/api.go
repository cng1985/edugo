package api

import "context"

const (
	EventPublished = "CourseVersionPublished.v1"
	EventWithdrawn = "CourseContentWithdrawn.v1"
)

type Lesson struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	LessonType  string `json:"lessonType"`
	Required    bool   `json:"required"`
	DurationSec int    `json:"durationSec"`
	ContentText string `json:"contentText,omitempty"`
	ResourceID  string `json:"resourceId,omitempty"`
	SortOrder       int    `json:"sortOrder"`
	ChapterID       string `json:"chapterId"`
	CourseVersionID string `json:"courseVersionId,omitempty"`
}

type Chapter struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	SortOrder int      `json:"sortOrder"`
	Lessons   []Lesson `json:"lessons"`
}

type Course struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	Summary            string    `json:"summary"`
	Status             string    `json:"status"`
	Version            int64     `json:"version"`
	OwnerID            string    `json:"ownerId"`
	OrgID              string    `json:"orgId"`
	DraftVersionID     string    `json:"draftVersionId,omitempty"`
	PublishedVersionID string    `json:"publishedVersionId,omitempty"`
	VersionNo          int       `json:"versionNo"`
	Chapters           []Chapter `json:"chapters,omitempty"`
}

type CreateCourseRequest struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

type UpdateCourseRequest struct {
	CourseID        string  `json:"courseId"`
	ExpectedVersion int64   `json:"expectedVersion"`
	Title           *string `json:"title"`
	Summary         *string `json:"summary"`
}

type CourseIDRequest struct {
	CourseID        string `json:"courseId"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

type ListCourseRequest struct {
	Page   int    `json:"page"`
	Size   int    `json:"size"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

type ChapterWrite struct {
	CourseID        string `json:"courseId"`
	ChapterID       string `json:"chapterId"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Title           string `json:"title"`
	SortOrder       int    `json:"sortOrder"`
}

type LessonWrite struct {
	CourseID        string `json:"courseId"`
	ChapterID       string `json:"chapterId"`
	LessonID        string `json:"lessonId"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Title           string `json:"title"`
	LessonType      string `json:"lessonType"`
	Required        *bool  `json:"required"`
	DurationSec     int    `json:"durationSec"`
	ContentText     string `json:"contentText"`
	ResourceID      string `json:"resourceId"`
	SortOrder       int    `json:"sortOrder"`
}

type SummaryRequest struct {
	CourseID        string `json:"courseId"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

type Reader interface {
	AssertJoinable(ctx context.Context, courseID string) (Course, error)
	GetLesson(ctx context.Context, lessonID string) (Lesson, string, error)
	ContentReadable(ctx context.Context, lessonID string) (Lesson, error)
}
