package dns

import (
	"sync"
	"testing"
	"time"
)

func TestFileBindingStoreSaveLoadAndList(t *testing.T) {
	store := NewFileBindingStore(t.TempDir())

	binding := Binding{
		NodeUUID:     "node-1",
		DNSLabel:     "dl-001",
		DNSName:      "dl-001.example.com",
		LastPublicIP: "1.1.1.1",
		UpdatedAt:    time.Now().UTC(),
	}
	if err := store.Save(binding); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := store.Load("node-1")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected binding, got nil")
	}
	if loaded.DNSName != "dl-001.example.com" {
		t.Fatalf("expected dl-001.example.com, got %s", loaded.DNSName)
	}

	items, err := store.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestFileBindingStoreSaveOverwritesCurrentBinding(t *testing.T) {
	store := NewFileBindingStore(t.TempDir())

	if err := store.Save(Binding{
		NodeUUID:     "node-1",
		DNSLabel:     "dl-001",
		DNSName:      "dl-001.example.com",
		LastPublicIP: "1.1.1.1",
		UpdatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("initial Save returned error: %v", err)
	}
	if err := store.Save(Binding{
		NodeUUID:     "node-1",
		DNSLabel:     "dl-009",
		DNSName:      "dl-009.example.com",
		LastPublicIP: "9.9.9.9",
		UpdatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("overwrite Save returned error: %v", err)
	}

	loaded, err := store.Load("node-1")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected binding, got nil")
	}
	if loaded.DNSLabel != "dl-009" || loaded.DNSName != "dl-009.example.com" {
		t.Fatalf("expected overwritten binding dl-009.example.com, got %+v", loaded)
	}
}

func TestFileBindingStoreReserveNextSequenceUsesExistingBindings(t *testing.T) {
	store := NewFileBindingStore(t.TempDir())

	if err := store.Save(Binding{
		NodeUUID:     "node-1",
		DNSLabel:     "dl-007",
		DNSName:      "dl-007.example.com",
		LastPublicIP: "7.7.7.7",
		UpdatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	first, err := store.ReserveNextSequence("dl-")
	if err != nil {
		t.Fatalf("first ReserveNextSequence returned error: %v", err)
	}
	second, err := store.ReserveNextSequence("dl-")
	if err != nil {
		t.Fatalf("second ReserveNextSequence returned error: %v", err)
	}
	if first != 8 || second != 9 {
		t.Fatalf("expected 8 then 9, got %d then %d", first, second)
	}
}

func TestFileBindingStoreReserveNextSequenceIsUniqueConcurrently(t *testing.T) {
	store := NewFileBindingStore(t.TempDir())

	const workers = 16
	results := make(chan int, workers)
	errs := make(chan error, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := store.ReserveNextSequence("dl-")
			if err != nil {
				errs <- err
				return
			}
			results <- value
		}()
	}

	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("ReserveNextSequence returned error: %v", err)
		}
	}

	seen := make(map[int]struct{}, workers)
	for value := range results {
		if _, exists := seen[value]; exists {
			t.Fatalf("duplicate reserved value: %d", value)
		}
		seen[value] = struct{}{}
	}
	if len(seen) != workers {
		t.Fatalf("expected %d unique values, got %d", workers, len(seen))
	}
}
