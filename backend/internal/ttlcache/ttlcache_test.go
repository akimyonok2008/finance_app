package ttlcache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCache_MissThenHit(t *testing.T) {
	c := New[int](time.Minute)

	_, ok := c.Get("a")
	assert.False(t, ok)

	c.Set("a", 42)
	v, ok := c.Get("a")
	assert.True(t, ok)
	assert.Equal(t, 42, v)
}

func TestCache_ExpiresAfterTTL(t *testing.T) {
	c := New[string](1 * time.Millisecond)
	c.Set("k", "v")
	time.Sleep(5 * time.Millisecond)

	_, ok := c.Get("k")
	assert.False(t, ok, "an expired entry must not be served")
}

func TestCache_Invalidate(t *testing.T) {
	c := New[int](time.Minute)
	c.Set("a", 1)
	c.Invalidate("a")

	_, ok := c.Get("a")
	assert.False(t, ok)
}

func TestCache_DifferentKeysAreIndependent(t *testing.T) {
	c := New[int](time.Minute)
	c.Set("a", 1)
	c.Set("b", 2)

	va, _ := c.Get("a")
	vb, _ := c.Get("b")
	assert.Equal(t, 1, va)
	assert.Equal(t, 2, vb)
}
