package app

import (
	"context"

	"github.com/cng1985/edugo/internal/modules/analytics"
	"github.com/cng1985/edugo/internal/modules/assessment"
	"github.com/cng1985/edugo/internal/modules/credential"
	"github.com/cng1985/edugo/internal/modules/identity"
	"github.com/cng1985/edugo/internal/modules/learning/learning"
	"github.com/cng1985/edugo/internal/modules/learning/planning"
	"github.com/cng1985/edugo/internal/modules/learning/review"
	"github.com/cng1985/edugo/internal/modules/notification"
	"github.com/cng1985/edugo/internal/modules/organization"
	"github.com/cng1985/edugo/internal/modules/teaching/course"
	"github.com/cng1985/edugo/internal/modules/teaching/knowledge"
	"github.com/cng1985/edugo/internal/modules/teaching/questionbank"
	"github.com/cng1985/edugo/internal/modules/tutoring"
	"github.com/cng1985/edugo/internal/platform/audit"
	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/eventbus"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/cng1985/edugo/internal/platform/idempotency"
	"github.com/cng1985/edugo/internal/platform/llm"
	"github.com/cng1985/edugo/internal/platform/observability"
	"github.com/cng1985/edugo/internal/platform/query"
	"github.com/cng1985/edugo/internal/platform/scheduler"
	"github.com/cng1985/edugo/internal/platform/storage"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

func New() *fx.App {
	return fx.New(Options()...)
}

func Run() {
	New().Run()
}

func Options() []fx.Option {
	return []fx.Option{
		fx.Provide(
			config.Load,
			observability.NewLogger,
			database.Open,
			migrate,
			audit.New,
			eventbus.New,
			scheduler.New,
			llm.New,
			storage.New,
			idempotency.New,
			httpserver.New,
		),
		identity.Module,
		organization.Module,
		course.Module,
		questionbank.Module,
		knowledge.Module,
		planning.Module,
		learning.Module,
		review.Module,
		assessment.Module,
		tutoring.Module,
		analytics.Module,
		credential.Module,
		notification.Module,
		query.Module,
		storage.HTTPModule,
		fx.Invoke(run),
	}
}

type migrateIn struct {
	fx.In
	DB    *gorm.DB
	Parts []database.Contribution `group:"schemas"`
}

func migrate(in migrateIn) (database.Ready, error) {
	parts := []database.Contribution{
		{Models: eventbus.Models()},
		{Models: scheduler.Models()},
		{Models: audit.Models()},
		{Models: storage.Models()},
		{Models: idempotency.Models()},
		{Models: llm.Models()},
	}
	parts = append(parts, in.Parts...)
	return database.Migrate(in.DB, parts)
}

func run(lc fx.Lifecycle, srv *httpserver.Server, bus *eventbus.Dispatcher, sch *scheduler.Scheduler, cfg *config.Config) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			bus.Start()
			sch.Start()
			if cfg.Server.Mode == "test" {
				return nil
			}
			return srv.Start()
		},
		OnStop: func(ctx context.Context) error {
			sch.Stop()
			bus.Stop()
			if cfg.Server.Mode == "test" {
				return nil
			}
			return srv.Stop(ctx)
		},
	})
}
