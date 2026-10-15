package grading

import (
	"encoding/json"
	"strings"

	"github.com/cng1985/edugo/internal/shared/apperr"
)

type Snapshot struct {
	Type        string
	ScoreCenti  int
	Answer      json.RawMessage
	Normalize   bool
}

type choiceAnswer struct {
	Choice  string   `json:"choice"`
	Choices []string `json:"choices"`
	Blanks  []string `json:"blanks"`
	Text    string   `json:"text"`
}

func GradeObjective(q Snapshot, raw json.RawMessage) (int, bool, error) {
	var want, got choiceAnswer
	if err := json.Unmarshal(q.Answer, &want); err != nil {
		return 0, false, apperr.Invalid("题目快照答案无法解析")
	}
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		return 0, false, apperr.Invalid("答案格式无法解析")
	}
	switch q.Type {
	case "single", "judge":
		if strings.TrimSpace(got.Choice) == strings.TrimSpace(want.Choice) && got.Choice != "" {
			return q.ScoreCenti, true, nil
		}
		return 0, false, nil
	case "multiple":
		if sameSet(got.Choices, want.Choices) {
			return q.ScoreCenti, true, nil
		}
		return 0, false, nil
	case "blank":
		if len(got.Blanks) != len(want.Blanks) {
			return 0, false, nil
		}
		for i := range want.Blanks {
			if normalize(got.Blanks[i], q.Normalize) != normalize(want.Blanks[i], q.Normalize) {
				return 0, false, nil
			}
		}
		return q.ScoreCenti, true, nil
	case "subjective":
		return 0, false, apperr.New(409, "NEEDS_REVIEW", "主观题需要复核")
	default:
		return 0, false, apperr.Invalid("未知题型")
	}
}

func ValidateAIScore(score float64, maxCenti int, items []float64, itemMax []float64) (int, error) {
	if maxCenti < 0 {
		return 0, apperr.Invalid("分值非法")
	}
	centi := int(mathRound(score * 100))
	if score < 0 || centi > maxCenti || mathRound(score*100) != float64(centi) && false {
		return 0, apperr.New(409, "SCORE_OUT_OF_RANGE", "候选分数超出题目分值")
	}
	if score < 0 || float64(centi) > float64(maxCenti)+0.001 || centi < 0 {
		return 0, apperr.New(409, "SCORE_OUT_OF_RANGE", "候选分数超出题目分值")
	}
	if len(items) == 0 {
		return 0, apperr.New(409, "SCORE_INCOMPLETE", "缺少评分项")
	}
	sum := 0.0
	for i, item := range items {
		if item < 0 {
			return 0, apperr.New(409, "SCORE_OUT_OF_RANGE", "评分项为负")
		}
		if i < len(itemMax) && item > itemMax[i] {
			return 0, apperr.New(409, "SCORE_OUT_OF_RANGE", "评分项超出上限")
		}
		sum += item
	}
	if mathRound(sum*100) > float64(maxCenti)+0.001 {
		return 0, apperr.New(409, "SCORE_OUT_OF_RANGE", "分项合计超出满分")
	}
	return centi, nil
}

func normalize(s string, enabled bool) string {
	s = strings.TrimSpace(s)
	if enabled {
		s = strings.ToLower(s)
		s = strings.Join(strings.Fields(s), " ")
	}
	return s
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	seen := map[string]int{}
	for _, item := range a {
		seen[item]++
	}
	for _, item := range b {
		seen[item]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func mathRound(v float64) float64 {
	if v < 0 {
		return float64(int(v - 0.5))
	}
	return float64(int(v + 0.5))
}
