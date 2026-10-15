package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/cng1985/edugo/internal/modules/assessment/api"
	"github.com/cng1985/edugo/internal/modules/assessment/internal/grading"
	"github.com/cng1985/edugo/internal/modules/assessment/internal/store"
	qapi "github.com/cng1985/edugo/internal/modules/teaching/questionbank/api"
	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/eventbus"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/xid"
	"gorm.io/gorm"
)

type Service struct {
	repo      *store.Repo
	questions qapi.Reader
	cfg       *config.Config
}

func New(repo *store.Repo, questions qapi.Reader, cfg *config.Config) *Service {
	return &Service{repo: repo, questions: questions, cfg: cfg}
}

type PaperInput struct {
	PaperID             string
	Title               string
	ExpectedVersion     int64
	QuestionVersionIDs  []string
	Scores              []int
}

func (s *Service) CreatePaper(ctx context.Context, title string) (api.Paper, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return api.Paper{}, err
	}
	if strings.TrimSpace(title) == "" {
		return api.Paper{}, apperr.Invalid("试卷标题不能为空")
	}
	now := time.Now().UTC()
	paper := store.Paper{ID: xid.New(), OwnerID: p.UserID, OrgID: orgOf(p), Title: title, Status: "draft", Version: 1, CreatedAt: now, UpdatedAt: now}
	ver := store.PaperVersion{ID: xid.New(), PaperID: paper.ID, VersionNo: 1, Status: "draft", Rules: "objective-exact-v1", CreatedAt: now}
	if err := s.repo.CreatePaper(paper, ver); err != nil {
		return api.Paper{}, err
	}
	paper.CurrentVersionID = ver.ID
	return api.Paper{ID: paper.ID, Title: title, Status: "draft", Version: 1, PaperVersionID: ver.ID}, nil
}

func (s *Service) UpdatePaper(ctx context.Context, in PaperInput) (api.Paper, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return api.Paper{}, err
	}
	paper, err := s.ownedPaper(p, in.PaperID)
	if err != nil {
		return api.Paper{}, err
	}
	if paper.Version != in.ExpectedVersion {
		return api.Paper{}, apperr.Conflict("VERSION_CONFLICT", "试卷已变化，请刷新后重试")
	}
	if paper.Status == "published" {
		return api.Paper{}, apperr.Conflict("PAPER_FROZEN", "已发布试卷不能覆盖，请新建版本")
	}
	if len(in.QuestionVersionIDs) != len(in.Scores) || len(in.QuestionVersionIDs) == 0 {
		return api.Paper{}, apperr.Invalid("题目与分值数量不一致")
	}
	version, err := s.repo.PaperVersion(paper.CurrentVersionID)
	if err != nil {
		return api.Paper{}, err
	}
	seen := map[string]bool{}
	qs := make([]store.PaperQuestion, 0, len(in.QuestionVersionIDs))
	total := 0
	for i, id := range in.QuestionVersionIDs {
		if seen[id] {
			return api.Paper{}, apperr.Invalid("试卷包含重复题目")
		}
		seen[id] = true
		if in.Scores[i] <= 0 {
			return api.Paper{}, apperr.Invalid("分值必须大于 0")
		}
		snap, err := s.questions.ApprovedSnapshot(ctx, id)
		if err != nil {
			return api.Paper{}, err
		}
		raw, _ := json.Marshal(map[string]any{
			"questionVersionId": snap.QuestionVersionID, "type": snap.Type, "stem": snap.Stem, "options": snap.Options,
			"answer": snap.Answer, "explanation": snap.Explanation, "scoreCenti": in.Scores[i],
		})
		total += in.Scores[i]
		qs = append(qs, store.PaperQuestion{ID: xid.New(), PaperVersionID: version.ID, QuestionVersionID: id, SortOrder: i + 1, ScoreCenti: in.Scores[i], Snapshot: string(raw)})
	}
	version.TotalScoreCenti = total
	paper.Title = in.Title
	if err := s.repo.ReplaceQuestions(paper, version, qs, paper.Version); err != nil {
		return api.Paper{}, mapErr(err)
	}
	paper.Version++
	return s.paperView(paper, version, qs, true)
}

func (s *Service) PublishPaper(ctx context.Context, id string, expected int64) (api.Paper, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return api.Paper{}, err
	}
	paper, err := s.ownedPaper(p, id)
	if err != nil {
		return api.Paper{}, err
	}
	if paper.Version != expected {
		return api.Paper{}, apperr.Conflict("VERSION_CONFLICT", "试卷已变化，请刷新后重试")
	}
	version, err := s.repo.PaperVersion(paper.CurrentVersionID)
	if err != nil {
		return api.Paper{}, err
	}
	qs, err := s.repo.Questions(version.ID)
	if err != nil {
		return api.Paper{}, err
	}
	if len(qs) == 0 || version.TotalScoreCenti <= 0 {
		return api.Paper{}, apperr.Invalid("发布失败：题目或总分不完整")
	}
	if err := s.repo.PublishPaper(paper, version, expected); err != nil {
		return api.Paper{}, mapErr(err)
	}
	paper.Version++
	paper.Status = "published"
	return s.paperView(paper, version, qs, true)
}

func (s *Service) CreateExam(ctx context.Context, title, paperID string, duration, maxAttempts, pass int, mode string) (api.Exam, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return api.Exam{}, err
	}
	if duration <= 0 || maxAttempts <= 0 {
		return api.Exam{}, apperr.Invalid("考试时长和次数必须大于 0")
	}
	if mode == "" {
		mode = "manual"
	}
	row := store.Exam{
		ID: xid.New(), OwnerID: p.UserID, OrgID: orgOf(p), PaperID: paperID, Title: title, Status: "draft",
		DurationSec: duration, MaxAttempts: maxAttempts, PassScoreCenti: pass, PublishMode: mode, Version: 1,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := s.repo.SaveExam(row); err != nil {
		return api.Exam{}, err
	}
	return toExam(row), nil
}

func (s *Service) PublishExam(ctx context.Context, id string, expected int64) (api.Exam, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return api.Exam{}, err
	}
	exam, err := s.ownedExam(p, id)
	if err != nil {
		return api.Exam{}, err
	}
	if exam.Version != expected {
		return api.Exam{}, apperr.Conflict("VERSION_CONFLICT", "考试已变化，请刷新后重试")
	}
	paper, err := s.repo.Paper(exam.PaperID)
	if err != nil {
		return api.Exam{}, err
	}
	if paper.Status != "published" {
		return api.Exam{}, apperr.Invalid("发布失败：试卷尚未发布")
	}
	version, err := s.repo.PaperVersion(paper.CurrentVersionID)
	if err != nil {
		return api.Exam{}, err
	}
	qs, err := s.repo.Questions(version.ID)
	if err != nil || len(qs) == 0 {
		return api.Exam{}, apperr.Invalid("发布失败：题目不足")
	}
	sum := 0
	for _, q := range qs {
		sum += q.ScoreCenti
	}
	if sum != version.TotalScoreCenti || exam.PassScoreCenti > sum {
		return api.Exam{}, apperr.Invalid("发布失败：总分或及格线冲突")
	}
	exam.Status = "published"
	exam.PaperVersionID = version.ID
	if err := s.repo.UpdateExam(exam, expected); err != nil {
		return api.Exam{}, mapErr(err)
	}
	exam.Version++
	return toExam(exam), nil
}

func (s *Service) Start(ctx context.Context, examID, idemKey string) (api.Attempt, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Attempt{}, err
	}
	if !p.CanLearn() {
		return api.Attempt{}, apperr.Forbidden("当前身份不能开始考试")
	}
	if strings.TrimSpace(idemKey) == "" {
		return api.Attempt{}, apperr.Invalid("开始考试需要 Idempotency-Key")
	}
	exam, err := s.repo.Exam(examID)
	if err != nil {
		return api.Attempt{}, err
	}
	if s.cfg.Organization.Enabled && exam.OrgID != p.OrgID {
		return api.Attempt{}, apperr.NotFound("考试不存在")
	}
	if exam.Status != "published" {
		return api.Attempt{}, apperr.Invalid("考试未开放")
	}
	now := time.Now().UTC()
	if exam.StartAt != nil && now.Before(*exam.StartAt) {
		return api.Attempt{}, apperr.Invalid("考试尚未开始")
	}
	if exam.EndAt != nil && now.After(*exam.EndAt) {
		return api.Attempt{}, apperr.Invalid("考试已结束")
	}
	if existing, err := s.repo.FindAttemptByKey(exam.ID, p.UserID, idemKey); err == nil {
		return s.attemptView(existing, false)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return api.Attempt{}, err
	}
	count, err := s.repo.CountAttempts(exam.ID, p.UserID)
	if err != nil {
		return api.Attempt{}, err
	}
	if int(count) >= exam.MaxAttempts {
		return api.Attempt{}, apperr.Forbidden("考试次数已用完")
	}
	deadline := now.Add(time.Duration(exam.DurationSec) * time.Second)
	if exam.EndAt != nil && exam.EndAt.Before(deadline) {
		deadline = *exam.EndAt
	}
	row := store.Attempt{
		ID: xid.New(), ExamID: exam.ID, UserID: p.UserID, AttemptNo: int(count) + 1, Status: "in_progress",
		StartedAt: now, DeadlineAt: deadline, PaperVersionID: exam.PaperVersionID, StartKey: idemKey, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.CreateAttempt(row); err != nil {
		if existing, e2 := s.repo.FindAttemptByKey(exam.ID, p.UserID, idemKey); e2 == nil {
			return s.attemptView(existing, false)
		}
		return api.Attempt{}, err
	}
	return s.attemptView(row, false)
}

func (s *Service) SaveAnswer(ctx context.Context, attemptID, questionID string, expected int64, answer json.RawMessage) (api.Answer, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Answer{}, err
	}
	attempt, err := s.ownedAttempt(p, attemptID)
	if err != nil {
		return api.Answer{}, err
	}
	if attempt.Status != "in_progress" {
		return api.Answer{}, apperr.Conflict("ATTEMPT_CLOSED", "答卷已提交")
	}
	if time.Now().UTC().After(attempt.DeadlineAt) {
		return api.Answer{}, apperr.Conflict("ATTEMPT_CLOSED", "已超过截止时间")
	}
	qs, err := s.repo.QuestionMap(attempt.PaperVersionID)
	if err != nil {
		return api.Answer{}, err
	}
	if _, ok := qs[questionID]; !ok {
		return api.Answer{}, apperr.Invalid("题目不属于本次答卷")
	}
	saved, err := s.repo.SaveAnswer(store.Answer{AttemptID: attemptID, QuestionSnapshotID: questionID, Answer: string(answer)}, expected)
	if err != nil {
		return api.Answer{}, err
	}
	return api.Answer{QuestionID: questionID, Answer: json.RawMessage(saved.Answer), Version: saved.Version, SavedAt: saved.SavedAt}, nil
}

func (s *Service) Submit(ctx context.Context, attemptID string) (api.Attempt, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Attempt{}, err
	}
	attempt, err := s.ownedAttempt(p, attemptID)
	if err != nil {
		return api.Attempt{}, err
	}
	if attempt.Status != "in_progress" {
		return s.attemptView(attempt, attempt.UserID == p.UserID && attempt.Status == "published")
	}
	exam, err := s.repo.Exam(attempt.ExamID)
	if err != nil {
		return api.Attempt{}, err
	}
	now := time.Now().UTC()
	if now.After(attempt.DeadlineAt.Add(time.Duration(exam.SubmitGraceSec) * time.Second)) {
		return s.finish(ctx, attempt, exam, false)
	}
	return s.finish(ctx, attempt, exam, true)
}

func (s *Service) finish(ctx context.Context, attempt store.Attempt, exam store.Exam, manual bool) (api.Attempt, error) {
	qs, err := s.repo.Questions(attempt.PaperVersionID)
	if err != nil {
		return api.Attempt{}, err
	}
	answers, err := s.repo.Answers(attempt.ID)
	if err != nil {
		return api.Attempt{}, err
	}
	byQ := map[string]store.Answer{}
	for _, a := range answers {
		byQ[a.QuestionSnapshotID] = a
	}
	lines := make([]store.GradeLine, 0, len(qs))
	subjective := false
	for _, q := range qs {
		snap, _, err := store.SnapshotOf(q.Snapshot)
		if err != nil {
			return api.Attempt{}, err
		}
		if snap.Type == "subjective" {
			subjective = true
			continue
		}
		raw := []byte(`{}`)
		if ans, ok := byQ[q.ID]; ok && ans.Answer != "" {
			raw = []byte(ans.Answer)
		}
		score, correct, err := grading.GradeObjective(snap, raw)
		if err != nil {
			return api.Attempt{}, err
		}
		lines = append(lines, store.GradeLine{Question: q, Score: score, Correct: correct})
	}
	if subjective {
		saved, err := s.repo.MarkPending(attempt.ID, "主观题待复核")
		if store.IsAlready(err) {
			again, _ := s.repo.Attempt(attempt.ID)
			return s.attemptView(again, again.Status == "published")
		}
		if err != nil {
			return api.Attempt{}, err
		}
		return s.attemptView(saved, false)
	}
	total := 0
	for _, line := range lines {
		total += line.Score
	}
	passed := total >= exam.PassScoreCenti
	var ev *eventbus.Event
	publish := exam.PublishMode == "auto_objective"
	if publish {
		e, err := eventbus.NewEvent(ctx, api.EventPublished, attempt.ID, 1, map[string]any{
			"userId": attempt.UserID, "attemptId": attempt.ID, "examId": attempt.ExamID, "scoreVersion": 1, "passed": passed,
		})
		if err != nil {
			return api.Attempt{}, err
		}
		ev = &e
	}
	saved, err := s.repo.SubmitObjective(attempt.ID, lines, exam.PassScoreCenti, publish, ev)
	if store.IsAlready(err) {
		again, _ := s.repo.Attempt(attempt.ID)
		show := again.Status == "published"
		return s.attemptView(again, show)
	}
	if err != nil {
		return api.Attempt{}, err
	}
	if publish && saved.Passed != nil && !*saved.Passed {
		// event payload passed flag corrected by not rewriting history; consumers read facts
	}
	_ = manual
	return s.attemptView(saved, publish)
}

func (s *Service) Result(ctx context.Context, attemptID string) (api.Attempt, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Attempt{}, err
	}
	attempt, err := s.repo.Attempt(attemptID)
	if err != nil {
		return api.Attempt{}, err
	}
	if attempt.UserID != p.UserID && !s.canReview(p, attempt.ExamID) {
		return api.Attempt{}, apperr.NotFound("答卷不存在")
	}
	show := attempt.Status == "published" || s.canReview(p, attempt.ExamID)
	if attempt.UserID != p.UserID && !s.canReview(p, attempt.ExamID) {
		show = false
	}
	return s.attemptView(attempt, show && (attempt.Status == "published" || s.canReview(p, attempt.ExamID)))
}

func (s *Service) ReviewScore(ctx context.Context, attemptID, questionID string, score float64, reason string) (api.Attempt, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Attempt{}, err
	}
	if !p.Can("exam.review") {
		return api.Attempt{}, apperr.Forbidden("无权复核成绩")
	}
	if strings.TrimSpace(reason) == "" {
		return api.Attempt{}, apperr.Invalid("复核必须填写原因")
	}
	attempt, err := s.repo.Attempt(attemptID)
	if err != nil || !s.canReview(p, attempt.ExamID) {
		return api.Attempt{}, apperr.NotFound("答卷不存在")
	}
	if attempt.Status == "published" || attempt.Status == "voided" {
		return api.Attempt{}, apperr.Conflict("ATTEMPT_STATE", "已发布成绩请使用改分")
	}
	qs, _ := s.repo.QuestionMap(attempt.PaperVersionID)
	q, ok := qs[questionID]
	if !ok {
		return api.Attempt{}, apperr.Invalid("题目不存在")
	}
	centi := int(score*100 + 0.5)
	if centi < 0 || centi > q.ScoreCenti {
		return api.Attempt{}, apperr.New(409, "SCORE_OUT_OF_RANGE", "分数超出题目分值")
	}
	before := 0
	if attempt.ScoreCenti != nil {
		before = *attempt.ScoreCenti
	}
	answers, _ := s.repo.Answers(attempt.ID)
	total := centi
	for _, ans := range answers {
		if ans.QuestionSnapshotID == questionID {
			continue
		}
		if ans.ScoreCenti != nil {
			total += *ans.ScoreCenti
		}
	}
	exam, _ := s.repo.Exam(attempt.ExamID)
	passed := total >= exam.PassScoreCenti
	now := time.Now().UTC()
	if err := s.repo.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&store.Answer{}).Where("attempt_id = ? AND question_snapshot_id = ?", attemptID, questionID).Updates(map[string]any{"score_centi": centi}).Error; err != nil {
			return err
		}
		if err := tx.Model(&store.Attempt{}).Where("id = ?", attemptID).Updates(map[string]any{
			"status": "ready", "score_centi": total, "passed": passed, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&store.ScoreRevision{
			ID: xid.New(), AttemptID: attemptID, BeforeCenti: before, AfterCenti: total, Reason: reason, ActorID: p.UserID, CreatedAt: now,
		}).Error
	}); err != nil {
		return api.Attempt{}, err
	}
	again, _ := s.repo.Attempt(attemptID)
	return s.attemptView(again, true)
}

func (s *Service) PublishResult(ctx context.Context, attemptID string) (api.Attempt, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Attempt{}, err
	}
	if !p.Can("exam.publishResult") {
		return api.Attempt{}, apperr.Forbidden("无权发布成绩")
	}
	attempt, err := s.repo.Attempt(attemptID)
	if err != nil || !s.canReview(p, attempt.ExamID) {
		return api.Attempt{}, apperr.NotFound("答卷不存在")
	}
	passed := false
	if attempt.Passed != nil {
		passed = *attempt.Passed
	}
	ev, err := eventbus.NewEvent(ctx, api.EventPublished, attempt.ID, int64(attempt.ScoreVersion+1), map[string]any{
		"userId": attempt.UserID, "attemptId": attempt.ID, "examId": attempt.ExamID, "scoreVersion": 1, "passed": passed,
	})
	if err != nil {
		return api.Attempt{}, err
	}
	if err := s.repo.PublishResult(attempt, p.UserID, ev); err != nil {
		return api.Attempt{}, err
	}
	again, _ := s.repo.Attempt(attemptID)
	return s.attemptView(again, true)
}

func (s *Service) ListExams(ctx context.Context) ([]api.Exam, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, err
	}
	org := ""
	if s.cfg.Organization.Enabled {
		org = p.OrgID
	}
	owner := ""
	status := "published"
	if p.Can("exam.manage") {
		owner = p.UserID
		status = ""
		if p.IsAdmin() {
			owner = ""
		}
	}
	rows, err := s.repo.ListExams(owner, org, status)
	if err != nil {
		return nil, err
	}
	out := make([]api.Exam, 0, len(rows))
	for _, row := range rows {
		if !p.Can("exam.manage") && row.Status != "published" {
			continue
		}
		out = append(out, toExam(row))
	}
	return out, nil
}

func (s *Service) Facts(ctx context.Context, userID, attemptID string) (api.Facts, error) {
	attempt, err := s.repo.Attempt(attemptID)
	if err != nil {
		return api.Facts{}, err
	}
	if attempt.UserID != userID || attempt.Status != "published" {
		return api.Facts{}, apperr.NotFound("成绩不可用")
	}
	qs, _ := s.repo.Questions(attempt.PaperVersionID)
	answers, _ := s.repo.Answers(attemptID)
	by := map[string]store.Answer{}
	for _, a := range answers {
		by[a.QuestionSnapshotID] = a
	}
	wrong := []api.WrongItem{}
	for _, q := range qs {
		snap, meta, _ := store.SnapshotOf(q.Snapshot)
		if snap.Type == "subjective" {
			continue
		}
		ans := by[q.ID]
		if ans.Correct == nil || !*ans.Correct {
			stem, _ := meta["stem"].(string)
			wrong = append(wrong, api.WrongItem{QuestionVersionID: q.QuestionVersionID, Stem: stem})
		}
	}
	score := 0.0
	if attempt.ScoreCenti != nil {
		score = float64(*attempt.ScoreCenti) / 100
	}
	passed := false
	if attempt.Passed != nil {
		passed = *attempt.Passed
	}
	return api.Facts{UserID: userID, AttemptID: attempt.ID, ExamID: attempt.ExamID, Published: true, Passed: passed, Score: score, ScoreVersion: attempt.ScoreVersion, Wrong: wrong}, nil
}

func (s *Service) AutoSubmit(ctx context.Context) error {
	rows, err := s.repo.DueAttempts(time.Now().UTC())
	if err != nil {
		return err
	}
	for _, row := range rows {
		exam, err := s.repo.Exam(row.ExamID)
		if err != nil {
			continue
		}
		_, _ = s.finish(ctx, row, exam, false)
	}
	return nil
}

func (s *Service) attemptView(attempt store.Attempt, reveal bool) (api.Attempt, error) {
	qs, err := s.repo.Questions(attempt.PaperVersionID)
	if err != nil {
		return api.Attempt{}, err
	}
	answers, _ := s.repo.Answers(attempt.ID)
	by := map[string]store.Answer{}
	for _, a := range answers {
		by[a.QuestionSnapshotID] = a
	}
	view := api.Attempt{ID: attempt.ID, ExamID: attempt.ExamID, Status: attempt.Status, AttemptNo: attempt.AttemptNo, DeadlineAt: attempt.DeadlineAt, Version: attempt.Version}
	if reveal && attempt.ScoreCenti != nil {
		score := float64(*attempt.ScoreCenti) / 100
		view.Score = &score
		view.Passed = attempt.Passed
	}
	for _, q := range qs {
		_, meta, err := store.SnapshotOf(q.Snapshot)
		if err != nil {
			continue
		}
		item := api.Question{ID: q.ID, Type: asString(meta["type"]), Stem: asString(meta["stem"]), Score: float64(q.ScoreCenti) / 100}
		if raw, ok := meta["options"].(json.RawMessage); ok {
			item.Options = raw
		}
		if reveal {
			if raw, ok := meta["answer"].(json.RawMessage); ok {
				item.Answer = raw
			}
		}
		view.Questions = append(view.Questions, item)
		if ans, ok := by[q.ID]; ok {
			a := api.Answer{QuestionID: q.ID, Answer: json.RawMessage(ans.Answer), Version: ans.Version, SavedAt: ans.SavedAt}
			if reveal && ans.ScoreCenti != nil {
				sc := float64(*ans.ScoreCenti) / 100
				a.Score = &sc
			}
			view.Answers = append(view.Answers, a)
		}
	}
	return view, nil
}

func (s *Service) paperView(paper store.Paper, version store.PaperVersion, qs []store.PaperQuestion, reveal bool) (api.Paper, error) {
	view := api.Paper{ID: paper.ID, Title: paper.Title, Status: paper.Status, Version: paper.Version, Total: float64(version.TotalScoreCenti) / 100, PaperVersionID: version.ID}
	for _, q := range qs {
		_, meta, _ := store.SnapshotOf(q.Snapshot)
		item := api.Question{ID: q.ID, Stem: asString(meta["stem"]), Type: asString(meta["type"]), Score: float64(q.ScoreCenti) / 100}
		view.Questions = append(view.Questions, item)
	}
	_ = reveal
	return view, nil
}

func (s *Service) editor(ctx context.Context) (principal.Principal, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return principal.Principal{}, err
	}
	if !p.Can("exam.manage") {
		return principal.Principal{}, apperr.Forbidden("无权管理考试")
	}
	return p, nil
}

func (s *Service) ownedPaper(p principal.Principal, id string) (store.Paper, error) {
	row, err := s.repo.Paper(id)
	if err != nil {
		return store.Paper{}, err
	}
	if s.cfg.Organization.Enabled && row.OrgID != p.OrgID {
		return store.Paper{}, apperr.NotFound("试卷不存在")
	}
	if !p.IsAdmin() && row.OwnerID != p.UserID {
		return store.Paper{}, apperr.NotFound("试卷不存在")
	}
	return row, nil
}

func (s *Service) ownedExam(p principal.Principal, id string) (store.Exam, error) {
	row, err := s.repo.Exam(id)
	if err != nil {
		return store.Exam{}, err
	}
	if !s.canReview(p, row.ID) && row.OwnerID != p.UserID {
		return store.Exam{}, apperr.NotFound("考试不存在")
	}
	return row, nil
}

func (s *Service) ownedAttempt(p principal.Principal, id string) (store.Attempt, error) {
	row, err := s.repo.Attempt(id)
	if err != nil {
		return store.Attempt{}, err
	}
	if row.UserID != p.UserID {
		return store.Attempt{}, apperr.NotFound("答卷不存在")
	}
	return row, nil
}

func (s *Service) canReview(p principal.Principal, examID string) bool {
	if !p.Can("exam.review") && !p.Can("exam.manage") {
		return false
	}
	exam, err := s.repo.Exam(examID)
	if err != nil {
		return false
	}
	if s.cfg.Organization.Enabled && exam.OrgID != p.OrgID {
		return false
	}
	return p.IsAdmin() || exam.OwnerID == p.UserID
}

func toExam(row store.Exam) api.Exam {
	return api.Exam{
		ID: row.ID, Title: row.Title, Status: row.Status, PaperID: row.PaperID, DurationSec: row.DurationSec,
		MaxAttempts: row.MaxAttempts, PassScore: float64(row.PassScoreCenti) / 100, PublishMode: row.PublishMode,
		Version: row.Version, StartAt: row.StartAt, EndAt: row.EndAt,
	}
}

func orgOf(p principal.Principal) string {
	if p.OrgID == "" {
		return "default"
	}
	return p.OrgID
}

func mapErr(err error) error {
	if errors.Is(err, database.ErrVersionConflict) {
		return apperr.Conflict("VERSION_CONFLICT", "数据已变化，请刷新后重试")
	}
	return err
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
