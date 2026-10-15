package audit

import (
	"context"
	"time"

	"github.com/cng1985/edugo/internal/shared/trace"
	"github.com/cng1985/edugo/internal/shared/xid"
	"gorm.io/gorm"
)

type Entry struct {
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Before     string
	After      string
	Reason     string
}

type Log struct {
	ID         string `gorm:"primaryKey;size:36"`
	ActorID    string `gorm:"size:36;index"`
	Action     string `gorm:"size:128;index;not null"`
	TargetType string `gorm:"size:64"`
	TargetID   string `gorm:"size:36;index"`
	Before     string `gorm:"type:text"`
	After      string `gorm:"type:text"`
	Reason     string `gorm:"size:512"`
	TraceID    string `gorm:"size:64;index"`
	CreatedAt  time.Time
}

func (Log) TableName() string { return "audit_logs" }

type Logger struct{ db *gorm.DB }

func New(db *gorm.DB) *Logger { return &Logger{db: db} }

func (l *Logger) Record(ctx context.Context, tx *gorm.DB, e Entry) error {
	db := l.db
	if tx != nil {
		db = tx
	}
	row := Log{
		ID: xid.New(), ActorID: e.ActorID, Action: e.Action, TargetType: e.TargetType,
		TargetID: e.TargetID, Before: e.Before, After: e.After, Reason: e.Reason,
		TraceID: trace.From(ctx), CreatedAt: time.Now().UTC(),
	}
	return db.Create(&row).Error
}

func Models() []any { return []any{&Log{}} }
