package cart

import "testing"

func TestStoreCounts(t *testing.T) {
	s := NewStore()
	if n, ok := s.Add("s1", "a"); !ok || n != 1 {
		t.Fatalf("first add = %d,%v; want 1,true", n, ok)
	}
	if n, _ := s.Add("s1", "a"); n != 2 {
		t.Fatalf("repeat add of same product = %d, want 2", n)
	}
	if n, _ := s.Add("s1", "b"); n != 3 {
		t.Fatalf("add of second product = %d, want 3", n)
	}
	if s.Count("s1") != 3 {
		t.Fatalf("Count(s1) = %d, want 3", s.Count("s1"))
	}
	if s.Count("unknown") != 0 {
		t.Fatal("unknown session must count 0")
	}
}
