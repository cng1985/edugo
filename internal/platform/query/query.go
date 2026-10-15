package query

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/cng1985/edugo/internal/platform/config"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/pagination"
	"github.com/cng1985/edugo/internal/shared/principal"
	"gorm.io/gorm"
)

var ident = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
var sqlFrag = regexp.MustCompile(`(?i)(union\s+select|drop\s+table|insert\s+into|delete\s+from|--|/\*|\*/|;)`)

type Field struct {
	Column   string
	Type     string
	Ops      map[string]struct{}
	Sortable bool
}

type Request struct {
	Model   string                     `json:"model"`
	Page    int                        `json:"page"`
	Size    int                        `json:"size"`
	Filters map[string]Filter          `json:"filters"`
	Sort    []pagination.Sort          `json:"sort"`
}

type Filter struct {
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

type Model struct {
	Code    string
	Fields  map[string]Field
	Execute func(ctx context.Context, db *gorm.DB, p principal.Principal, req Request) (any, int64, error)
}

type Engine struct {
	db     *gorm.DB
	cfg    *config.Config
	models map[string]Model
}

func New(db *gorm.DB, cfg *config.Config, models []Model) *Engine {
	m := map[string]Model{}
	for _, item := range models {
		m[item.Code] = item
	}
	return &Engine{db: db, cfg: cfg, models: m}
}

func (e *Engine) Run(ctx context.Context, kind string, p principal.Principal, req Request) (any, int64, int, int, error) {
	if !ident.MatchString(req.Model) || sqlFrag.MatchString(req.Model) {
		return nil, 0, 0, 0, apperr.Invalid("查询模型非法")
	}
	model, ok := e.models[req.Model]
	if !ok {
		return nil, 0, 0, 0, apperr.Invalid("未知查询模型")
	}
	if err := e.validate(req); err != nil {
		return nil, 0, 0, 0, err
	}
	page, size := req.Page, req.Size
	if kind != "one" {
		var err error
		page, size, err = pagination.Normalize(req.Page, req.Size, e.cfg.Query.DefaultPageSize, e.cfg.Query.MaxPageSize)
		if err != nil {
			return nil, 0, 0, 0, err
		}
		req.Page, req.Size = page, size
	}
	data, total, err := model.Execute(ctx, e.db, p, req)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	if kind == "list" && total > int64(e.cfg.Query.MaxListSize) {
		return nil, 0, 0, 0, apperr.Invalid("结果过大，请缩小筛选或使用导出任务")
	}
	return data, total, page, size, nil
}

func (e *Engine) validate(req Request) error {
	model := e.models[req.Model]
	for name, filter := range req.Filters {
		if sqlFrag.MatchString(name) || sqlFrag.MatchString(filter.Op) || sqlFrag.MatchString(string(filter.Value)) {
			return apperr.Invalid("查询包含不允许的片段")
		}
		field, ok := model.Fields[name]
		if !ok || !ident.MatchString(name) {
			return apperr.Invalid("字段未授权或不存在")
		}
		if _, ok := field.Ops[filter.Op]; !ok {
			return apperr.Invalid("不支持的操作符")
		}
	}
	for _, sort := range req.Sort {
		field, ok := model.Fields[sort.Field]
		if !ok || !field.Sortable {
			return apperr.Invalid("排序字段未授权")
		}
		if sort.Order != "asc" && sort.Order != "desc" {
			return apperr.Invalid("排序方向非法")
		}
	}
	return nil
}

func Apply(db *gorm.DB, fields map[string]Field, filters map[string]Filter, sorts []pagination.Sort) (*gorm.DB, error) {
	for name, filter := range filters {
		field := fields[name]
		col := field.Column
		var value any
		if err := json.Unmarshal(filter.Value, &value); err != nil {
			return nil, apperr.Invalid("过滤值无法解析")
		}
		switch filter.Op {
		case "eq":
			db = db.Where(col+" = ?", value)
		case "ne":
			db = db.Where(col+" <> ?", value)
		case "gt":
			db = db.Where(col+" > ?", value)
		case "gte":
			db = db.Where(col+" >= ?", value)
		case "lt":
			db = db.Where(col+" < ?", value)
		case "lte":
			db = db.Where(col+" <= ?", value)
		case "like":
			s, ok := value.(string)
			if !ok {
				return nil, apperr.Invalid("like 需要字符串")
			}
			s = strings.ReplaceAll(s, `\`, `\\`)
			s = strings.ReplaceAll(s, "%", `\%`)
			s = strings.ReplaceAll(s, "_", `\_`)
			db = db.Where(col+" LIKE ? ESCAPE '\\'", "%"+s+"%")
		case "in":
			arr, ok := value.([]any)
			if !ok || len(arr) == 0 || len(arr) > 100 {
				return nil, apperr.Invalid("in 参数非法")
			}
			db = db.Where(col+" IN ?", arr)
		case "between":
			arr, ok := value.([]any)
			if !ok || len(arr) != 2 {
				return nil, apperr.Invalid("between 需要两个边界")
			}
			db = db.Where(col+" >= ? AND "+col+" < ?", arr[0], arr[1])
		default:
			return nil, apperr.Invalid("不支持的操作符")
		}
	}
	if len(sorts) == 0 {
		sorts = []pagination.Sort{{Field: "createdAt", Order: "desc"}}
	}
	for _, sort := range sorts {
		field, ok := fields[sort.Field]
		if !ok {
			continue
		}
		db = db.Order(field.Column + " " + sort.Order)
	}
	if _, ok := fields["id"]; ok {
		db = db.Order("id desc")
	}
	return db, nil
}

func Ops(list ...string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, item := range list {
		m[item] = struct{}{}
	}
	return m
}

func StringField(column string, sortable bool) Field {
	return Field{Column: column, Type: "string", Sortable: sortable, Ops: Ops("eq", "ne", "like", "in")}
}
