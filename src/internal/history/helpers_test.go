package history

import (
	"testing"
	"time"
)

func mustEntry(t *testing.T, fields EntryFields) Entry {
	t.Helper()
	entry, err := NewEntry(fields)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return entry
}

func mustOpen(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return store
}

func mustAppend(t *testing.T, store *Store, fields EntryFields) {
	t.Helper()
	if err := store.Append(mustEntry(t, fields)); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

func mustExclusions(t *testing.T, h History, now time.Time, tag string) Exclusions {
	t.Helper()
	got, err := h.Exclusions(now, tag)
	if err != nil {
		t.Fatalf("Exclusions: %v", err)
	}
	return got
}

func mustCompute(t *testing.T, entries []Entry, now time.Time, tag string) Exclusions {
	t.Helper()
	got, err := computeExclusions(entries, now, tag)
	if err != nil {
		t.Fatalf("computeExclusions: %v", err)
	}
	return got
}

func mustCountAsFailure(t *testing.T, f EntryFields) bool {
	t.Helper()
	got, err := countsAsFailure(f)
	if err != nil {
		t.Fatalf("countsAsFailure: %v", err)
	}
	return got
}
