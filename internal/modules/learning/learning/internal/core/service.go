package core

import (
	"context"
	"time"

	courseapi "github.com/cng1985/edugo/internal/modules/teaching/course/api"
	"github.com/cng1985/edugo/internal/modules/learning/learning/api"
	"github.com/cng1985/edugo/internal/modules/learning/learning/internal/store"
	"github.com/cng1985/edugo/internal/platform/eventbus"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/xid"
)

type Service struct {
	repo    *store.Repo
	courses courseapi.Reader
}

func New(repo *store.Repo, courses courseapi.Reader) *Service {
	return &Service{repo: repo, courses: courses}
}

func (s *Service) Join(ctx context.Context, courseID string) (api.Enrollment, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Enrollment{}, err
	}
	if !p.CanLearn() {
		return api.Enrollment{}, apperr.Forbidden("当前身份不能加入课程")
	}
	course, err := s.courses.AssertJoinable(ctx, courseID)
	if err != nil {
		return api.Enrollment{}, err
	}
	if _, err := s.repo.ActiveEnrollment(p.UserID, courseID); err == nil {
		return api.Enrollment{}, apperr.Conflict("ENROLLMENT_EXISTS", "已经加入该课程")
	}
	flag := p.UserID + ":" + courseID
	row := store.Enrollment{
		ID: xid.New(), UserID: p.UserID, CourseID: courseID, CourseVersionID: course.PublishedVersionID,
		Status: "active", ActiveFlag: &flag, JoinedAt: time.Now().UTC(), CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.CreateEnrollment(row); err != nil {
		return api.Enrollment{}, apperr.Conflict("ENROLLMENT_EXISTS", "已经加入该课程")
	}
	required, elective := 0, 0
	for _, ch := range course.Chapters {
		for _, lesson := range ch.Lessons {
			if lesson.Required {
				required++
			} else {
				elective++
			}
		}
	}
	if err := s.repo.InitProgress(row, required, elective); err != nil {
		return api.Enrollment{}, err
	}
	return toEnroll(row), nil
}

func (s *Service) List(ctx context.Context) ([]api.Enrollment, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListEnrollments(p.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]api.Enrollment, 0, len(rows))
	for _, row := range rows {
		out = append(out, toEnroll(row))
	}
	return out, nil
}

func (s *Service) Withdraw(ctx context.Context, courseID string) error {
	p, err := principal.Must(ctx)
	if err != nil {
		return err
	}
	row, err := s.repo.ActiveEnrollment(p.UserID, courseID)
	if err != nil {
		return err
	}
	row.Status = "withdrawn"
	row.ActiveFlag = nil
	return s.repo.SaveEnrollment(row)
}

func (s *Service) StartSession(ctx context.Context, lessonID string) (api.Session, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Session{}, err
	}
	if !p.CanLearn() {
		return api.Session{}, apperr.Forbidden("当前身份不能提交学习记录")
	}
	lesson, err := s.courses.ContentReadable(ctx, lessonID)
	if err != nil {
		return api.Session{}, err
	}
	courseID, err := s.courseOfLesson(ctx, lessonID)
	if err != nil {
		return api.Session{}, err
	}
	enroll, err := s.repo.ActiveEnrollment(p.UserID, courseID)
	if err != nil {
		return api.Session{}, apperr.Invalid("未加入或课程已撤回")
	}
	if lesson.CourseVersionID != enroll.CourseVersionID {
		return api.Session{}, apperr.Invalid("课时不属于已加入的课程版本")
	}
	now := time.Now().UTC()
	row := store.Session{
		ID: xid.New(), UserID: p.UserID, EnrollmentID: enroll.ID, LessonID: lessonID, Status: "active",
		StartedAt: now, ExpiresAt: now.Add(30 * time.Minute),
	}
	if err := s.repo.CreateSession(row); err != nil {
		return api.Session{}, err
	}
	return api.Session{ID: row.ID, LessonID: lessonID, EnrollmentID: enroll.ID, StartedAt: now, ExpiresAt: row.ExpiresAt}, nil
}

type SubmitInput struct {
	SessionID  string
	Sequence   int
	RecordType string
	PositionMs int64
	Passed     bool
}

func (s *Service) Submit(ctx context.Context, in SubmitInput) (api.RecordResult, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.RecordResult{}, err
	}
	if in.Sequence < 1 {
		return api.RecordResult{}, apperr.Invalid("序号必须从 1 开始")
	}
	session, err := s.repo.Session(in.SessionID)
	if err != nil {
		return api.RecordResult{}, err
	}
	if session.UserID != p.UserID {
		return api.RecordResult{}, apperr.NotFound("学习会话不存在")
	}
	lesson, err := s.courses.ContentReadable(ctx, session.LessonID)
	if err != nil {
		return api.RecordResult{}, err
	}
	courseID, err := s.courseOfLesson(ctx, session.LessonID)
	if err != nil {
		return api.RecordResult{}, err
	}
	enroll, err := s.repo.ActiveEnrollment(p.UserID, courseID)
	if err != nil || enroll.ID != session.EnrollmentID {
		return api.RecordResult{}, apperr.Invalid("未加入或课程已撤回")
	}
	now := time.Now().UTC()
	out, err := s.repo.ApplyRecord(store.HeartbeatInput{
		SessionID: in.SessionID, Sequence: in.Sequence, RecordType: in.RecordType, PositionMs: in.PositionMs,
		Passed: in.Passed, Required: lesson.Required, DurationMs: int64(lesson.DurationSec) * 1000, LessonType: lesson.LessonType,
		Now: now, UserID: p.UserID, CourseID: courseID, VersionID: lesson.CourseVersionID, Policy: "policy-v1",
	}, func(result store.HeartbeatOutput) []eventbus.Event {
		var events []eventbus.Event
		if result.LessonBecameDone {
			ev, e := eventbus.NewEvent(ctx, api.EventLessonCompleted, session.LessonID, 1, map[string]any{
				"userId": p.UserID, "enrollmentId": enroll.ID, "lessonId": session.LessonID,
				"courseVersionId": lesson.CourseVersionID, "policyVersion": "policy-v1", "completedAt": now,
			})
			if e == nil {
				events = append(events, ev)
			}
		}
		if result.CourseDone {
			ev, e := eventbus.NewEvent(ctx, api.EventCourseCompleted, courseID, 1, map[string]any{
				"userId": p.UserID, "courseId": courseID, "courseVersionId": lesson.CourseVersionID,
				"evidenceId": result.EvidenceID, "completedAt": now,
			})
			if e == nil {
				events = append(events, ev)
			}
		}
		return events
	})
	if err != nil {
		return api.RecordResult{}, err
	}
	return api.RecordResult{
		SessionID: in.SessionID, Sequence: in.Sequence, EffectiveSec: out.EffectiveSec, Duplicate: out.Duplicate,
		LessonDone: out.LessonDone, CourseRate: out.Rate, RequiredDone: out.RequiredDone, RequiredTotal: out.RequiredTotal,
		ServerTime: now.Format(time.RFC3339),
	}, nil
}

func (s *Service) Progress(ctx context.Context, courseID string) (api.Progress, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Progress{}, err
	}
	row, err := s.repo.Progress(p.UserID, courseID)
	if err != nil {
		return api.Progress{}, apperr.NotFound("没有学习进度")
	}
	var rate *float64
	if row.RateValid && row.RequiredTotal > 0 {
		v := float64(row.RateBasis) / 100
		rate = &v
	}
	return api.Progress{
		CourseID: courseID, EnrollmentID: row.EnrollmentID, RequiredDone: row.RequiredDone, RequiredTotal: row.RequiredTotal,
		ElectiveDone: row.ElectiveDone, ElectiveTotal: row.ElectiveTotal, Rate: rate, Completed: row.Completed,
	}, nil
}

func (s *Service) IsEnrolled(ctx context.Context, userID, courseID string) (bool, string, error) {
	row, err := s.repo.ActiveEnrollment(userID, courseID)
	if err != nil {
		return false, "", nil
	}
	return true, row.CourseVersionID, nil
}

func (s *Service) Completion(ctx context.Context, userID, courseID string) (bool, string, error) {
	row, err := s.repo.Progress(userID, courseID)
	if err != nil {
		return false, "", nil
	}
	return row.Completed, row.EvidenceID, nil
}

func (s *Service) courseOfLesson(ctx context.Context, lessonID string) (string, error) {
	_, courseID, err := s.courses.GetLesson(ctx, lessonID)
	return courseID, err
}

func toEnroll(row store.Enrollment) api.Enrollment {
	return api.Enrollment{ID: row.ID, CourseID: row.CourseID, CourseVersionID: row.CourseVersionID, Status: row.Status, JoinedAt: row.JoinedAt}
}
