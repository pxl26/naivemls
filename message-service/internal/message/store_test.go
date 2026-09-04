package message

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsAndFiltersMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "messages.txt")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	times := []time.Time{time.UnixMilli(100), time.UnixMilli(200)}
	store.now = func() time.Time {
		now := times[0]
		times = times[1:]
		return now
	}
	if _, err := store.Add(1, "alice", "AQ=="); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(1, "bob", "Ag=="); err != nil {
		t.Fatal(err)
	}

	reloaded, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	messages := reloaded.Between(100, 199)
	if len(messages) != 1 || messages[0].ID != 1 || messages[0].Sender != "alice" {
		t.Fatalf("unexpected messages: %+v", messages)
	}
}
