package planning

import (
	"github.com/cng1985/edugo/internal/modules/learning/planning/internal/core"
	"github.com/cng1985/edugo/internal/modules/learning/planning/internal/httpapi"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"go.uber.org/fx"
)

var Module = fx.Module("planning",
	fx.Provide(fx.Private, core.New, httpapi.New),
	fx.Provide(
		fx.Annotate(func(h *httpapi.Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`)),
		fx.Annotate(func() database.Contribution { return database.Contribution{Models: core.Models()} }, fx.ResultTags(`group:"schemas"`)),
	),
)
