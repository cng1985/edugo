package core

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cng1985/edugo/internal/modules/teaching/course/api"
	"github.com/cng1985/edugo/internal/modules/teaching/course/internal/store"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/eventbus"
	"github.com/cng1985/edugo/internal/platform/llm"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/pagination"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/xid"
	platcfg "github.com/cng1985/edugo/internal/platform/config"
)

type Files interface{ Exists(id string) bool }

type Service struct {
	repo  *store.Repo
	files Files
	llm   llm.Client
	cfg   *platcfg.Config
}

func New(repo *store.Repo, files Files, client llm.Client, cfg *platcfg.Config) *Service {
	return &Service{repo: repo, files: files, llm: client, cfg: cfg}
}

func (s *Service) CreateCourse(ctx context.Context, req api.CreateCourseRequest) (api.Course, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return api.Course{}, err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return api.Course{}, apperr.Invalid("课程标题不能为空")
	}
	now := time.Now().UTC()
	version := store.Version{ID: xid.New(), VersionNo: 1, Status: "draft", Title: title, Summary: req.Summary, PolicyVersion: "policy-v1", CreatedAt: now}
	course := store.Course{
		ID: xid.New(), OwnerID: p.UserID, OrgID: s.org(p), Title: title, Summary: req.Summary, Status: "draft", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	version.CourseID = course.ID
	if err := s.repo.CreateDraft(course, version); err != nil {
		return api.Course{}, err
	}
	course.DraftVersionID = version.ID
	return s.view(course, version, nil, nil, true), nil
}

func (s *Service) UpdateCourse(ctx context.Context, req api.UpdateCourseRequest) (api.Course, error) {
	course, version, err := s.editable(ctx, req.CourseID, req.ExpectedVersion)
	if err != nil {
		return api.Course{}, err
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			return api.Course{}, apperr.Invalid("课程标题不能为空")
		}
		version.Title = title
		course.Title = title
	}
	if req.Summary != nil {
		version.Summary = *req.Summary
		course.Summary = *req.Summary
	}
	if err := s.repo.SaveVersion(version); err != nil {
		return api.Course{}, err
	}
	if err := s.repo.SaveCourse(course, course.Version); err != nil {
		return api.Course{}, mapErr(err)
	}
	course.Version++
	return s.compose(course, version, true)
}

func (s *Service) Detail(ctx context.Context, id string) (api.Course, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Course{}, err
	}
	course, err := s.repo.Course(id)
	if err != nil {
		return api.Course{}, err
	}
	if s.cfg.Organization.Enabled && course.OrgID != p.OrgID {
		return api.Course{}, apperr.NotFound("课程不存在")
	}
	owner := s.canEdit(p, course)
	if course.Status == "withdrawn" && !owner {
		return api.Course{}, apperr.NotFound("课程不存在")
	}
	versionID := course.PublishedVersionID
	if owner {
		versionID = course.DraftVersionID
	}
	if versionID == "" {
		return api.Course{}, apperr.NotFound("课程不存在")
	}
	version, err := s.repo.Version(versionID)
	if err != nil {
		return api.Course{}, err
	}
	if !owner && version.Status == "withdrawn" {
		return api.Course{}, apperr.NotFound("课程不存在")
	}
	view, err := s.compose(course, version, owner)
	if err != nil {
		return api.Course{}, err
	}
	if !owner {
		for i := range view.Chapters {
			for j := range view.Chapters[i].Lessons {
				if course.Status == "withdrawn" {
					view.Chapters[i].Lessons[j].ContentText = ""
					view.Chapters[i].Lessons[j].ResourceID = ""
				}
			}
		}
	}
	return view, nil
}

func (s *Service) List(ctx context.Context, req api.ListCourseRequest) ([]api.Course, int64, int, int, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	page, size, err := pagination.Normalize(req.Page, req.Size, s.cfg.Query.DefaultPageSize, s.cfg.Query.MaxPageSize)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	org := ""
	if s.cfg.Organization.Enabled {
		org = p.OrgID
	}
	var rows []store.Course
	var total int64
	switch {
	case p.IsAdmin():
		rows, total, err = s.repo.List("", org, req.Status, req.Title, pagination.Offset(page, size), size, false)
	case p.HasRole("teacher"):
		rows, total, err = s.repo.List(p.UserID, org, req.Status, req.Title, pagination.Offset(page, size), size, true)
	default:
		status := req.Status
		if status == "" {
			status = "published"
		}
		if status != "published" {
			return []api.Course{}, 0, page, size, nil
		}
		rows, total, err = s.repo.List("", org, "published", req.Title, pagination.Offset(page, size), size, false)
	}
	if err != nil {
		return nil, 0, 0, 0, err
	}
	out := make([]api.Course, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.Course{ID: row.ID, Title: row.Title, Summary: row.Summary, Status: row.Status, Version: row.Version, OwnerID: row.OwnerID, OrgID: row.OrgID, PublishedVersionID: row.PublishedVersionID})
	}
	return out, total, page, size, nil
}

func (s *Service) Publish(ctx context.Context, req api.CourseIDRequest) (api.Course, error) {
	course, version, err := s.editable(ctx, req.CourseID, req.ExpectedVersion)
	if err != nil {
		return api.Course{}, err
	}
	if strings.TrimSpace(version.Title) == "" {
		return api.Course{}, apperr.Invalid("发布失败：缺少标题")
	}
	lessons, err := s.repo.LessonsOf(version.ID)
	if err != nil {
		return api.Course{}, err
	}
	if len(lessons) == 0 {
		return api.Course{}, apperr.Invalid("发布失败：至少需要一个课时")
	}
	required := 0
	for _, lesson := range lessons {
		if lesson.Required {
			required++
		}
		switch lesson.LessonType {
		case "video":
			if lesson.ResourceID == "" || !s.files.Exists(lesson.ResourceID) {
				return api.Course{}, apperr.Invalid("发布失败：视频课时缺少可访问资源")
			}
			if lesson.DurationSec <= 0 {
				return api.Course{}, apperr.Invalid("发布失败：视频课时需要时长")
			}
		case "document":
			if strings.TrimSpace(lesson.ContentText) == "" && (lesson.ResourceID == "" || !s.files.Exists(lesson.ResourceID)) {
				return api.Course{}, apperr.Invalid("发布失败：文档课时缺少内容")
			}
		case "exercise":
			if strings.TrimSpace(lesson.ContentText) == "" {
				return api.Course{}, apperr.Invalid("发布失败：练习课时缺少说明")
			}
		default:
			return api.Course{}, apperr.Invalid("发布失败：课时类型非法")
		}
	}
	if required == 0 {
		return api.Course{}, apperr.Invalid("发布失败：至少需要一个必修课时")
	}
	ev, err := eventbus.NewEvent(ctx, api.EventPublished, course.ID, course.Version+1, map[string]any{
		"courseId": course.ID, "courseVersionId": version.ID, "contentHash": version.ID,
	})
	if err != nil {
		return api.Course{}, err
	}
	if err := s.repo.Publish(course, version, course.Version, ev); err != nil {
		return api.Course{}, mapErr(err)
	}
	return s.Detail(ctx, course.ID)
}

func (s *Service) Archive(ctx context.Context, req api.CourseIDRequest) (api.Course, error) {
	return s.transition(ctx, req, "archived", "archived", false)
}

func (s *Service) Withdraw(ctx context.Context, req api.CourseIDRequest) (api.Course, error) {
	if strings.TrimSpace(req.Reason) == "" {
		return api.Course{}, apperr.Invalid("撤回必须填写原因")
	}
	return s.transition(ctx, req, "withdrawn", "withdrawn", true)
}

func (s *Service) transition(ctx context.Context, req api.CourseIDRequest, courseStatus, versionStatus string, evented bool) (api.Course, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return api.Course{}, err
	}
	course, err := s.repo.Course(req.CourseID)
	if err != nil {
		return api.Course{}, err
	}
	if !s.canEdit(p, course) {
		return api.Course{}, apperr.NotFound("课程不存在")
	}
	if course.Version != req.ExpectedVersion {
		return api.Course{}, apperr.Conflict("VERSION_CONFLICT", "课程已变化，请刷新后重试")
	}
	if course.PublishedVersionID == "" {
		return api.Course{}, apperr.Invalid("课程尚未发布")
	}
	var ev *eventbus.Event
	if evented {
		e, err := eventbus.NewEvent(ctx, api.EventWithdrawn, course.ID, course.Version+1, map[string]any{
			"courseId": course.ID, "courseVersionId": course.PublishedVersionID, "reason": req.Reason,
		})
		if err != nil {
			return api.Course{}, err
		}
		ev = &e
	}
	if err := s.repo.Transition(course, course.PublishedVersionID, courseStatus, versionStatus, req.Reason, course.Version, ev); err != nil {
		return api.Course{}, mapErr(err)
	}
	return s.Detail(ctx, course.ID)
}

func (s *Service) CreateChapter(ctx context.Context, req api.ChapterWrite) (api.Course, error) {
	course, version, err := s.editable(ctx, req.CourseID, req.ExpectedVersion)
	if err != nil {
		return api.Course{}, err
	}
	if strings.TrimSpace(req.Title) == "" {
		return api.Course{}, apperr.Invalid("章节标题不能为空")
	}
	if _, err := s.repo.AddChapter(store.Chapter{ID: xid.New(), CourseVersionID: version.ID, Title: req.Title, SortOrder: req.SortOrder}, course.ID, course.Version); err != nil {
		return api.Course{}, mapErr(err)
	}
	course.Version++
	return s.compose(course, version, true)
}

func (s *Service) UpdateChapter(ctx context.Context, req api.ChapterWrite) (api.Course, error) {
	course, version, err := s.editable(ctx, req.CourseID, req.ExpectedVersion)
	if err != nil {
		return api.Course{}, err
	}
	if err := s.repo.SaveChapter(store.Chapter{ID: req.ChapterID, Title: req.Title, SortOrder: req.SortOrder}, course.ID, course.Version); err != nil {
		return api.Course{}, mapErr(err)
	}
	course.Version++
	return s.compose(course, version, true)
}

func (s *Service) DeleteChapter(ctx context.Context, req api.ChapterWrite) (api.Course, error) {
	course, version, err := s.editable(ctx, req.CourseID, req.ExpectedVersion)
	if err != nil {
		return api.Course{}, err
	}
	if err := s.repo.DeleteChapter(req.ChapterID, course.ID, course.Version); err != nil {
		return api.Course{}, mapErr(err)
	}
	course.Version++
	return s.compose(course, version, true)
}

func (s *Service) CreateLesson(ctx context.Context, req api.LessonWrite) (api.Course, error) {
	course, version, err := s.editable(ctx, req.CourseID, req.ExpectedVersion)
	if err != nil {
		return api.Course{}, err
	}
	lesson, err := s.lessonFrom(req, version.ID)
	if err != nil {
		return api.Course{}, err
	}
	lesson.ID = xid.New()
	if _, err := s.repo.AddLesson(lesson, course.ID, course.Version); err != nil {
		return api.Course{}, mapErr(err)
	}
	course.Version++
	return s.compose(course, version, true)
}

func (s *Service) UpdateLesson(ctx context.Context, req api.LessonWrite) (api.Course, error) {
	course, version, err := s.editable(ctx, req.CourseID, req.ExpectedVersion)
	if err != nil {
		return api.Course{}, err
	}
	lesson, err := s.lessonFrom(req, version.ID)
	if err != nil {
		return api.Course{}, err
	}
	lesson.ID = req.LessonID
	if err := s.repo.SaveLesson(lesson, course.ID, course.Version); err != nil {
		return api.Course{}, mapErr(err)
	}
	course.Version++
	return s.compose(course, version, true)
}

func (s *Service) DeleteLesson(ctx context.Context, req api.LessonWrite) (api.Course, error) {
	course, version, err := s.editable(ctx, req.CourseID, req.ExpectedVersion)
	if err != nil {
		return api.Course{}, err
	}
	if err := s.repo.DeleteLesson(req.LessonID, course.ID, course.Version); err != nil {
		return api.Course{}, mapErr(err)
	}
	course.Version++
	return s.compose(course, version, true)
}

func (s *Service) GenerateSummary(ctx context.Context, req api.SummaryRequest) (api.Course, error) {
	course, version, err := s.editable(ctx, req.CourseID, req.ExpectedVersion)
	if err != nil {
		return api.Course{}, err
	}
	lessons, _ := s.repo.LessonsOf(version.ID)
	var b strings.Builder
	for _, lesson := range lessons {
		b.WriteString(lesson.Title)
		b.WriteString("\n")
		b.WriteString(lesson.ContentText)
		b.WriteString("\n")
	}
	res, err := s.llm.Complete(ctx, llm.ChatRequest{
		Module: "course", TaskID: course.ID, Profile: "summary", PromptVersion: "summary-v1", SchemaVersion: "v1",
		System: "你是课程编辑。只根据给定课时内容写一段摘要草稿，不要编造未提供的事实。",
		User:   b.String(),
	})
	if err != nil {
		return api.Course{}, err
	}
	version.SummaryDraft = res.Content
	if err := s.repo.SaveVersion(version); err != nil {
		return api.Course{}, err
	}
	view, err := s.compose(course, version, true)
	if err != nil {
		return api.Course{}, err
	}
	view.Summary = version.SummaryDraft
	return view, nil
}

func (s *Service) AssertJoinable(ctx context.Context, courseID string) (api.Course, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Course{}, err
	}
	course, err := s.repo.Course(courseID)
	if err != nil {
		return api.Course{}, err
	}
	if s.cfg.Organization.Enabled && course.OrgID != p.OrgID {
		return api.Course{}, apperr.NotFound("课程不存在")
	}
	if course.Status != "published" || course.PublishedVersionID == "" {
		return api.Course{}, apperr.Invalid("课程当前不可加入")
	}
	version, err := s.repo.Version(course.PublishedVersionID)
	if err != nil {
		return api.Course{}, err
	}
	return s.compose(course, version, false)
}

func (s *Service) GetLesson(ctx context.Context, lessonID string) (api.Lesson, string, error) {
	lesson, err := s.repo.Lesson(lessonID)
	if err != nil {
		return api.Lesson{}, "", err
	}
	version, err := s.repo.Version(lesson.CourseVersionID)
	if err != nil {
		return api.Lesson{}, "", err
	}
	course, err := s.repo.Course(version.CourseID)
	if err != nil {
		return api.Lesson{}, "", err
	}
	if course.Status == "withdrawn" || version.Status == "withdrawn" {
		return api.Lesson{}, "", apperr.NotFound("课时不可访问")
	}
	return toLesson(lesson), course.ID, nil
}

func (s *Service) ContentReadable(ctx context.Context, lessonID string) (api.Lesson, error) {
	lesson, _, err := s.GetLesson(ctx, lessonID)
	return lesson, err
}

func (s *Service) editable(ctx context.Context, courseID string, expected int64) (store.Course, store.Version, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return store.Course{}, store.Version{}, err
	}
	course, err := s.repo.Course(courseID)
	if err != nil {
		return store.Course{}, store.Version{}, err
	}
	if !s.canEdit(p, course) {
		return store.Course{}, store.Version{}, apperr.NotFound("课程不存在")
	}
	if course.Version != expected {
		return store.Course{}, store.Version{}, apperr.Conflict("VERSION_CONFLICT", "课程已变化，请刷新后重试")
	}
	version, err := s.repo.Version(course.DraftVersionID)
	if err != nil {
		return store.Course{}, store.Version{}, err
	}
	if version.Status != "draft" {
		course, version, err = s.repo.Fork(course, expected)
		if err != nil {
			return store.Course{}, store.Version{}, mapErr(err)
		}
	}
	return course, version, nil
}

func (s *Service) editor(ctx context.Context) (principal.Principal, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return principal.Principal{}, err
	}
	if !p.Can("course.manage") {
		return principal.Principal{}, apperr.Forbidden("无权管理课程")
	}
	return p, nil
}

func (s *Service) canEdit(p principal.Principal, course store.Course) bool {
	if s.cfg.Organization.Enabled && course.OrgID != p.OrgID {
		return false
	}
	return p.IsAdmin() || course.OwnerID == p.UserID
}

func (s *Service) org(p principal.Principal) string {
	if p.OrgID == "" {
		return "default"
	}
	return p.OrgID
}

func (s *Service) lessonFrom(req api.LessonWrite, versionID string) (store.Lesson, error) {
	if strings.TrimSpace(req.Title) == "" || req.ChapterID == "" {
		return store.Lesson{}, apperr.Invalid("课时标题和章节不能为空")
	}
	switch req.LessonType {
	case "video", "document", "exercise":
	default:
		return store.Lesson{}, apperr.Invalid("课时类型非法")
	}
	required := true
	if req.Required != nil {
		required = *req.Required
	}
	return store.Lesson{
		CourseVersionID: versionID, ChapterID: req.ChapterID, Title: req.Title, LessonType: req.LessonType,
		Required: required, DurationSec: req.DurationSec, ContentText: req.ContentText, ResourceID: req.ResourceID, SortOrder: req.SortOrder,
	}, nil
}

func (s *Service) compose(course store.Course, version store.Version, full bool) (api.Course, error) {
	chapters, lessons, err := s.repo.Outline(version.ID)
	if err != nil {
		return api.Course{}, err
	}
	return s.view(course, version, chapters, lessons, full), nil
}

func (s *Service) view(course store.Course, version store.Version, chapters []store.Chapter, lessons []store.Lesson, full bool) api.Course {
	out := api.Course{
		ID: course.ID, Title: version.Title, Summary: version.Summary, Status: course.Status, Version: course.Version,
		OwnerID: course.OwnerID, OrgID: course.OrgID, VersionNo: version.VersionNo,
	}
	if full {
		out.DraftVersionID = course.DraftVersionID
		out.Summary = version.Summary
		if version.SummaryDraft != "" {
			out.Summary = version.SummaryDraft
		}
	}
	out.PublishedVersionID = course.PublishedVersionID
	byChapter := map[string][]api.Lesson{}
	for _, lesson := range lessons {
		item := toLesson(lesson)
		if !full && course.Status == "withdrawn" {
			item.ContentText = ""
			item.ResourceID = ""
		}
		byChapter[lesson.ChapterID] = append(byChapter[lesson.ChapterID], item)
	}
	for _, ch := range chapters {
		out.Chapters = append(out.Chapters, api.Chapter{ID: ch.ID, Title: ch.Title, SortOrder: ch.SortOrder, Lessons: byChapter[ch.ID]})
	}
	return out
}

func toLesson(lesson store.Lesson) api.Lesson {
	return api.Lesson{
		ID: lesson.ID, Title: lesson.Title, LessonType: lesson.LessonType, Required: lesson.Required,
		DurationSec: lesson.DurationSec, ContentText: lesson.ContentText, ResourceID: lesson.ResourceID,
		SortOrder: lesson.SortOrder, ChapterID: lesson.ChapterID, CourseVersionID: lesson.CourseVersionID,
	}
}

func mapErr(err error) error {
	if errors.Is(err, database.ErrVersionConflict) {
		return apperr.Conflict("VERSION_CONFLICT", "课程已变化，请刷新后重试")
	}
	return err
}
