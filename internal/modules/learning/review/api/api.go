package api

import "time"

const EventSubmitted = "ReviewSubmitted.v1"

type Card struct {
	ID            string    `json:"id"`
	ContentKey    string    `json:"contentKey"`
	Front         string    `json:"front"`
	Back          string    `json:"back"`
	Status        string    `json:"status"`
	EaseFactor    float64   `json:"easeFactor"`
	IntervalDays  int       `json:"intervalDays"`
	Repetitions   int       `json:"repetitions"`
	NextReviewAt  time.Time `json:"nextReviewAt"`
	Version       int64     `json:"version"`
	Algorithm     string    `json:"algorithm"`
}

type Stats struct {
	Due      int `json:"due"`
	Learning int `json:"learning"`
	Review   int `json:"review"`
	Mastered int `json:"mastered"`
	Paused   int `json:"paused"`
}
