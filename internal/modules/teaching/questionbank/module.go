package questionbank

import (
	"github.com/cng1985/edugo/internal/modules/teaching/questionbank/api"
	"github.com/cng1985/edugo/internal/modules/teaching/questionbank/internal/core"
	"github.com/cng1985/edugo/internal/modules/teaching/questionbank/internal/httpapi"
	"github.com/cng1985/edugo/internal/modules/teaching/questionbank/internal/store"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"go.uber.org/fx"
)

var Module = fx.Module("questionbank",
	fx.Provide(fx.Private, store.New, core.New, httpapi.New),
	fx.Provide(
		func(s *core.Service) api.Reader { return s },
		fx.Annotate(func(h *httpapi.Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`)),
		fx.Annotate(func() database.Contribution { return database.Contribution{Models: store.Models()} }, fx.ResultTags(`group:"schemas"`)),
	),
)
