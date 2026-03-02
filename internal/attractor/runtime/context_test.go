package runtime

import (
	"sync"
	"testing"
)

func TestContextBasic(t *testing.T) {
	c := NewContext()
	c.Set("key", "value")

	v, ok := c.Get("key")
	if !ok || v != "value" {
		t.Fatal("expected to get key=value")
	}

	_, ok = c.Get("missing")
	if ok {
		t.Fatal("expected missing key to return false")
	}

	c.Delete("key")
	_, ok = c.Get("key")
	if ok {
		t.Fatal("expected deleted key to be gone")
	}
}

func TestContextMerge(t *testing.T) {
	c := NewContext()
	c.Set("a", "1")
	c.Merge(map[string]string{"b": "2", "a": "overwritten"})

	v, _ := c.Get("a")
	if v != "overwritten" {
		t.Fatal("expected merge to overwrite")
	}
	v, _ = c.Get("b")
	if v != "2" {
		t.Fatal("expected merged key b")
	}
}

func TestContextSnapshot(t *testing.T) {
	c := NewContext()
	c.Set("x", "1")
	snap := c.Snapshot()
	c.Set("x", "2")

	if snap["x"] != "1" {
		t.Fatal("snapshot should be a copy")
	}
}

func TestContextClone(t *testing.T) {
	c := NewContext()
	c.Set("x", "1")
	c2 := c.Clone()
	c.Set("x", "2")

	v, _ := c2.Get("x")
	if v != "1" {
		t.Fatal("clone should be independent")
	}
}

func TestContextLen(t *testing.T) {
	c := NewContext()
	if c.Len() != 0 {
		t.Fatal("expected empty")
	}
	c.Set("a", "1")
	c.Set("b", "2")
	if c.Len() != 2 {
		t.Fatal("expected 2")
	}
}

func TestContextConcurrency(t *testing.T) {
	c := NewContext()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "key"
			c.Set(key, "value")
			c.Get(key)
			c.Snapshot()
			c.Delete(key)
		}(i)
	}
	wg.Wait()
}
