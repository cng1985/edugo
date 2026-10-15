package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/cng1985/edugo/internal/modules/teaching/questionbank/api"
	"github.com/cng1985/edugo/internal/modules/teaching/questionbank/internal/store"
	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/llm"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/pagination"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/xid"
)

type Service struct {
	repo *store.Repo
	llm  llm.Client
	cfg  *config.Config
}

func New(repo *store.Repo, client llm.Client, cfg *config.Config) *Service {
	return &Service{repo: repo, llm: client, cfg: cfg}
}

func (s *Service) Create(ctx context.Context, req api.WriteRequest) (api.Question, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return api.Question{}, err
	}
	if err := validate(req); err != nil {
		return api.Question{}, err
	}
	now := time.Now().UTC()
	q := store.Question{ID: xid.New(), OwnerID: p.UserID, OrgID: org(p), Status: "draft", Version: 1, CreatedAt: now, UpdatedAt: now}
	v := versionFrom(req, q.ID, 1)
	if err := s.repo.Create(q, v); err != nil {
		return api.Question{}, err
	}
	q.CurrentVersionID = v.ID
	return toAPI(q, v, true), nil
}

func (s *Service) Update(ctx context.Context, req api.WriteRequest) (api.Question, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return api.Question{}, err
	}
	q, err := s.owned(p, req.QuestionID)
	if err != nil {
		return api.Question{}, err
	}
	if q.Version != req.ExpectedVersion {
		return api.Question{}, apperr.Conflict("VERSION_CONFLICT", "题目已变化，请刷新后重试")
	}
	if err := validate(req); err != nil {
		return api.Question{}, err
	}
	current, err := s.repo.Version(q.CurrentVersionID)
	if err != nil {
		return api.Question{}, err
	}
	var v store.Version
	if current.ReviewStatus == "draft" {
		v = versionFrom(req, q.ID, current.VersionNo)
		v.ID = current.ID
		v.CreatedAt = current.CreatedAt
	} else {
		n, err := s.repo.LatestNo(q.ID)
		if err != nil {
			return api.Question{}, err
		}
		v = versionFrom(req, q.ID, n+1)
	}
	q.Status = "draft"
	if err := s.repo.SaveDraft(q, v, q.Version); err != nil {
		return api.Question{}, mapErr(err)
	}
	q.Version++
	q.CurrentVersionID = v.ID
	return toAPI(q, v, true), nil
}

func (s *Service) Detail(ctx context.Context, id string) (api.Question, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Question{}, err
	}
	if p.HasRole("student") && !p.Can("question.manage") {
		return api.Question{}, apperr.NotFound("题目不存在")
	}
	q, err := s.owned(p, id)
	if err != nil {
		return api.Question{}, err
	}
	v, err := s.repo.Version(q.CurrentVersionID)
	if err != nil {
		return api.Question{}, err
	}
	return toAPI(q, v, true), nil
}

func (s *Service) List(ctx context.Context, req api.ListRequest) ([]api.Question, int64, int, int, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	page, size, err := pagination.Normalize(req.Page, req.Size, s.cfg.Query.DefaultPageSize, s.cfg.Query.MaxPageSize)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	owner := p.UserID
	if p.IsAdmin() {
		owner = ""
	}
	orgFilter := ""
	if s.cfg.Organization.Enabled {
		orgFilter = p.OrgID
	}
	rows, total, err := s.repo.List(owner, orgFilter, req.Type, req.Status, req.Stem, pagination.Offset(page, size), size)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	out := make([]api.Question, 0, len(rows))
	for _, row := range rows {
		v, err := s.repo.Version(row.CurrentVersionID)
		if err != nil {
			continue
		}
		out = append(out, toAPI(row, v, true))
	}
	return out, total, page, size, nil
}

func (s *Service) SubmitReview(ctx context.Context, req api.IDRequest) (api.Question, error) {
	return s.setReview(ctx, req, "draft", "pending_review")
}

func (s *Service) Approve(ctx context.Context, req api.IDRequest) (api.Question, error) {
	return s.setReview(ctx, req, "pending_review", "approved")
}

func (s *Service) Reject(ctx context.Context, req api.IDRequest) (api.Question, error) {
	if strings.TrimSpace(req.Reason) == "" {
		return api.Question{}, apperr.Invalid("拒绝必须填写原因")
	}
	return s.setReview(ctx, req, "pending_review", "rejected")
}

func (s *Service) setReview(ctx context.Context, req api.IDRequest, from, to string) (api.Question, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return api.Question{}, err
	}
	q, err := s.owned(p, req.QuestionID)
	if err != nil {
		return api.Question{}, err
	}
	if q.Version != req.ExpectedVersion {
		return api.Question{}, apperr.Conflict("VERSION_CONFLICT", "题目已变化，请刷新后重试")
	}
	v, err := s.repo.Version(q.CurrentVersionID)
	if err != nil {
		return api.Question{}, err
	}
	if v.ReviewStatus != from {
		return api.Question{}, apperr.Conflict("QUESTION_STATE", "题目状态不允许该操作")
	}
	v.ReviewStatus = to
	q.Status = to
	if err := s.repo.SaveDraft(q, v, q.Version); err != nil {
		return api.Question{}, mapErr(err)
	}
	q.Version++
	return toAPI(q, v, true), nil
}

func (s *Service) Generate(ctx context.Context, req api.GenerateRequest) (api.Generation, error) {
	p, err := s.editor(ctx)
	if err != nil {
		return api.Generation{}, err
	}
	if req.Count < 1 || req.Count > 50 {
		return api.Generation{}, apperr.Invalid("出题数量必须在 1 到 50 之间")
	}
	task := store.Generation{ID: xid.New(), OwnerID: p.UserID, Status: "running", Input: req.Type, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.repo.CreateGeneration(task); err != nil {
		return api.Generation{}, err
	}
	raw, err := s.llm.Complete(ctx, llm.ChatRequest{
		Module: "questionbank", TaskID: task.ID, Profile: "questionGeneration", PromptVersion: "gen-v1", SchemaVersion: "v1", JSONMode: true,
		System: "只输出 JSON：{\"questions\":[{\"type\":\"single\",\"stem\":\"\",\"options\":[{\"key\":\"A\",\"text\":\"\"}],\"answer\":{\"choice\":\"A\"},\"explanation\":\"\",\"difficulty\":\"easy\"}]}。答案必须命中选项。",
		User:   "题型 " + req.Type + " 难度 " + req.Difficulty + " 数量 " + itoa(req.Count),
	})
	if err != nil {
		task.Status = "failed"
		task.Error = apperr.As(err).ErrorCode
		_ = s.repo.SaveGeneration(task)
		return api.Generation{ID: task.ID, Status: task.Status, Error: task.Error}, nil
	}
	items, vErr := parseGenerated(raw.Content, req)
	if vErr != nil {
		task.Status = "failed"
		task.Error = vErr.Error()
		task.Output = raw.Content
		_ = s.repo.SaveGeneration(task)
		return api.Generation{ID: task.ID, Status: "failed", Error: "校验失败，未写入正式题库"}, nil
	}
	out := make([]api.Question, 0, len(items))
	for _, item := range items {
		item.MaxScoreCenti = 1000
		created, err := s.Create(ctx, item)
		if err != nil {
			task.Status = "failed"
			task.Error = err.Error()
			_ = s.repo.SaveGeneration(task)
			return api.Generation{ID: task.ID, Status: "failed", Error: "校验失败，未写入正式题库"}, nil
		}
		pending, err := s.SubmitReview(ctx, api.IDRequest{QuestionID: created.ID, ExpectedVersion: created.Version})
		if err != nil {
			return api.Generation{}, err
		}
		out = append(out, pending)
	}
	task.Status = "succeeded"
	task.Output = raw.Content
	_ = s.repo.SaveGeneration(task)
	return api.Generation{ID: task.ID, Status: "succeeded", Items: out}, nil
}

func (s *Service) ApprovedSnapshot(ctx context.Context, versionID string) (api.Snapshot, error) {
	v, err := s.repo.Version(versionID)
	if err != nil {
		return api.Snapshot{}, err
	}
	if v.ReviewStatus != "approved" {
		return api.Snapshot{}, apperr.Invalid("题目尚未审核通过")
	}
	return api.Snapshot{
		QuestionVersionID: v.ID, Type: v.Type, Stem: v.Stem, Options: json.RawMessage(v.Options),
		Answer: json.RawMessage(v.Answer), Explanation: v.Explanation, MaxScoreCenti: v.MaxScoreCenti, Difficulty: v.Difficulty,
	}, nil
}

func (s *Service) editor(ctx context.Context) (principal.Principal, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return principal.Principal{}, err
	}
	if !p.Can("question.manage") {
		return principal.Principal{}, apperr.Forbidden("无权管理题库")
	}
	return p, nil
}

func (s *Service) owned(p principal.Principal, id string) (store.Question, error) {
	q, err := s.repo.Question(id)
	if err != nil {
		return store.Question{}, err
	}
	if s.cfg.Organization.Enabled && q.OrgID != p.OrgID {
		return store.Question{}, apperr.NotFound("题目不存在")
	}
	if !p.IsAdmin() && q.OwnerID != p.UserID {
		return store.Question{}, apperr.NotFound("题目不存在")
	}
	return q, nil
}

func validate(req api.WriteRequest) error {
	switch req.Type {
	case "single", "multiple", "blank", "judge", "subjective":
	default:
		return apperr.Invalid("题型非法")
	}
	if strings.TrimSpace(req.Stem) == "" {
		return apperr.Invalid("题干不能为空")
	}
	if req.Type != "subjective" && !json.Valid(req.Answer) {
		return apperr.Invalid("答案格式非法")
	}
	return nil
}

func versionFrom(req api.WriteRequest, questionID string, no int) store.Version {
	score := req.MaxScoreCenti
	if score <= 0 {
		score = 1000
	}
	options := string(req.Options)
	if options == "" {
		options = "[]"
	}
	answer := string(req.Answer)
	if answer == "" {
		answer = "{}"
	}
	return store.Version{
		ID: xid.New(), QuestionID: questionID, VersionNo: no, Type: req.Type, Stem: req.Stem, Options: options,
		Answer: answer, Explanation: req.Explanation, Difficulty: req.Difficulty, MaxScoreCenti: score,
		ReviewStatus: "draft", CreatedAt: time.Now().UTC(),
	}
}

func toAPI(q store.Question, v store.Version, withAnswer bool) api.Question {
	item := api.Question{
		ID: q.ID, VersionID: v.ID, VersionNo: v.VersionNo, Type: v.Type, Stem: v.Stem, Difficulty: v.Difficulty,
		MaxScoreCenti: v.MaxScoreCenti, ReviewStatus: v.ReviewStatus, OwnerID: q.OwnerID, Version: q.Version,
		Options: json.RawMessage(v.Options),
	}
	if withAnswer {
		item.Answer = json.RawMessage(v.Answer)
		item.Explanation = v.Explanation
	}
	return item
}

func parseGenerated(raw string, req api.GenerateRequest) ([]api.WriteRequest, error) {
	var body struct {
		Questions []struct {
			Type        string          `json:"type"`
			Stem        string          `json:"stem"`
			Options     json.RawMessage `json:"options"`
			Answer      json.RawMessage `json:"answer"`
			Explanation string          `json:"explanation"`
			Difficulty  string          `json:"difficulty"`
		} `json:"questions"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, err
	}
	if len(body.Questions) != req.Count {
		return nil, errors.New("count")
	}
	out := make([]api.WriteRequest, 0, len(body.Questions))
	for _, q := range body.Questions {
		if req.Type != "" && q.Type != req.Type {
			return nil, errors.New("type")
		}
		item := api.WriteRequest{Type: q.Type, Stem: q.Stem, Options: q.Options, Answer: q.Answer, Explanation: q.Explanation, Difficulty: q.Difficulty}
		if err := validate(item); err != nil {
			return nil, err
		}
		if q.Type == "single" || q.Type == "multiple" || q.Type == "judge" {
			var ans struct {
				Choice  string   `json:"choice"`
				Choices []string `json:"choices"`
			}
			_ = json.Unmarshal(q.Answer, &ans)
			var opts []struct {
				Key string `json:"key"`
			}
			_ = json.Unmarshal(q.Options, &opts)
			keys := map[string]bool{}
			for _, op := range opts {
				keys[op.Key] = true
			}
			if ans.Choice != "" && !keys[ans.Choice] {
				return nil, errors.New("answer option")
			}
			for _, c := range ans.Choices {
				if !keys[c] {
					return nil, errors.New("answer option")
				}
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func org(p principal.Principal) string {
	if p.OrgID == "" {
		return "default"
	}
	return p.OrgID
}

func mapErr(err error) error {
	if errors.Is(err, database.ErrVersionConflict) {
		return apperr.Conflict("VERSION_CONFLICT", "题目已变化，请刷新后重试")
	}
	return err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
