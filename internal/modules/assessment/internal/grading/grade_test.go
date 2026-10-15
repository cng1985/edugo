package grading

import (
	"encoding/json"
	"testing"
)

func TestObjective(t *testing.T) {
	q := Snapshot{Type: "single", ScoreCenti: 1000, Answer: json.RawMessage(`{"choice":"A"}`)}
	score, ok, err := GradeObjective(q, json.RawMessage(`{"choice":"A"}`))
	if err != nil || !ok || score != 1000 {
		t.Fatalf("single %d %v %v", score, ok, err)
	}
	score, ok, _ = GradeObjective(q, json.RawMessage(`{"choice":"B"}`))
	if ok || score != 0 {
		t.Fatal("wrong choice")
	}
	multi := Snapshot{Type: "multiple", ScoreCenti: 500, Answer: json.RawMessage(`{"choices":["A","C"]}`)}
	_, ok, _ = GradeObjective(multi, json.RawMessage(`{"choices":["C","A"]}`))
	if !ok {
		t.Fatal("set equality")
	}
	_, ok, _ = GradeObjective(multi, json.RawMessage(`{"choices":["A"]}`))
	if ok {
		t.Fatal("partial")
	}
	blank := Snapshot{Type: "blank", ScoreCenti: 200, Normalize: true, Answer: json.RawMessage(`{"blanks":["Go"]}`)}
	_, ok, _ = GradeObjective(blank, json.RawMessage(`{"blanks":[" go "]}`))
	if !ok {
		t.Fatal("blank normalize")
	}
}

func TestAIScoreBounds(t *testing.T) {
	if _, err := ValidateAIScore(12, 1000, []float64{12}, []float64{10}); err == nil {
		t.Fatal("over max")
	}
	if _, err := ValidateAIScore(-1, 1000, []float64{-1}, []float64{10}); err == nil {
		t.Fatal("negative")
	}
	if _, err := ValidateAIScore(8, 1000, nil, nil); err == nil {
		t.Fatal("missing items")
	}
	got, err := ValidateAIScore(8.5, 1000, []float64{8.5}, []float64{10})
	if err != nil || got != 850 {
		t.Fatalf("ok %d %v", got, err)
	}
}
