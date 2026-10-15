package assessment

import (
	"github.com/cng1985/edugo/internal/modules/assessment/api"
	"github.com/cng1985/edugo/internal/modules/assessment/internal/core"
	"github.com/cng1985/edugo/internal/modules/assessment/internal/httpapi"
	"github.com/cng1985/edugo/internal/modules/assessment/internal/store"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/cng1985/edugo/internal/platform/scheduler"
	"go.uber.org/fx"
	"time"
)

var Module = fx.Module("assessment",
	fx.Provide(fx.Private, store.New, core.New, httpapi.New),
	fx.Provide(
		func(s *core.Service) api.Reader { return s },
		fx.Annotate(func(h *httpapi.Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`)),
		fx.Annotate(func() database.Contribution { return database.Contribution{Models: store.Models()} }, fx.ResultTags(`group:"schemas"`)),
	),
	fx.Invoke(func(s *core.Service, sch *scheduler.Scheduler) {
		sch.Register("assessment.auto_submit", time.Minute, s.AutoSubmit)
	}),
)
