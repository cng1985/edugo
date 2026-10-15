package core

import (
	"context"
	"strings"
	"time"

	"github.com/cng1985/edugo/internal/modules/learning/planning/internal/graph"
	"github.com/cng1985/edugo/internal/platform/llm"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/principal"
	"github.com/cng1985/edugo/internal/shared/xid"
	"gorm.io/gorm"
)

type Plan struct {
	ID         string    `gorm:"primaryKey;size:36" json:"id"`
	UserID     string    `gorm:"size:36;index" json:"userId"`
	Title      string    `gorm:"size:200" json:"title"`
	Status     string    `gorm:"size:32;index" json:"status"`
	Timezone   string    `gorm:"size:64" json:"timezone"`
	DeadlineAt time.Time `json:"deadlineAt"`
	Version    int64     `json:"version"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (Plan) TableName() string { return "study_plans" }

type Item struct {
	ID       string `gorm:"primaryKey;size:36" json:"id"`
	PlanID   string `gorm:"size:36;index" json:"planId"`
	Title    string `json:"title"`
	Required bool   `json:"required"`
	Status   string `gorm:"size:32" json:"status"`
	Manual   bool   `json:"manual"`
}

func (Item) TableName() string { return "plan_items" }

type Path struct {
	ID              string `gorm:"primaryKey;size:36" json:"id"`
	UserID          string `gorm:"size:36;index" json:"userId"`
	Title           string `json:"title"`
	Status          string `gorm:"size:32" json:"status"`
	ActiveVersionID string `gorm:"size:36" json:"activeVersionId"`
	Version         int64  `json:"version"`
}

func (Path) TableName() string { return "learning_paths" }

type PathVersion struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	PathID    string `gorm:"size:36;index" json:"pathId"`
	VersionNo int    `json:"versionNo"`
	Status    string `gorm:"size:32" json:"status"`
}

func (PathVersion) TableName() string { return "path_versions" }

type Node struct {
	ID            string `gorm:"primaryKey;size:36" json:"id"`
	PathVersionID string `gorm:"size:36;index" json:"pathVersionId"`
	Title         string `json:"title"`
	CourseID      string `gorm:"size:36" json:"courseId"`
	Required      bool   `json:"required"`
	SortOrder     int    `json:"sortOrder"`
	Status        string `gorm:"size:32" json:"status"`
}

func (Node) TableName() string { return "path_nodes" }

type Edge struct {
	ID            string `gorm:"primaryKey;size:36" json:"id"`
	PathVersionID string `gorm:"size:36;not null;uniqueIndex:uk_edge" json:"pathVersionId"`
	FromNodeID    string `gorm:"size:36;not null;uniqueIndex:uk_edge" json:"fromNodeId"`
	ToNodeID      string `gorm:"size:36;not null;uniqueIndex:uk_edge" json:"toNodeId"`
}

func (Edge) TableName() string { return "path_edges" }

type Goal struct {
	ID     string `gorm:"primaryKey;size:36" json:"id"`
	UserID string `gorm:"size:36;index" json:"userId"`
	Title  string `json:"title"`
}

func (Goal) TableName() string { return "study_goals" }

type Progress struct {
	ID            string `gorm:"primaryKey;size:36" json:"id"`
	PathVersionID string `gorm:"size:36;index" json:"pathVersionId"`
	NodeID        string `gorm:"size:36;index" json:"nodeId"`
	UserID        string `gorm:"size:36;index" json:"userId"`
	Status        string `json:"status"`
}

func (Progress) TableName() string { return "path_progress" }

func Models() []any {
	return []any{&Plan{}, &Item{}, &Path{}, &PathVersion{}, &Node{}, &Edge{}, &Goal{}, &Progress{}}
}

type Service struct {
	db  *gorm.DB
	llm llm.Client
}

func New(db *gorm.DB, client llm.Client) *Service { return &Service{db: db, llm: client} }

func (s *Service) CreatePlan(ctx context.Context, title string, deadline time.Time, items []Item) (Plan, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Plan{}, err
	}
	if strings.TrimSpace(title) == "" || len(items) == 0 {
		return Plan{}, apperr.Invalid("计划和计划项不能为空")
	}
	plan := Plan{ID: xid.New(), UserID: p.UserID, Title: title, Status: "draft", Timezone: tz(p), DeadlineAt: deadline.UTC(), Version: 1, CreatedAt: time.Now().UTC()}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&plan).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].ID = xid.New()
			items[i].PlanID = plan.ID
			if items[i].Status == "" {
				items[i].Status = "pending"
			}
			if err := tx.Create(&items[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return plan, err
}

func (s *Service) ListPlans(ctx context.Context) ([]Plan, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, err
	}
	var rows []Plan
	err = s.db.Where("user_id = ?", p.UserID).Order("created_at desc").Find(&rows).Error
	return rows, err
}

func (s *Service) SetStatus(ctx context.Context, id, status string) (Plan, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Plan{}, err
	}
	var plan Plan
	if err := s.db.Where("id = ? AND user_id = ?", id, p.UserID).First(&plan).Error; err != nil {
		return Plan{}, apperr.NotFound("计划不存在")
	}
	switch status {
	case "active":
		if plan.Status != "draft" && plan.Status != "paused" {
			return plan, nil
		}
	case "paused":
		if plan.Status != "active" {
			return Plan{}, apperr.Conflict("PLAN_STATE", "只有执行中的计划可以暂停")
		}
	case "archived":
	default:
		return Plan{}, apperr.Invalid("状态非法")
	}
	plan.Status = status
	plan.Version++
	return plan, s.db.Save(&plan).Error
}

func (s *Service) Overdue(plan Plan, now time.Time) bool {
	return !now.Before(plan.DeadlineAt)
}

func (s *Service) CreatePath(ctx context.Context, title string, nodes []Node, edges [][2]int) (Path, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Path{}, err
	}
	ids := make([]string, len(nodes))
	for i := range nodes {
		ids[i] = xid.New()
		nodes[i].ID = ids[i]
	}
	edgePairs := make([][2]string, 0, len(edges))
	for _, e := range edges {
		if e[0] < 0 || e[1] < 0 || e[0] >= len(nodes) || e[1] >= len(nodes) {
			return Path{}, apperr.Invalid("依赖节点不存在")
		}
		edgePairs = append(edgePairs, [2]string{ids[e[0]], ids[e[1]]})
	}
	if graph.HasCycle(ids, edgePairs) {
		return Path{}, apperr.Invalid("学习路径存在循环依赖")
	}
	path := Path{ID: xid.New(), UserID: p.UserID, Title: title, Status: "draft", Version: 1}
	ver := PathVersion{ID: xid.New(), PathID: path.ID, VersionNo: 1, Status: "draft"}
	path.ActiveVersionID = ver.ID
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&path).Error; err != nil {
			return err
		}
		if err := tx.Create(&ver).Error; err != nil {
			return err
		}
		for i := range nodes {
			nodes[i].PathVersionID = ver.ID
			nodes[i].Status = "locked"
			if err := tx.Create(&nodes[i]).Error; err != nil {
				return err
			}
		}
		for _, e := range edgePairs {
			if err := tx.Create(&Edge{ID: xid.New(), PathVersionID: ver.ID, FromNodeID: e[0], ToNodeID: e[1]}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return path, err
}

func (s *Service) Activate(ctx context.Context, id string) (Path, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return Path{}, err
	}
	var path Path
	if err := s.db.Where("id = ? AND user_id = ?", id, p.UserID).First(&path).Error; err != nil {
		return Path{}, apperr.NotFound("路径不存在")
	}
	var nodes []Node
	var edges []Edge
	s.db.Where("path_version_id = ?", path.ActiveVersionID).Find(&nodes)
	s.db.Where("path_version_id = ?", path.ActiveVersionID).Find(&edges)
	ids := make([]string, len(nodes))
	pairs := make([][2]string, len(edges))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	for i, e := range edges {
		pairs[i] = [2]string{e.FromNodeID, e.ToNodeID}
	}
	if graph.HasCycle(ids, pairs) {
		return Path{}, apperr.Invalid("学习路径存在循环依赖")
	}
	path.Status = "active"
	s.db.Model(&PathVersion{}).Where("id = ?", path.ActiveVersionID).Update("status", "active")
	return path, s.db.Save(&path).Error
}

func (s *Service) Generate(ctx context.Context, goal string) (Path, error) {
	_, err := s.llm.Complete(ctx, llm.ChatRequest{Module: "planning", Profile: "pathGeneration", PromptVersion: "path-v1", System: "只返回可审核草稿", User: goal})
	if err != nil {
		return Path{}, err
	}
	return s.CreatePath(ctx, "AI 草稿："+goal, []Node{{Title: goal, Required: true}}, nil)
}

func (s *Service) ListPaths(ctx context.Context) ([]Path, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, err
	}
	var rows []Path
	err = s.db.Where("user_id = ?", p.UserID).Order("title asc").Find(&rows).Error
	return rows, err
}

func (s *Service) ArchivePath(ctx context.Context, id string) error {
	p, err := principal.Must(ctx)
	if err != nil {
		return err
	}
	res := s.db.Model(&Path{}).Where("id = ? AND user_id = ?", id, p.UserID).Update("status", "archived")
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperr.NotFound("路径不存在")
	}
	return nil
}

func (s *Service) Detail(ctx context.Context, id string) (map[string]any, error) {
	p, err := principal.Must(ctx)
	if err != nil {
		return nil, err
	}
	var path Path
	if err := s.db.Where("id = ? AND user_id = ?", id, p.UserID).First(&path).Error; err != nil {
		return nil, apperr.NotFound("路径不存在")
	}
	var nodes []Node
	var edges []Edge
	s.db.Where("path_version_id = ?", path.ActiveVersionID).Order("sort_order asc").Find(&nodes)
	s.db.Where("path_version_id = ?", path.ActiveVersionID).Find(&edges)
	return map[string]any{"path": path, "nodes": nodes, "edges": edges}, nil
}

func tz(p principal.Principal) string {
	if p.Timezone == "" {
		return "Asia/Shanghai"
	}
	return p.Timezone
}
