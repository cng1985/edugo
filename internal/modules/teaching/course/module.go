package course

import (
	"context"

	"github.com/cng1985/edugo/internal/modules/teaching/course/api"
	"github.com/cng1985/edugo/internal/modules/teaching/course/internal/core"
	"github.com/cng1985/edugo/internal/modules/teaching/course/internal/httpapi"
	"github.com/cng1985/edugo/internal/modules/teaching/course/internal/store"
	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/httpserver"
	"github.com/cng1985/edugo/internal/platform/query"
	"github.com/cng1985/edugo/internal/platform/storage"
	"github.com/cng1985/edugo/internal/shared/principal"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

var Module = fx.Module("course",
	fx.Provide(
		fx.Private,
		store.New,
		httpapi.New,
		func(s *storage.Store) core.Files { return s },
		core.New,
	),
	fx.Provide(
		func(s *core.Service) api.Reader { return s },
		fx.Annotate(func(h *httpapi.Handler) httpserver.Registrar { return h }, fx.ResultTags(`group:"routes"`)),
		fx.Annotate(func() database.Contribution {
			return database.Contribution{Models: store.Models()}
		}, fx.ResultTags(`group:"schemas"`)),
		fx.Annotate(newCourseQuery, fx.ResultTags(`group:"queries"`)),
	),
)

func newCourseQuery(db *gorm.DB) query.Model {
	fields := map[string]query.Field{
		"title":     query.StringField("title", false),
		"status":    {Column: "status", Type: "string", Sortable: true, Ops: query.Ops("eq")},
		"createdAt": {Column: "created_at", Type: "time", Sortable: true, Ops: query.Ops("gte", "lt")},
		"id":        {Column: "id", Type: "string", Sortable: true, Ops: query.Ops("eq")},
	}
	return query.Model{
		Code: "course", Fields: fields,
		Execute: func(ctx context.Context, gdb *gorm.DB, p principal.Principal, req query.Request) (any, int64, error) {
			q := gdb.WithContext(ctx).Model(&store.Course{})
			if !p.IsAdmin() {
				if p.Can("course.manage") {
					q = q.Where("owner_id = ? OR status = ?", p.UserID, "published")
				} else {
					q = q.Where("status = ?", "published")
				}
			}
			var err error
			q, err = query.Apply(q, fields, req.Filters, req.Sort)
			if err != nil {
				return nil, 0, err
			}
			var total int64
			if err := q.Count(&total).Error; err != nil {
				return nil, 0, err
			}
			if req.Page > 0 {
				q = q.Offset((req.Page - 1) * req.Size).Limit(req.Size)
			}
			var rows []store.Course
			if err := q.Find(&rows).Error; err != nil {
				return nil, 0, err
			}
			return rows, total, nil
		},
	}
}
