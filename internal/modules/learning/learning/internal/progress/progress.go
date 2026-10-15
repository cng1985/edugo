package progress

import (
	"math"
	"sort"
)

type Interval struct {
	Start int64
	End   int64
}

func Merge(items []Interval, start, end int64) []Interval {
	if end <= start {
		return items
	}
	items = append(items, Interval{Start: start, End: end})
	sort.Slice(items, func(i, j int) bool { return items[i].Start < items[j].Start })
	out := []Interval{items[0]}
	for _, iv := range items[1:] {
		last := &out[len(out)-1]
		if iv.Start <= last.End {
			if iv.End > last.End {
				last.End = iv.End
			}
			continue
		}
		out = append(out, iv)
	}
	return out
}

func Coverage(items []Interval) int64 {
	var total int64
	for _, iv := range items {
		if iv.End > iv.Start {
			total += iv.End - iv.Start
		}
	}
	return total
}

func Rate(done, total int) *float64 {
	if total <= 0 {
		return nil
	}
	value := math.Round((float64(done)/float64(total))*10000) / 100
	return &value
}

func VideoComplete(coveredMs, durationMs int64) bool {
	if durationMs <= 0 {
		return false
	}
	return coveredMs*100 >= durationMs*90
}
