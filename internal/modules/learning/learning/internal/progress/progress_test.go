package progress

import "testing"

func TestRequiredRate(t *testing.T) {
	got := Rate(1, 2)
	if got == nil || *got != 50 {
		t.Fatalf("rate %v", got)
	}
	if Rate(0, 0) != nil {
		t.Fatal("zero denominator")
	}
}

func TestMergeCoverage(t *testing.T) {
	items := Merge(nil, 0, 1000)
	items = Merge(items, 500, 2000)
	if Coverage(items) != 2000 {
		t.Fatalf("coverage %d", Coverage(items))
	}
	if VideoComplete(89, 100) || !VideoComplete(90, 100) {
		t.Fatal("threshold")
	}
}
