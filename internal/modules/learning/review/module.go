package review

import (
	"context"
	"encoding/json"

	asmapi "github.com/cng1985/edugo/internal/modules/assessment/api"
	"github.com/cng1985/edugo/internal/modules/learning/review/internal/core"
	"github.com/cng1985/edugo/internal/modules/learning/review/internal/httpapi"
	"github.com/cng1985/edugo/internal/modules/learning/review/internal/store"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/eventbus"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

var Module = fx.Module("review",
	fx.Provide(fx.Private, store.New, core.New, httpapi.New),
	fx.Provide(
		fx.Annotate(func(h *httpapi.Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`)),
		fx.Annotate(func() database.Contribution { return database.Contribution{Models: store.Models()} }, fx.ResultTags(`group:"schemas"`)),
		fx.Annotate(func(svc *core.Service) eventbus.Consumer {
			return eventbus.Consumer{
				Name:  "review",
				Types: map[string]struct{}{asmapi.EventPublished: {}, asmapi.EventRevised: {}, asmapi.EventInvalid: {}},
				Handle: func(ctx context.Context, tx *gorm.DB, ev eventbus.Event) error {
					var payload struct {
						UserID    string `json:"userId"`
						AttemptID string `json:"attemptId"`
					}
					if err := json.Unmarshal(ev.Payload, &payload); err != nil {
						return err
					}
					if ev.Type != asmapi.EventPublished {
						return nil
					}
					return svc.OnResult(ctx, payload.UserID, payload.AttemptID)
				},
			}
		}, fx.ResultTags(`group:"consumers"`)),
	),
)
