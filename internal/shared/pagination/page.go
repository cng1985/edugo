package pagination

import "github.com/cng1985/edugo/internal/shared/apperr"

type Sort struct {
	Field string `json:"field"`
	Order string `json:"order"`
}

type Request struct {
	Page int    `json:"page"`
	Size int    `json:"size"`
	Sort []Sort `json:"sort"`
}

func Normalize(page, size, defSize, maxSize int) (int, int, error) {
	if page == 0 {
		page = 1
	}
	if size == 0 {
		size = defSize
	}
	if page < 1 || size < 1 || size > maxSize {
		return 0, 0, apperr.Invalid("分页参数超出范围")
	}
	return page, size, nil
}

func Offset(page, size int) int { return (page - 1) * size }
