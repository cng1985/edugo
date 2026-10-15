package eventbus

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/shared/trace"
	"github.com/cng1985/edugo/internal/shared/xid"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

type Event struct {
	ID               string
	Type             string
	SchemaVersion    int
	AggregateID      string
	AggregateVersion int64
	OccurredAt       time.Time
	TraceID          string
	Payload          []byte
}

type Outbox struct {
	ID               string `gorm:"primaryKey;size:36"`
	EventType        string `gorm:"size:128;index;not null"`
	SchemaVersion    int    `gorm:"not null"`
	AggregateID      string `gorm:"size:36;index;not null"`
	AggregateVersion int64  `gorm:"not null"`
	OccurredAt       time.Time
	TraceID          string `gorm:"size:64"`
	Payload          string `gorm:"type:text"`
	Status           string `gorm:"size:32;index;not null"`
	Attempts         int    `gorm:"not null"`
	NextAttemptAt    time.Time `gorm:"index"`
	LastError        string `gorm:"size:512"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (Outbox) TableName() string { return "outbox_events" }

type Inbox struct {
	ID           string `gorm:"primaryKey;size:36"`
	ConsumerName string `gorm:"size:128;not null;uniqueIndex:uk_inbox"`
	EventID      string `gorm:"size:36;not null;uniqueIndex:uk_inbox"`
	ProcessedAt  time.Time
}

func (Inbox) TableName() string { return "consumer_inbox" }

type Failure struct {
	ID           string `gorm:"primaryKey;size:36"`
	EventID      string `gorm:"size:36;index"`
	ConsumerName string `gorm:"size:128"`
	Reason       string `gorm:"size:1024"`
	CreatedAt    time.Time
}

func (Failure) TableName() string { return "event_failures" }

func Models() []any { return []any{&Outbox{}, &Inbox{}, &Failure{}} }

func NewEvent(ctx context.Context, eventType, aggregateID string, version int64, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	return Event{
		ID: xid.New(), Type: eventType, SchemaVersion: 1, AggregateID: aggregateID,
		AggregateVersion: version, OccurredAt: time.Now().UTC(), TraceID: trace.From(ctx), Payload: raw,
	}, nil
}

func Insert(tx *gorm.DB, events ...Event) error {
	if len(events) == 0 {
		return nil
	}
	rows := make([]Outbox, 0, len(events))
	now := time.Now().UTC()
	for _, ev := range events {
		rows = append(rows, Outbox{
			ID: ev.ID, EventType: ev.Type, SchemaVersion: ev.SchemaVersion, AggregateID: ev.AggregateID,
			AggregateVersion: ev.AggregateVersion, OccurredAt: ev.OccurredAt, TraceID: ev.TraceID,
			Payload: string(ev.Payload), Status: "pending", NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
		})
	}
	return tx.Create(&rows).Error
}

type Consumer struct {
	Name   string
	Types  map[string]struct{}
	Handle func(ctx context.Context, tx *gorm.DB, ev Event) error
}

type Dispatcher struct {
	db        *gorm.DB
	cfg       *config.Config
	consumers []Consumer
	log       *slog.Logger
	cancel    context.CancelFunc
	done      chan struct{}
}

type Params struct {
	fx.In
	DB        *gorm.DB
	Cfg       *config.Config
	Log       *slog.Logger
	Consumers []Consumer `group:"consumers"`
}

func New(p Params) *Dispatcher {
	return &Dispatcher{db: p.DB, cfg: p.Cfg, consumers: p.Consumers, log: p.Log, done: make(chan struct{})}
}

func (d *Dispatcher) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	go func() {
		defer close(d.done)
		ticker := time.NewTicker(d.cfg.EventBus.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := d.Tick(ctx); err != nil {
					d.log.Error("outbox tick", "errorCode", "OUTBOX_TICK", "err", err.Error())
				}
			}
		}
	}()
}

func (d *Dispatcher) Stop() {
	if d.cancel != nil {
		d.cancel()
		<-d.done
	}
}

func (d *Dispatcher) Tick(ctx context.Context) error {
	var rows []Outbox
	now := time.Now().UTC()
	if err := d.db.WithContext(ctx).Where("status = ? AND next_attempt_at <= ?", "pending", now).Order("created_at asc, id asc").Limit(50).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := d.dispatchOne(ctx, row); err != nil {
			d.log.Error("outbox event", "eventId", row.ID, "eventType", row.EventType, "err", err.Error())
		}
	}
	return nil
}

func (d *Dispatcher) dispatchOne(ctx context.Context, row Outbox) error {
	ev := Event{
		ID: row.ID, Type: row.EventType, SchemaVersion: row.SchemaVersion, AggregateID: row.AggregateID,
		AggregateVersion: row.AggregateVersion, OccurredAt: row.OccurredAt, TraceID: row.TraceID, Payload: []byte(row.Payload),
	}
	failed := false
	var lastErr string
	for _, consumer := range d.consumers {
		if _, ok := consumer.Types[ev.Type]; !ok {
			continue
		}
		var count int64
		if err := d.db.Model(&Inbox{}).Where("consumer_name = ? AND event_id = ?", consumer.Name, ev.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		err := consumer.Handle(ctx, d.db, ev)
		if err == nil {
			inboxErr := d.db.Create(&Inbox{ID: xid.New(), ConsumerName: consumer.Name, EventID: ev.ID, ProcessedAt: time.Now().UTC()}).Error
			if inboxErr != nil && !isDuplicate(inboxErr) {
				err = inboxErr
			}
		}
		if err != nil {
			failed = true
			lastErr = trim(err.Error(), 500)
			_ = d.db.Create(&Failure{ID: xid.New(), EventID: ev.ID, ConsumerName: consumer.Name, Reason: lastErr, CreatedAt: time.Now().UTC()}).Error
		}
	}
	if failed {
		attempts := row.Attempts + 1
		status := "pending"
		if attempts >= d.cfg.EventBus.MaxAttempts {
			status = "dead"
		}
		delay := time.Duration(1<<min(attempts, 6)) * time.Second
		return d.db.Model(&Outbox{}).Where("id = ?", row.ID).Updates(map[string]any{
			"attempts": attempts, "status": status, "last_error": lastErr, "next_attempt_at": time.Now().UTC().Add(delay), "updated_at": time.Now().UTC(),
		}).Error
	}
	return d.db.Model(&Outbox{}).Where("id = ?", row.ID).Updates(map[string]any{"status": "delivered", "updated_at": time.Now().UTC()}).Error
}

func (d *Dispatcher) Replay(id string) error {
	return d.db.Model(&Outbox{}).Where("id = ?", id).Updates(map[string]any{
		"status": "pending", "next_attempt_at": time.Now().UTC(), "updated_at": time.Now().UTC(),
	}).Error
}

func (d *Dispatcher) Stats() (pending, dead int64, err error) {
	if err = d.db.Model(&Outbox{}).Where("status = ?", "pending").Count(&pending).Error; err != nil {
		return
	}
	err = d.db.Model(&Outbox{}).Where("status = ?", "dead").Count(&dead).Error
	return
}

func isDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
