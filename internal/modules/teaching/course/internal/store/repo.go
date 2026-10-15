package store

import (
	"errors"
	"time"

	"github.com/cng1985/edugo/internal/platform/database"
	"github.com/cng1985/edugo/internal/platform/eventbus"
	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/xid"
	"gorm.io/gorm"
)

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) CreateDraft(course Course, version Version) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		course.DraftVersionID = version.ID
		return tx.Create(&course).Error
	})
}

func (r *Repo) Course(id string) (Course, error) {
	var row Course
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Course{}, apperr.NotFound("课程不存在")
	}
	return row, err
}

func (r *Repo) Version(id string) (Version, error) {
	var row Version
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Version{}, apperr.NotFound("课程版本不存在")
	}
	return row, err
}

func (r *Repo) Lesson(id string) (Lesson, error) {
	var row Lesson
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Lesson{}, apperr.NotFound("课时不存在")
	}
	return row, err
}

func (r *Repo) Outline(versionID string) ([]Chapter, []Lesson, error) {
	var chapters []Chapter
	if err := r.db.Where("course_version_id = ?", versionID).Order("sort_order asc, id asc").Find(&chapters).Error; err != nil {
		return nil, nil, err
	}
	var lessons []Lesson
	err := r.db.Where("course_version_id = ?", versionID).Order("sort_order asc, id asc").Find(&lessons).Error
	return chapters, lessons, err
}

func (r *Repo) SaveCourse(row Course, expected int64) error {
	res := r.db.Model(&Course{}).Where("id = ? AND version = ?", row.ID, expected).Updates(map[string]any{
		"title": row.Title, "summary": row.Summary, "status": row.Status, "draft_version_id": row.DraftVersionID,
		"published_version_id": row.PublishedVersionID, "version": expected + 1, "updated_at": time.Now().UTC(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return database.ErrVersionConflict
	}
	return nil
}

func (r *Repo) SaveVersion(row Version) error { return r.db.Save(&row).Error }

func (r *Repo) NextVersionNo(courseID string) (int, error) {
	var maxNo int
	err := r.db.Model(&Version{}).Where("course_id = ?", courseID).Select("coalesce(max(version_no),0)").Scan(&maxNo).Error
	return maxNo + 1, err
}

func (r *Repo) Fork(course Course, expected int64) (Course, Version, error) {
	var created Version
	err := r.db.Transaction(func(tx *gorm.DB) error {
		src, err := loadVersion(tx, course.DraftVersionID)
		if err != nil {
			return err
		}
		var maxNo int
		if err := tx.Model(&Version{}).Where("course_id = ?", course.ID).Select("coalesce(max(version_no),0)").Scan(&maxNo).Error; err != nil {
			return err
		}
		created = Version{
			ID: xid.New(), CourseID: course.ID, VersionNo: maxNo + 1, Status: "draft", Title: src.Title,
			Summary: src.Summary, SummaryDraft: src.SummaryDraft, PolicyVersion: "policy-v1", CreatedAt: time.Now().UTC(),
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		var chapters []Chapter
		if err := tx.Where("course_version_id = ?", src.ID).Find(&chapters).Error; err != nil {
			return err
		}
		chapterMap := map[string]string{}
		for _, ch := range chapters {
			nid := xid.New()
			chapterMap[ch.ID] = nid
			ch.ID = nid
			ch.CourseVersionID = created.ID
			if err := tx.Create(&ch).Error; err != nil {
				return err
			}
		}
		var lessons []Lesson
		if err := tx.Where("course_version_id = ?", src.ID).Find(&lessons).Error; err != nil {
			return err
		}
		for _, lesson := range lessons {
			old := lesson.ID
			lesson.ID = xid.New()
			lesson.CourseVersionID = created.ID
			lesson.ChapterID = chapterMap[lesson.ChapterID]
			if err := tx.Create(&lesson).Error; err != nil {
				return err
			}
			if lesson.ResourceID != "" {
				if err := tx.Create(&LessonResource{ID: xid.New(), LessonID: lesson.ID, ResourceID: lesson.ResourceID, Kind: lesson.LessonType}).Error; err != nil {
					return err
				}
			}
			_ = old
		}
		res := tx.Model(&Course{}).Where("id = ? AND version = ?", course.ID, expected).Updates(map[string]any{
			"draft_version_id": created.ID, "version": expected + 1, "updated_at": time.Now().UTC(),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return database.ErrVersionConflict
		}
		course.DraftVersionID = created.ID
		course.Version = expected + 1
		return nil
	})
	return course, created, err
}

func (r *Repo) AddChapter(ch Chapter, courseID string, expected int64) (Chapter, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := bump(tx, courseID, expected); err != nil {
			return err
		}
		return tx.Create(&ch).Error
	})
	return ch, err
}

func (r *Repo) SaveChapter(ch Chapter, courseID string, expected int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := bump(tx, courseID, expected); err != nil {
			return err
		}
		return tx.Model(&Chapter{}).Where("id = ?", ch.ID).Updates(map[string]any{"title": ch.Title, "sort_order": ch.SortOrder}).Error
	})
}

func (r *Repo) DeleteChapter(id, courseID string, expected int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := bump(tx, courseID, expected); err != nil {
			return err
		}
		if err := tx.Where("chapter_id = ?", id).Delete(&Lesson{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&Chapter{}).Error
	})
}

func (r *Repo) AddLesson(lesson Lesson, courseID string, expected int64) (Lesson, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := bump(tx, courseID, expected); err != nil {
			return err
		}
		if err := tx.Create(&lesson).Error; err != nil {
			return err
		}
		if lesson.ResourceID != "" {
			return tx.Create(&LessonResource{ID: xid.New(), LessonID: lesson.ID, ResourceID: lesson.ResourceID, Kind: lesson.LessonType}).Error
		}
		return nil
	})
	return lesson, err
}

func (r *Repo) SaveLesson(lesson Lesson, courseID string, expected int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := bump(tx, courseID, expected); err != nil {
			return err
		}
		if err := tx.Model(&Lesson{}).Where("id = ?", lesson.ID).Updates(map[string]any{
			"title": lesson.Title, "lesson_type": lesson.LessonType, "required": lesson.Required,
			"duration_sec": lesson.DurationSec, "content_text": lesson.ContentText, "resource_id": lesson.ResourceID, "sort_order": lesson.SortOrder,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("lesson_id = ?", lesson.ID).Delete(&LessonResource{}).Error; err != nil {
			return err
		}
		if lesson.ResourceID != "" {
			return tx.Create(&LessonResource{ID: xid.New(), LessonID: lesson.ID, ResourceID: lesson.ResourceID, Kind: lesson.LessonType}).Error
		}
		return nil
	})
}

func (r *Repo) DeleteLesson(id, courseID string, expected int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := bump(tx, courseID, expected); err != nil {
			return err
		}
		if err := tx.Where("lesson_id = ?", id).Delete(&LessonResource{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&Lesson{}).Error
	})
}

func (r *Repo) Publish(course Course, version Version, expected int64, ev eventbus.Event) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Model(&Version{}).Where("id = ?", version.ID).Updates(map[string]any{"status": "published", "published_at": now, "title": version.Title, "summary": version.Summary}).Error; err != nil {
			return err
		}
		res := tx.Model(&Course{}).Where("id = ? AND version = ?", course.ID, expected).Updates(map[string]any{
			"status": "published", "title": version.Title, "summary": version.Summary, "published_version_id": version.ID,
			"draft_version_id": version.ID, "version": expected + 1, "updated_at": now,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return database.ErrVersionConflict
		}
		return eventbus.Insert(tx, ev)
	})
}

func (r *Repo) Transition(course Course, versionID, courseStatus, versionStatus, reason string, expected int64, ev *eventbus.Event) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Model(&Version{}).Where("id = ?", versionID).Updates(map[string]any{"status": versionStatus, "withdraw_reason": reason}).Error; err != nil {
			return err
		}
		res := tx.Model(&Course{}).Where("id = ? AND version = ?", course.ID, expected).Updates(map[string]any{
			"status": courseStatus, "version": expected + 1, "updated_at": now,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return database.ErrVersionConflict
		}
		if ev != nil {
			return eventbus.Insert(tx, *ev)
		}
		return nil
	})
}

func (r *Repo) List(ownerID, orgID, status, title string, offset, limit int, includeOwn bool) ([]Course, int64, error) {
	q := r.db.Model(&Course{})
	if orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if includeOwn && ownerID != "" {
		if status != "" {
			q = q.Where("(owner_id = ? OR status = ?) AND status = ?", ownerID, "published", status)
		} else {
			q = q.Where("owner_id = ? OR status = ?", ownerID, "published")
		}
	} else {
		if status != "" {
			q = q.Where("status = ?", status)
		}
		if ownerID != "" {
			q = q.Where("owner_id = ?", ownerID)
		}
	}
	if title != "" {
		q = q.Where("title LIKE ?", "%"+title+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Course
	err := q.Order("created_at desc, id desc").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}

func (r *Repo) RequiredLessonCount(versionID string) (total, withResource int, err error) {
	var lessons []Lesson
	if err = r.db.Where("course_version_id = ?", versionID).Find(&lessons).Error; err != nil {
		return
	}
	return len(lessons), 0, nil
}

func (r *Repo) LessonsOf(versionID string) ([]Lesson, error) {
	var lessons []Lesson
	err := r.db.Where("course_version_id = ?", versionID).Find(&lessons).Error
	return lessons, err
}

func bump(tx *gorm.DB, courseID string, expected int64) error {
	res := tx.Model(&Course{}).Where("id = ? AND version = ?", courseID, expected).Updates(map[string]any{
		"version": expected + 1, "updated_at": time.Now().UTC(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return database.ErrVersionConflict
	}
	return nil
}

func loadVersion(tx *gorm.DB, id string) (Version, error) {
	var row Version
	err := tx.Where("id = ?", id).First(&row).Error
	return row, err
}
