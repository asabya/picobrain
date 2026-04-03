package picobrain

import (
	"context"
	"testing"
	"time"
)

func TestCacheBasicOperations(t *testing.T) {
	cache := NewThoughtCache(3)

	thought1 := Thought{ID: "1", Content: "first", CreatedAt: time.Now()}
	thought2 := Thought{ID: "2", Content: "second", CreatedAt: time.Now()}
	thought3 := Thought{ID: "3", Content: "third", CreatedAt: time.Now()}

	cache.Put(thought1)
	cache.Put(thought2)
	cache.Put(thought3)

	if cache.Len() != 3 {
		t.Errorf("expected cache size 3, got %d", cache.Len())
	}

	got, found := cache.Get("1")
	if !found {
		t.Error("expected to find thought 1")
	}
	if got.Content != "first" {
		t.Errorf("expected content 'first', got '%s'", got.Content)
	}

	cache.Remove("1")
	if cache.Len() != 2 {
		t.Errorf("expected cache size 2 after remove, got %d", cache.Len())
	}

	_, found = cache.Get("1")
	if found {
		t.Error("expected thought 1 to be removed")
	}
}

func TestCacheLRUEviction(t *testing.T) {
	cache := NewThoughtCache(2)

	thought1 := Thought{ID: "1", Content: "first", CreatedAt: time.Now()}
	thought2 := Thought{ID: "2", Content: "second", CreatedAt: time.Now()}
	thought3 := Thought{ID: "3", Content: "third", CreatedAt: time.Now()}

	cache.Put(thought1)
	cache.Put(thought2)

	cache.Get("1")

	cache.Put(thought3)

	if cache.Len() != 2 {
		t.Errorf("expected cache size 2, got %d", cache.Len())
	}

	_, found := cache.Get("1")
	if !found {
		t.Error("expected thought 1 to still be in cache (was recently used)")
	}

	_, found = cache.Get("2")
	if found {
		t.Error("expected thought 2 to be evicted (least recently used)")
	}

	_, found = cache.Get("3")
	if !found {
		t.Error("expected thought 3 to be in cache")
	}
}

func TestCacheGetAll(t *testing.T) {
	cache := NewThoughtCache(3)

	thought1 := Thought{ID: "1", Content: "first", CreatedAt: time.Now()}
	thought2 := Thought{ID: "2", Content: "second", CreatedAt: time.Now()}
	thought3 := Thought{ID: "3", Content: "third", CreatedAt: time.Now()}

	cache.Put(thought1)
	cache.Put(thought2)
	cache.Put(thought3)

	cache.Get("1")

	all := cache.GetAll()
	if len(all) != 3 {
		t.Fatalf("expected 3 thoughts, got %d", len(all))
	}

	if all[0].ID != "1" {
		t.Errorf("expected first thought to be ID '1', got '%s'", all[0].ID)
	}
}

func TestCacheGetRecent(t *testing.T) {
	cache := NewThoughtCache(5)

	for i := 1; i <= 5; i++ {
		cache.Put(Thought{ID: string(rune('0' + i)), Content: "thought", CreatedAt: time.Now()})
	}

	recent := cache.GetRecent(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent thoughts, got %d", len(recent))
	}

	if recent[0].ID != "5" || recent[1].ID != "4" || recent[2].ID != "3" {
		t.Error("expected most recent thoughts in order")
	}
}

func TestCacheClear(t *testing.T) {
	cache := NewThoughtCache(3)

	cache.Put(Thought{ID: "1", Content: "first", CreatedAt: time.Now()})
	cache.Put(Thought{ID: "2", Content: "second", CreatedAt: time.Now()})

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("expected empty cache after clear, got %d", cache.Len())
	}

	_, found := cache.Get("1")
	if found {
		t.Error("expected cache to be empty after clear")
	}
}

func TestCacheDefaultSize(t *testing.T) {
	cache := NewThoughtCache(0)

	for i := 1; i <= 55; i++ {
		cache.Put(Thought{ID: string(rune('0' + (i % 10))), Content: "thought", CreatedAt: time.Now()})
	}

	if cache.Len() > 50 {
		t.Errorf("expected cache size <= 50 (default), got %d", cache.Len())
	}
}

func TestCacheUpdateExisting(t *testing.T) {
	cache := NewThoughtCache(3)

	thought1 := Thought{ID: "1", Content: "original", CreatedAt: time.Now()}
	cache.Put(thought1)

	thought1Updated := Thought{ID: "1", Content: "updated", CreatedAt: time.Now()}
	cache.Put(thought1Updated)

	got, found := cache.Get("1")
	if !found {
		t.Fatal("expected to find thought 1")
	}
	if got.Content != "updated" {
		t.Errorf("expected updated content, got '%s'", got.Content)
	}

	if cache.Len() != 1 {
		t.Errorf("expected cache size 1 after update, got %d", cache.Len())
	}
}

func TestBrainCacheIntegration(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	thought1 := &Thought{Content: "first thought", Source: "test"}
	thought2 := &Thought{Content: "second thought", Source: "test"}

	if err := brain.Store(ctx, thought1); err != nil {
		t.Fatalf("Store thought1: %v", err)
	}
	if err := brain.Store(ctx, thought2); err != nil {
		t.Fatalf("Store thought2: %v", err)
	}

	if brain.cache.Len() != 2 {
		t.Errorf("expected cache size 2, got %d", brain.cache.Len())
	}

	got, err := brain.Get(ctx, thought1.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "first thought" {
		t.Errorf("expected 'first thought', got '%s'", got.Content)
	}

	if err := brain.Delete(ctx, thought1.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if brain.cache.Len() != 1 {
		t.Errorf("expected cache size 1 after delete, got %d", brain.cache.Len())
	}

	_, found := brain.cache.Get(thought1.ID)
	if found {
		t.Error("expected thought1 to be removed from cache after delete")
	}
}

func TestBrainCacheListRecent(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		thought := &Thought{Content: "thought", Source: "test"}
		if err := brain.Store(ctx, thought); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	since := time.Now().Add(-1 * time.Hour)
	_, err := brain.ListRecent(ctx, since, 3, "")
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}

	if brain.cache.Len() < 3 {
		t.Errorf("expected cache to have at least 3 items after ListRecent, got %d", brain.cache.Len())
	}
}

func TestBrainCacheReflect(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	obs1 := &Thought{Content: "Obs 1", Type: "observation", Source: "test"}
	obs2 := &Thought{Content: "Obs 2", Type: "observation", Source: "test"}
	brain.Store(ctx, obs1)
	brain.Store(ctx, obs2)

	newThought := &Thought{Content: "Consolidated", Type: "insight", Source: "test"}
	_, err := brain.Reflect(ctx, []string{obs1.ID, obs2.ID}, []*Thought{newThought})
	if err != nil {
		t.Fatalf("Reflect: %v", err)
	}

	_, found := brain.cache.Get(obs1.ID)
	if found {
		t.Error("expected obs1 to be removed from cache after reflect")
	}
	_, found = brain.cache.Get(obs2.ID)
	if found {
		t.Error("expected obs2 to be removed from cache after reflect")
	}

	_, found = brain.cache.Get(newThought.ID)
	if !found {
		t.Error("expected new thought to be in cache after reflect")
	}
}

func TestBrainGetRecent(t *testing.T) {
	brain := testBrain(t)
	ctx := context.Background()

	thoughts := []*Thought{
		{Content: "first", Source: "test"},
		{Content: "second", Source: "test"},
		{Content: "third", Source: "test"},
	}

	for _, th := range thoughts {
		if err := brain.Store(ctx, th); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	recent := brain.GetRecent(2)
	if len(recent) != 2 {
		t.Fatalf("expected 2 recent thoughts, got %d", len(recent))
	}

	if recent[0].Content != "third" {
		t.Errorf("expected most recent thought first, got '%s'", recent[0].Content)
	}
}
