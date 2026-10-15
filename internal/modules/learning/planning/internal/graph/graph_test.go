package graph

import "testing"

func TestCycle(t *testing.T) {
	nodes := []string{"a", "b", "c"}
	if HasCycle(nodes, [][2]string{{"a", "b"}, {"b", "c"}}) {
		t.Fatal("unexpected cycle")
	}
	if !HasCycle(nodes, [][2]string{{"a", "b"}, {"b", "a"}}) {
		t.Fatal("expected cycle")
	}
	if !HasCycle(nodes, [][2]string{{"a", "a"}}) {
		t.Fatal("self loop")
	}
}

func TestUnlock(t *testing.T) {
	if Unlocked([]string{"a", "b"}, map[string]bool{"a": true}) {
		t.Fatal("locked")
	}
	if !Unlocked([]string{"a"}, map[string]bool{"a": true}) {
		t.Fatal("unlocked")
	}
}
