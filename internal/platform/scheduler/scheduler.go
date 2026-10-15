package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/shared/xid"
	"gorm.io/gorm"
)

type Definition struct {
	Name       string `gorm:"primaryKey;size:128"`
	IntervalSec int   `gorm:"not null"`
	NextRunAt  time.Time `gorm:"index"`
	LeaseOwner string `gorm:"size:64"`
	LeaseUntil *time.Time
	Paused     bool
	UpdatedAt  time.Time
}

func (Definition) TableName() string { return "job_definitions" }

type Run struct {
	ID         string `gorm:"primaryKey;size:36"`
	JobName    string `gorm:"size:128;index"`
	StartedAt  time.Time
	FinishedAt *time.Time
	Status     string `gorm:"size:32"`
	Error      string `gorm:"size:1024"`
}

func (Run) TableName() string { return "job_runs" }

func Models() []any { return []any{&Definition{}, &Run{}} }

type Handler func(ctx context.Context) error

type job struct {
	name     string
	interval time.Duration
	handle   Handler
}

type Scheduler struct {
	db     *gorm.DB
	cfg    *config.Config
	log    *slog.Logger
	mu     sync.Mutex
	jobs   []job
	owner  string
	cancel context.CancelFunc
	done   chan struct{}
}

func New(db *gorm.DB, cfg *config.Config, log *slog.Logger) *Scheduler {
	return &Scheduler{db: db, cfg: cfg, log: log, owner: xid.New(), done: make(chan struct{})}
}

func (s *Scheduler) Register(name string, interval time.Duration, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, job{name: name, interval: interval, handle: h})
}

func (s *Scheduler) ensure() error {
	s.mu.Lock()
	jobs := append([]job(nil), s.jobs...)
	s.mu.Unlock()
	now := time.Now().UTC()
	for _, j := range jobs {
		var def Definition
		err := s.db.Where("name = ?", j.name).First(&def).Error
		if err == gorm.ErrRecordNotFound {
			def = Definition{Name: j.name, IntervalSec: int(j.interval.Seconds()), NextRunAt: now, UpdatedAt: now}
			if def.IntervalSec < 1 {
				def.IntervalSec = 1
			}
			if err := s.db.Create(&def).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) Start() {
	if !s.cfg.Scheduler.Enabled {
		close(s.done)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		defer close(s.done)
		_ = s.ensure()
		ticker := time.NewTicker(s.cfg.Scheduler.CheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.Tick(ctx); err != nil {
					s.log.Error("scheduler tick", "err", err.Error())
				}
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	<-s.done
}

func (s *Scheduler) Tick(ctx context.Context) error {
	if err := s.ensure(); err != nil {
		return err
	}
	s.mu.Lock()
	jobs := append([]job(nil), s.jobs...)
	s.mu.Unlock()
	now := time.Now().UTC()
	for _, j := range jobs {
		claimed, err := s.claim(j.name, now)
		if err != nil || !claimed {
			continue
		}
		run := Run{ID: xid.New(), JobName: j.name, StartedAt: time.Now().UTC(), Status: "running"}
		_ = s.db.Create(&run).Error
		err = j.handle(ctx)
		finished := time.Now().UTC()
		status := "succeeded"
		msg := ""
		if err != nil {
			status = "failed"
			msg = trim(err.Error(), 1000)
			s.log.Error("job failed", "job", j.name, "err", msg)
		}
		_ = s.db.Model(&Run{}).Where("id = ?", run.ID).Updates(map[string]any{"finished_at": finished, "status": status, "error": msg}).Error
		next := time.Now().UTC().Add(j.interval)
		_ = s.db.Model(&Definition{}).Where("name = ?", j.name).Updates(map[string]any{
			"next_run_at": next, "lease_owner": "", "lease_until": nil, "updated_at": time.Now().UTC(),
		}).Error
	}
	return nil
}

func (s *Scheduler) claim(name string, now time.Time) (bool, error) {
	until := now.Add(45 * time.Second)
	res := s.db.Model(&Definition{}).
		Where("name = ? AND paused = ? AND next_run_at <= ? AND (lease_until IS NULL OR lease_until < ?)", name, false, now, now).
		Updates(map[string]any{"lease_owner": s.owner, "lease_until": until, "updated_at": now})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (s *Scheduler) ListRuns(limit int) ([]Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var rows []Run
	err := s.db.Order("started_at desc").Limit(limit).Find(&rows).Error
	return rows, err
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
