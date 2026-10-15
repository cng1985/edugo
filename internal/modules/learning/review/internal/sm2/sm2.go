package sm2

import (
	"math"
	"time"

	"github.com/cng1985/edugo/internal/shared/apperr"
)

const Version = "sm2-edugo-v1"

type State struct {
	EaseFactor    float64
	Repetitions   int
	IntervalDays  int
	Status        string
	SuccessStreak int
}

func Initial() State {
	return State{EaseFactor: 2.5, Repetitions: 0, IntervalDays: 0, Status: "learning", SuccessStreak: 0}
}

func Review(prev State, quality int, reviewedAt time.Time, loc *time.Location) (State, time.Time, error) {
	if quality < 0 || quality > 5 {
		return prev, time.Time{}, apperr.Invalid("评分必须是 0 到 5 的整数")
	}
	if loc == nil {
		loc = time.UTC
	}
	next := prev
	if quality < 3 {
		next.IntervalDays = 1
		next.Repetitions = 0
		next.Status = "learning"
		next.SuccessStreak = 0
	} else if prev.Repetitions == 0 {
		next.IntervalDays = 1
		next.Repetitions = 1
	} else if prev.Repetitions == 1 {
		next.IntervalDays = 6
		next.Repetitions = 2
	} else {
		next.IntervalDays = int(math.Ceil(float64(prev.IntervalDays) * prev.EaseFactor))
		if next.IntervalDays < 1 {
			next.IntervalDays = 1
		}
		next.Repetitions = prev.Repetitions + 1
	}
	if quality >= 3 {
		next.SuccessStreak = prev.SuccessStreak + 1
		next.Status = "review"
		if next.SuccessStreak >= 3 && quality >= 4 {
			next.Status = "mastered"
		}
	}
	delta := 0.1 - float64(5-quality)*(0.08+float64(5-quality)*0.02)
	next.EaseFactor = math.Max(1.3, prev.EaseFactor+delta)
	local := reviewedAt.In(loc)
	nextLocal := time.Date(local.Year(), local.Month(), local.Day()+next.IntervalDays, local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), loc)
	return next, nextLocal.UTC(), nil
}
