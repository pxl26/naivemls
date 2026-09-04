package e2ee

import (
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestStorePersistsCommits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "e2ee.txt")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.AddProposal(Commit{Epoch: 1, Data: "AQ=="}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddProposal(Commit{Epoch: 2, Data: "Ag=="}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	current, commits, err := reloaded.CommitsAfter(1)
	if err != nil {
		t.Fatal(err)
	}
	if current != 2 || len(commits) != 1 || commits[0].Epoch != 2 || commits[0].Data != "Ag==" {
		t.Fatalf("unexpected reloaded state: current=%d commits=%+v", current, commits)
	}
}

func TestStoreRejectsWrongAndConcurrentEpochs(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "e2ee.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddProposal(Commit{Epoch: 2, Data: "Ag=="}); !errors.Is(err, ErrEpochMismatch) {
		t.Fatalf("expected epoch mismatch, got %v", err)
	}

	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if store.AddProposal(Commit{Epoch: 1, Data: "AQ=="}) == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()

	if accepted.Load() != 1 {
		t.Fatalf("expected exactly one accepted proposal, got %d", accepted.Load())
	}
	if store.CurrentEpoch() != 1 {
		t.Fatalf("expected epoch 1, got %d", store.CurrentEpoch())
	}
}
