package cmd

import (
	"bytes"
	"testing"
)

func TestUnregister_ByPath(t *testing.T) {
	target, store := mustInit(t)
	if code := RunUnregisterCLI([]string{"--path", target}, store, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("unregister failed")
	}
	r, _ := store.Load()
	if len(r.Projects) != 0 {
		t.Fatalf("expected unregistered, got %+v", r)
	}
}

func TestUnregister_ByID(t *testing.T) {
	_, store := mustInit(t)
	r, _ := store.Load()
	id := r.Projects[0].ProjectID
	if code := RunUnregisterCLI([]string{"--project-id", id}, store, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("unregister failed")
	}
	r, _ = store.Load()
	if len(r.Projects) != 0 {
		t.Fatalf("expected unregistered")
	}
}

func TestUnregister_RequiresOneFlag(t *testing.T) {
	_, store := mustInit(t)
	errb := &bytes.Buffer{}
	code := RunUnregisterCLI([]string{}, store, &bytes.Buffer{}, errb)
	if code == 0 {
		t.Fatalf("expected error without flag")
	}
}
