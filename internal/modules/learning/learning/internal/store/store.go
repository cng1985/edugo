package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/cng1985/edugo/internal/modules/learning/learning/internal/progress"
	"github.com/cng1985/edugo/internal/platform/eventbus"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/xid"
	"gorm.io/gorm"
)

type Enrollment struct {
	ID              string `gorm:"primaryKey;size:36"`
	UserID          string `gorm:"size:36;index;not null"`
	CourseID        string `gorm:"size:36;index;not null"`
	CourseVersionID string `gorm:"size:36;not null"`
	Status          string `gorm:"size:32;index;not null"`
	ActiveFlag      *string `gorm:"size:80;uniqueIndex"`
	JoinedAt        time.Time
	CreatedAt       time.Time
}

func (Enrollment) TableName() string { return "enrollments" }

type Session struct {
	ID             string `gorm:"primaryKey;size:36"`
	UserID         string `gorm:"size:36;index;not null"`
	EnrollmentID   string `gorm:"size:36;index;not null"`
	LessonID       string `gorm:"size:36;index;not null"`
	Status         string `gorm:"size:32;not null"`
	StartedAt      time.Time
	LastHeartbeat  time.Time
	LastPositionMs int64
	EffectiveSec   int
	ExpiresAt      time.Time
}

func (Session) TableName() string { return "study_sessions" }

type Record struct {
	ID               string `gorm:"primaryKey;size:36"`
	SessionID        string `gorm:"size:36;not null;uniqueIndex:uk_session_seq"`
	Sequence         int    `gorm:"not null;uniqueIndex:uk_session_seq"`
	RecordType       string `gorm:"size:32;not null"`
	PositionMs       int64
	EffectiveSec     int
	ServerReceivedAt time.Time
	PayloadHash      string `gorm:"size:64"`
}

func (Record) TableName() string { return "study_records" }

type LessonProgress struct {
	ID            string `gorm:"primaryKey;size:36"`
	EnrollmentID  string `gorm:"size:36;not null;uniqueIndex:uk_enroll_lesson"`
	LessonID      string `gorm:"size:36;not null;uniqueIndex:uk_enroll_lesson"`
	Completed     bool
	CoveredJSON   string `gorm:"type:text"`
	CoveredMs     int64
	EffectiveSec  int
	PolicyVersion string `gorm:"size:32"`
	CompletedAt   *time.Time
	Version       int64 `gorm:"not null;default:1"`
}

func (LessonProgress) TableName() string { return "lesson_progress" }

type CourseProgress struct {
	ID            string `gorm:"primaryKey;size:36"`
	EnrollmentID  string `gorm:"size:36;uniqueIndex;not null"`
	UserID        string `gorm:"size:36;index;not null"`
	CourseID      string `gorm:"size:36;index;not null"`
	RequiredTotal int
	RequiredDone  int
	ElectiveTotal int
	ElectiveDone  int
	RateBasis     int
	RateValid     bool
	Completed     bool
	EvidenceID    string `gorm:"size:36"`
	UpdatedAt     time.Time
}

func (CourseProgress) TableName() string { return "course_progress" }

func Models() []any {
	return []any{&Enrollment{}, &Session{}, &Record{}, &LessonProgress{}, &CourseProgress{}}
}

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) CreateEnrollment(row Enrollment) error { return r.db.Create(&row).Error }
func (r *Repo) SaveEnrollment(row Enrollment) error   { return r.db.Save(&row).Error }

func (r *Repo) ActiveEnrollment(userID, courseID string) (Enrollment, error) {
	var row Enrollment
	err := r.db.Where("user_id = ? AND course_id = ? AND status = ?", userID, courseID, "active").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Enrollment{}, apperr.NotFound("尚未加入课程")
	}
	return row, err
}

func (r *Repo) ListEnrollments(userID string) ([]Enrollment, error) {
	var rows []Enrollment
	err := r.db.Where("user_id = ?", userID).Order("joined_at desc").Find(&rows).Error
	return rows, err
}

func (r *Repo) CreateSession(row Session) error { return r.db.Create(&row).Error }

func (r *Repo) Session(id string) (Session, error) {
	var row Session
	err := r.db.First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Session{}, apperr.NotFound("学习会话不存在")
	}
	return row, err
}

type HeartbeatInput struct {
	SessionID    string
	Sequence     int
	RecordType   string
	PositionMs   int64
	Passed       bool
	Required     bool
	DurationMs   int64
	LessonType   string
	Now          time.Time
	UserID       string
	CourseID     string
	VersionID    string
	Policy       string
}

type HeartbeatOutput struct {
	Duplicate     bool
	EffectiveSec  int
	LessonDone       bool
	LessonBecameDone bool
	RequiredDone  int
	RequiredTotal int
	ElectiveDone  int
	ElectiveTotal int
	Rate          *float64
	CourseDone    bool
	EvidenceID    string
	Events        []eventbus.Event
}

func (r *Repo) ApplyRecord(in HeartbeatInput, buildEvents func(out HeartbeatOutput) []eventbus.Event) (HeartbeatOutput, error) {
	var out HeartbeatOutput
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var existing Record
		err := tx.Where("session_id = ? AND sequence = ?", in.SessionID, in.Sequence).First(&existing).Error
		if err == nil {
			out.Duplicate = true
			out.EffectiveSec = existing.EffectiveSec
			cp, _ := courseOf(tx, in.UserID, in.CourseID)
			out.RequiredDone = cp.RequiredDone
			out.RequiredTotal = cp.RequiredTotal
			out.Rate = rateOf(cp)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var session Session
		if err := tx.First(&session, "id = ?", in.SessionID).Error; err != nil {
			return err
		}
		if session.UserID != in.UserID || session.Status != "active" || in.Now.After(session.ExpiresAt) {
			return apperr.Invalid("会话不可用或已过期")
		}
		delta := 0
		if in.RecordType == "heartbeat" && !session.LastHeartbeat.IsZero() {
			gap := in.Now.Sub(session.LastHeartbeat)
			if gap > 0 && gap <= 45*time.Second {
				delta = int(gap.Seconds())
				if delta < 1 {
					delta = 1
				}
			}
		}
		lp := LessonProgress{}
		err = tx.Where("enrollment_id = ? AND lesson_id = ?", session.EnrollmentID, session.LessonID).First(&lp).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			lp = LessonProgress{ID: xid.New(), EnrollmentID: session.EnrollmentID, LessonID: session.LessonID, PolicyVersion: in.Policy, Version: 1}
			err = nil
		}
		if err != nil {
			return err
		}
		wasDone := lp.Completed
		var covered []progress.Interval
		_ = json.Unmarshal([]byte(lp.CoveredJSON), &covered)
		if in.RecordType == "heartbeat" && in.LessonType == "video" && in.PositionMs >= session.LastPositionMs {
			advance := in.PositionMs - session.LastPositionMs
			allowed := int64(delta)*1000 + 2000
			if session.LastHeartbeat.IsZero() {
				allowed = 2000
			}
			if advance <= allowed {
				covered = progress.Merge(covered, session.LastPositionMs, in.PositionMs)
			}
		}
		raw, _ := json.Marshal(covered)
		lp.CoveredJSON = string(raw)
		lp.CoveredMs = progress.Coverage(covered)
		lp.EffectiveSec += delta
		if in.LessonType == "video" && progress.VideoComplete(lp.CoveredMs, in.DurationMs) {
			lp.Completed = true
		}
		if in.RecordType == "confirm" && in.LessonType == "document" {
			lp.Completed = true
		}
		if in.RecordType == "exercise" && in.LessonType == "exercise" && in.Passed {
			lp.Completed = true
		}
		if lp.Completed && lp.CompletedAt == nil {
			now := in.Now
			lp.CompletedAt = &now
		}
		if lp.Version == 0 {
			lp.Version = 1
		}
		if err := tx.Save(&lp).Error; err != nil {
			return err
		}
		session.LastHeartbeat = in.Now
		session.LastPositionMs = in.PositionMs
		session.EffectiveSec += delta
		session.ExpiresAt = in.Now.Add(30 * time.Minute)
		if err := tx.Save(&session).Error; err != nil {
			return err
		}
		rec := Record{ID: xid.New(), SessionID: in.SessionID, Sequence: in.Sequence, RecordType: in.RecordType, PositionMs: in.PositionMs, EffectiveSec: delta, ServerReceivedAt: in.Now}
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		out.EffectiveSec = delta
		out.LessonDone = lp.Completed
		out.LessonBecameDone = !wasDone && lp.Completed
		if err := r.recalc(tx, session.EnrollmentID, in, &out, wasDone, lp.Completed); err != nil {
			return err
		}
		out.Events = buildEvents(out)
		if delta > 0 || out.LessonBecameDone {
			ev, err := eventbus.NewEvent(context.Background(), "StudyActivityRecorded.v1", rec.ID, 1, map[string]any{
				"userId": in.UserID, "lessonId": session.LessonID, "effectiveSec": delta, "recordId": rec.ID,
			})
			if err != nil {
				return err
			}
			out.Events = append(out.Events, ev)
		}
		return eventbus.Insert(tx, out.Events...)
	})
	return out, err
}

func (r *Repo) recalc(tx *gorm.DB, enrollmentID string, in HeartbeatInput, out *HeartbeatOutput, wasDone, nowDone bool) error {
	var enrollment Enrollment
	if err := tx.First(&enrollment, "id = ?", enrollmentID).Error; err != nil {
		return err
	}
	cp, err := courseOf(tx, enrollment.UserID, enrollment.CourseID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cp = CourseProgress{ID: xid.New(), EnrollmentID: enrollment.ID, UserID: enrollment.UserID, CourseID: enrollment.CourseID}
	} else if err != nil {
		return err
	}
	out.RequiredTotal = cp.RequiredTotal
	out.RequiredDone = cp.RequiredDone
	out.ElectiveTotal = cp.ElectiveTotal
	out.ElectiveDone = cp.ElectiveDone
	if !wasDone && nowDone {
		if in.Required {
			cp.RequiredDone++
		} else {
			cp.ElectiveDone++
		}
	}
	if cp.RequiredTotal > 0 {
		rate := int(float64(cp.RequiredDone) / float64(cp.RequiredTotal) * 10000)
		cp.RateBasis = rate
		cp.RateValid = true
	}
	if cp.RequiredTotal > 0 && cp.RequiredDone >= cp.RequiredTotal && !cp.Completed {
		cp.Completed = true
		cp.EvidenceID = xid.New()
		out.CourseDone = true
		out.EvidenceID = cp.EvidenceID
	}
	cp.UpdatedAt = in.Now
	if err := tx.Save(&cp).Error; err != nil {
		return err
	}
	out.RequiredDone = cp.RequiredDone
	out.RequiredTotal = cp.RequiredTotal
	out.ElectiveDone = cp.ElectiveDone
	out.ElectiveTotal = cp.ElectiveTotal
	out.Rate = rateOf(cp)
	return nil
}

func (r *Repo) InitProgress(enrollment Enrollment, required, elective int) error {
	cp := CourseProgress{
		ID: xid.New(), EnrollmentID: enrollment.ID, UserID: enrollment.UserID, CourseID: enrollment.CourseID,
		RequiredTotal: required, ElectiveTotal: elective, UpdatedAt: time.Now().UTC(),
	}
	if required > 0 {
		cp.RateValid = true
	}
	return r.db.Create(&cp).Error
}

func (r *Repo) Progress(userID, courseID string) (CourseProgress, error) {
	return courseOf(r.db, userID, courseID)
}

func courseOf(tx *gorm.DB, userID, courseID string) (CourseProgress, error) {
	var cp CourseProgress
	err := tx.Where("user_id = ? AND course_id = ?", userID, courseID).First(&cp).Error
	return cp, err
}

func rateOf(cp CourseProgress) *float64 {
	if !cp.RateValid || cp.RequiredTotal == 0 {
		return nil
	}
	v := float64(cp.RateBasis) / 100
	return &v
}
