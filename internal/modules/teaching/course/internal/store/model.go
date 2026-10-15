package store

import "time"

type Course struct {
	ID                 string `gorm:"primaryKey;size:36"`
	OwnerID            string `gorm:"size:36;index;not null"`
	OrgID              string `gorm:"size:36;index;not null"`
	Title              string `gorm:"size:200;not null"`
	Summary            string `gorm:"type:text"`
	Status             string `gorm:"size:32;index;not null"`
	DraftVersionID     string `gorm:"size:36"`
	PublishedVersionID string `gorm:"size:36"`
	Version            int64  `gorm:"not null;default:1"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (Course) TableName() string { return "courses" }

type Version struct {
	ID             string `gorm:"primaryKey;size:36"`
	CourseID       string `gorm:"size:36;not null;uniqueIndex:uk_course_ver"`
	VersionNo      int    `gorm:"not null;uniqueIndex:uk_course_ver"`
	Status         string `gorm:"size:32;index;not null"`
	Title          string `gorm:"size:200;not null"`
	Summary        string `gorm:"type:text"`
	SummaryDraft   string `gorm:"type:text"`
	PolicyVersion  string `gorm:"size:32;not null"`
	PublishedAt    *time.Time
	WithdrawReason string `gorm:"size:512"`
	CreatedAt      time.Time
}

func (Version) TableName() string { return "course_versions" }

type Chapter struct {
	ID              string `gorm:"primaryKey;size:36"`
	CourseVersionID string `gorm:"size:36;index;not null"`
	Title           string `gorm:"size:200;not null"`
	SortOrder       int
}

func (Chapter) TableName() string { return "chapters" }

type Lesson struct {
	ID              string `gorm:"primaryKey;size:36"`
	CourseVersionID string `gorm:"size:36;index;not null"`
	ChapterID       string `gorm:"size:36;index;not null"`
	Title           string `gorm:"size:200;not null"`
	LessonType      string `gorm:"size:32;not null"`
	Required        bool
	DurationSec     int
	ContentText     string `gorm:"type:text"`
	ResourceID      string `gorm:"size:36"`
	SortOrder       int
}

func (Lesson) TableName() string { return "lessons" }

type LessonResource struct {
	ID         string `gorm:"primaryKey;size:36"`
	LessonID   string `gorm:"size:36;index;not null"`
	ResourceID string `gorm:"size:36;index;not null"`
	Kind       string `gorm:"size:32"`
}

func (LessonResource) TableName() string { return "lesson_resources" }

func Models() []any {
	return []any{&Course{}, &Version{}, &Chapter{}, &Lesson{}, &LessonResource{}}
}
