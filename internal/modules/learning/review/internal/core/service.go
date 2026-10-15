package core

import (
	"context"
	"errors"
	"strings"
	"time"

	asmapi "github.com/cng1985/edugo/internal/modules/assessment/api"
	"github.com/cng1985/edugo/internal/modules/learning/review/api"
	"github.com/cng1985/edugo/internal/modules/learning/review/internal/sm2"
	"github.com/cng1985/edugo/internal/modules/learning/review/internal/store"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/eventbus"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/xid"
	"gorm.io/gorm"
)

type Service struct {
	repo  *store.Repo
	facts asmapi.Reader
}

func New(repo *store.Repo, facts asmapi.Reader) *Service {
	return &Service{repo: repo, facts: facts}
}

func (s *Service) Create(ctx context.Context, front, back string) (api.Card, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Card{}, err
	}
	if !p.CanLearn() {
		return api.Card{}, apperr.Forbidden("不能代替他人创建复习卡片")
	}
	if strings.TrimSpace(front) == "" {
		return api.Card{}, apperr.Invalid("卡片正面不能为空")
	}
	init := sm2.Initial()
	card := store.Card{
		ID: xid.New(), UserID: p.UserID, ContentKey: "custom:" + xid.New(), Front: front, Back: back,
		EaseFactor: init.EaseFactor, Status: init.Status, Algorithm: sm2.Version, SourceStatus: "manual",
		NextReviewAt: time.Now().UTC(), Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := s.repo.Create(card); err != nil {
		return api.Card{}, err
	}
	return toAPI(card), nil
}

func (s *Service) EnsureWrong(ctx context.Context, userID, questionVersionID, stem string) error {
	key := "qv:" + questionVersionID
	if _, err := s.repo.ByKey(userID, key); err == nil {
		return nil
	}
	init := sm2.Initial()
	card := store.Card{
		ID: xid.New(), UserID: userID, ContentKey: key, QuestionVersion: questionVersionID, Front: stem, Back: "请回忆后再查看解析",
		EaseFactor: init.EaseFactor, Status: init.Status, Algorithm: sm2.Version, SourceStatus: "valid",
		NextReviewAt: time.Now().UTC(), Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	err := s.repo.Create(card)
	if err != nil {
		if _, e2 := s.repo.ByKey(userID, key); e2 == nil {
			return nil
		}
	}
	return err
}

func (s *Service) OnResult(ctx context.Context, userID, attemptID string) error {
	facts, err := s.facts.Facts(ctx, userID, attemptID)
	if err != nil {
		return err
	}
	for _, item := range facts.Wrong {
		if err := s.EnsureWrong(ctx, userID, item.QuestionVersionID, item.Stem); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Due(ctx context.Context) ([]api.Card, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.Due(p.UserID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return mapCards(rows), nil
}

func (s *Service) Submit(ctx context.Context, cardID, submissionID string, quality int, expected int64) (api.Card, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Card{}, err
	}
	if p.HasRole("teacher") && !p.HasRole("student") {
		return api.Card{}, apperr.Forbidden("教师不能代替学生提交记忆评分")
	}
	if strings.TrimSpace(submissionID) == "" {
		return api.Card{}, apperr.Invalid("缺少 submissionId")
	}
	card, err := s.repo.Get(cardID, p.UserID)
	if err != nil {
		return api.Card{}, err
	}
	if card.Status == "paused" || card.Status == "archived" {
		return api.Card{}, apperr.Invalid("卡片已暂停或归档")
	}
	if log, err := s.repo.FindLog(card.ID, submissionID); err == nil {
		_ = log
		return toAPI(card), nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return api.Card{}, err
	}
	if card.Version != expected {
		return api.Card{}, apperr.Conflict("VERSION_CONFLICT", "卡片已变化，请刷新后重试")
	}
	loc, _ := time.LoadLocation(p.Timezone)
	if loc == nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	now := time.Now().UTC()
	prev := sm2.State{EaseFactor: card.EaseFactor, Repetitions: card.Repetitions, IntervalDays: card.IntervalDays, Status: card.Status, SuccessStreak: card.SuccessStreak}
	next, at, err := sm2.Review(prev, quality, now, loc)
	if err != nil {
		return api.Card{}, err
	}
	before := store.StateJSON(card)
	card.EaseFactor = next.EaseFactor
	card.Repetitions = next.Repetitions
	card.IntervalDays = next.IntervalDays
	card.Status = next.Status
	card.SuccessStreak = next.SuccessStreak
	card.NextReviewAt = at
	card.LastReviewAt = &now
	ev, err := eventbus.NewEvent(ctx, api.EventSubmitted, card.ID, expected+1, map[string]any{
		"cardId": card.ID, "userId": p.UserID, "quality": quality, "algorithmVersion": sm2.Version, "reviewedAt": now,
	})
	if err != nil {
		return api.Card{}, err
	}
	saved, err := s.repo.Apply(card, expected, store.Log{
		ID: xid.New(), CardID: card.ID, SubmissionID: submissionID, Quality: quality, BeforeState: before, AfterState: store.StateJSON(card), ReviewedAt: now,
	}, ev)
	if errors.Is(err, database.ErrVersionConflict) {
		return api.Card{}, apperr.Conflict("VERSION_CONFLICT", "卡片已变化，请刷新后重试")
	}
	if err != nil {
		if log, e2 := s.repo.FindLog(card.ID, submissionID); e2 == nil {
			_ = log
			again, _ := s.repo.Get(cardID, p.UserID)
			return toAPI(again), nil
		}
		return api.Card{}, err
	}
	return toAPI(saved), nil
}

func (s *Service) Stats(ctx context.Context) (api.Stats, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return api.Stats{}, err
	}
	due, learning, review, mastered, paused, err := s.repo.Counts(p.UserID, time.Now().UTC())
	if err != nil {
		return api.Stats{}, err
	}
	return api.Stats{Due: int(due), Learning: int(learning), Review: int(review), Mastered: int(mastered), Paused: int(paused)}, nil
}

func (s *Service) Pause(ctx context.Context, id string, expected int64) error {
	return s.setStatus(ctx, id, expected, "paused")
}

func (s *Service) Resume(ctx context.Context, id string, expected int64) error {
	return s.setStatus(ctx, id, expected, "review")
}

func (s *Service) setStatus(ctx context.Context, id string, expected int64, status string) error {
	p, err := principal.Must(ctx)
	if err != nil {
		return err
	}
	if err := s.repo.SetStatus(id, p.UserID, status, expected); err != nil {
		if errors.Is(err, database.ErrVersionConflict) {
			return apperr.Conflict("VERSION_CONFLICT", "卡片已变化，请刷新后重试")
		}
		return err
	}
	return nil
}

func mapCards(rows []store.Card) []api.Card {
	out := make([]api.Card, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAPI(row))
	}
	return out
}

func toAPI(card store.Card) api.Card {
	return api.Card{
		ID: card.ID, ContentKey: card.ContentKey, Front: card.Front, Back: card.Back, Status: card.Status,
		EaseFactor: card.EaseFactor, IntervalDays: card.IntervalDays, Repetitions: card.Repetitions,
		NextReviewAt: card.NextReviewAt, Version: card.Version, Algorithm: card.Algorithm,
	}
}
