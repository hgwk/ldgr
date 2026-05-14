package ids

import (
	"regexp"
	"testing"
)

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestNewProjectID_Format(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := NewProjectID()
		if !hex32.MatchString(id) {
			t.Fatalf("project_id %q does not match 32-char lowercase hex", id)
		}
	}
}

func TestNewProjectID_Unique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewProjectID()
		if seen[id] {
			t.Fatalf("duplicate id at iteration %d: %s", i, id)
		}
		seen[id] = true
	}
}

func TestDisplay(t *testing.T) {
	got := Display("myapp", "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c")
	want := "myapp-9f8a7c"
	if got != want {
		t.Fatalf("Display = %q, want %q", got, want)
	}
}
