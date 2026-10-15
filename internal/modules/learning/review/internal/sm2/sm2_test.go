package sm2

import (
	"math"
	"testing"
	"time"
)

func TestSamples(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, loc)
	cases := []struct {
		name string
		prev State
		q    int
		days int
		reps int
		ef   float64
		st   string
	}{
		{"first", State{EaseFactor: 2.5, Repetitions: 0, IntervalDays: 0}, 4, 1, 1, 2.5, "review"},
		{"second", State{EaseFactor: 2.5, Repetitions: 1, IntervalDays: 1, SuccessStreak: 1, Status: "review"}, 4, 6, 2, 2.5, "review"},
		{"third", State{EaseFactor: 2.5, Repetitions: 2, IntervalDays: 6, SuccessStreak: 2, Status: "review"}, 4, 15, 3, 2.5, "mastered"},
		{"fail", State{EaseFactor: 1.3, Repetitions: 3, IntervalDays: 15, Status: "mastered", SuccessStreak: 3}, 0, 1, 0, 1.3, "learning"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, next, err := Review(tc.prev, tc.q, at, loc)
			if err != nil {
				t.Fatal(err)
			}
			if got.IntervalDays != tc.days || got.Repetitions != tc.reps || got.Status != tc.st {
				t.Fatalf("state %+v", got)
			}
			if math.Abs(got.EaseFactor-tc.ef) > 0.0001 {
				t.Fatalf("ef %v", got.EaseFactor)
			}
			want := at.AddDate(0, 0, tc.days).UTC()
			if !next.Equal(want) {
				t.Fatalf("next %s want %s", next, want)
			}
		})
	}
}

func TestInvalidQuality(t *testing.T) {
	prev := Initial()
	for _, q := range []int{-1, 6} {
		got, _, err := Review(prev, q, time.Now(), time.UTC)
		if err == nil {
			t.Fatal("expected error")
		}
		if got != prev {
			t.Fatalf("state changed %+v", got)
		}
	}
}
