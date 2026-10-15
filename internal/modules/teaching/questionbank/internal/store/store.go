package store

import (
	"errors"
	"time"

	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"gorm.io/gorm"
)

type Question struct {
	ID               string `gorm:"primaryKey;size:36"`
	OwnerID          string `gorm:"size:36;index;not null"`
	OrgID            string `gorm:"size:36;index;not null"`
	Status           string `gorm:"size:32;index;not null"`
	CurrentVersionID string `gorm:"size:36"`
	Version          int64  `gorm:"not null;default:1"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (Question) TableName() string { return "questions" }

type Version struct {
	ID            string `gorm:"primaryKey;size:36"`
	QuestionID    string `gorm:"size:36;not null;uniqueIndex:uk_q_ver"`
	VersionNo     int    `gorm:"not null;uniqueIndex:uk_q_ver"`
	Type          string `gorm:"size:32;not null"`
	Stem          string `gorm:"type:text;not null"`
	Options       string `gorm:"type:text"`
	Answer        string `gorm:"type:text"`
	Explanation   string `gorm:"type:text"`
	Difficulty    string `gorm:"size:32"`
	MaxScoreCenti int
	ReviewStatus  string `gorm:"size:32;index;not null"`
	CreatedAt     time.Time
}

func (Version) TableName() string { return "question_versions" }

type Tag struct {
	ID                string `gorm:"primaryKey;size:36"`
	QuestionVersionID string `gorm:"size:36;index;not null"`
	Tag               string `gorm:"size:64;not null"`
}

func (Tag) TableName() string { return "question_tags" }

type KnowledgeLink struct {
	ID                string `gorm:"primaryKey;size:36"`
	QuestionVersionID string `gorm:"size:36;index;not null"`
	KnowledgePointID  string `gorm:"size:36;index;not null"`
}

func (KnowledgeLink) TableName() string { return "question_knowledge_links" }

type Generation struct {
	ID        string `gorm:"primaryKey;size:36"`
	OwnerID   string `gorm:"size:36;index;not null"`
	Status    string `gorm:"size:32;index;not null"`
	Input     string `gorm:"type:text"`
	Output    string `gorm:"type:text"`
	Error     string `gorm:"size:1024"`
	IdemKey   string `gorm:"size:128;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Generation) TableName() string { return "question_generations" }

func Models() []any {
	return []any{&Question{}, &Version{}, &Tag{}, &KnowledgeLink{}, &Generation{}}
}

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(q Question, v Version) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		q.CurrentVersionID = v.ID
		return tx.Create(&q).Error
	})
}

func (r *Repo) Question(id string) (Question, error) {
	var row Question
	err := r.db.First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Question{}, apperr.NotFound("题目不存在")
	}
	return row, err
}

func (r *Repo) Version(id string) (Version, error) {
	var row Version
	err := r.db.First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Version{}, apperr.NotFound("题目版本不存在")
	}
	return row, err
}

func (r *Repo) LatestNo(questionID string) (int, error) {
	var n int
	err := r.db.Model(&Version{}).Where("question_id = ?", questionID).Select("coalesce(max(version_no),0)").Scan(&n).Error
	return n, err
}

func (r *Repo) SaveDraft(q Question, v Version, expected int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&v).Error; err != nil {
			return err
		}
		res := tx.Model(&Question{}).Where("id = ? AND version = ?", q.ID, expected).Updates(map[string]any{
			"current_version_id": v.ID, "status": q.Status, "version": expected + 1, "updated_at": time.Now().UTC(),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return database.ErrVersionConflict
		}
		return nil
	})
}

func (r *Repo) List(owner, org, typ, status, stem string, offset, limit int) ([]Question, int64, error) {
	q := r.db.Model(&Question{})
	if owner != "" {
		q = q.Where("owner_id = ?", owner)
	}
	if org != "" {
		q = q.Where("org_id = ?", org)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Question
	err := q.Order("created_at desc, id desc").Offset(offset).Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	if typ == "" && stem == "" {
		return rows, total, nil
	}
	filtered := make([]Question, 0, len(rows))
	for _, row := range rows {
		v, e := r.Version(row.CurrentVersionID)
		if e != nil {
			continue
		}
		if typ != "" && v.Type != typ {
			continue
		}
		if stem != "" && !contains(v.Stem, stem) {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered, int64(len(filtered)), nil
}

func (r *Repo) CreateGeneration(row Generation) error { return r.db.Create(&row).Error }
func (r *Repo) SaveGeneration(row Generation) error   { return r.db.Save(&row).Error }

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || len(sub) > 0 && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
