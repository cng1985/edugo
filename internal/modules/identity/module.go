package identity

import (
	"context"

	"github.com/cng1985/edugo/internal/modules/identity/api"
	"github.com/cng1985/edugo/internal/modules/identity/internal/core"
	"github.com/cng1985/edugo/internal/modules/identity/internal/httpapi"
	"github.com/cng1985/edugo/internal/modules/identity/internal/store"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"go.uber.org/fx"
)

var Module = fx.Module("identity",
	fx.Provide(
		fx.Private,
		store.New,
		core.New,
		httpapi.New,
	),
	fx.Provide(
		func(s *core.Service) httpserver.SessionResolver { return s },
		func(s *core.Service) httpserver.AgentResolver { return s },
		func(s *core.Service) api.Accounts { return s },
		fx.Annotate(func(h *httpapi.Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`)),
		fx.Annotate(func() database.Contribution {
			return database.Contribution{Models: store.Models()}
		}, fx.ResultTags(`group:"schemas"`)),
	),
	fx.Invoke(func(s *core.Service, _ database.Ready, lc fx.Lifecycle) {
		lc.Append(fx.Hook{OnStart: func(ctx context.Context) error { return s.Seed(ctx) }})
	}),
)

func Service() any { return (*core.Service)(nil) }
