// Copyright (C) 2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package containers

import (
	"context"
	"fmt"
	"sync"
)

// NewLRUCache returns a new thread-safe Cache with a simple
// Least-Recently-Used eviction policy.
//
// It is invalid (runtime-panic) to call NewLRUCache with a
// non-positive capacity or a nil source.
//
//nolint:predeclared // 'cap' is the best name for it.
func NewLRUCache[K comparable, V any](cap int, src Source[K, V]) Cache[K, V] {
	if cap <= 0 {
		panic(fmt.Errorf("containers.NewLRUCache: invalid capacity: %v", cap))
	}
	if src == nil {
		panic(fmt.Errorf("containers.NewLRUCache: nil source"))
	}
	ret := &lruCache[K, V]{
		cap: cap,
		src: src,

		byName: make(map[K]*LinkedListEntry[lruEntry[K, V]], cap),
	}
	for i := 0; i < cap; i++ {
		ret.unused.Store(new(LinkedListEntry[lruEntry[K, V]]))
	}
	return ret
}

type lruEntry[K comparable, V any] struct {
	key K
	val V

	refs int
	del  chan struct{} // non-nil if a delete is waiting on .refs to drop to zero
}

type lruCache[K comparable, V any] struct {
	cap int
	src Source[K, V]

	mu sync.Mutex

	// Pinned entries are in .byName, but not in any LinkedList.
	unused    LinkedList[lruEntry[K, V]]
	evictable LinkedList[lruEntry[K, V]] // only entries with .refs==0
	byName    map[K]*LinkedListEntry[lruEntry[K, V]]

	waiters LinkedList[chan struct{}]
}

// Blocking primitives /////////////////////////////////////////////////////////

// waitForAvail is called before storing something into the cache.
// This is nescessary because if the cache is full and all entries are
// pinned, then we won't have to store the entry until something gets
// unpinned ("Release()d").
func (c *lruCache[K, V]) waitForAvail() {
	if !(c.unused.IsEmpty() && c.evictable.IsEmpty()) {
		// There is already an available `lruEntry` that we
		// can either use or evict.
		return
	}
	ch := make(chan struct{})
	c.waiters.Store(&LinkedListEntry[chan struct{}]{Value: ch})
	c.mu.Unlock()
	<-ch // receive the lock from .Release()
	if c.unused.IsEmpty() && c.evictable.IsEmpty() {
		panic(fmt.Errorf("should not happen: waitForAvail is returning, but nothing is available"))
	}
}

// unlockAndNotifyAvail is called when an entry becomes unused or
// evictable, and wakes up the highest-priority .waitForAvail() waiter
// (if there is one).
func (c *lruCache[K, V]) unlockAndNotifyAvail() {
	waiter := c.waiters.Oldest
	if waiter == nil {
		c.mu.Unlock()
		return
	}
	c.waiters.Delete(waiter)
	// We don't actually unlock, we're "transferring" the lock to
	// the waiter.
	close(waiter.Value)
}

// Calling .Delete(k) on an entry that is pinned needs to block until
// the entry is no longer pinned.
func (c *lruCache[K, V]) unlockAndWaitForDel(entry *LinkedListEntry[lruEntry[K, V]]) {
	if entry.Value.del == nil {
		entry.Value.del = make(chan struct{})
	}
	ch := entry.Value.del
	c.mu.Unlock()
	<-ch
}

// notifyOfDel unblocks any calls to .Delete(k), notifying them that
// the entry has been deleted and they can now return.
func (*lruCache[K, V]) notifyOfDel(entry *LinkedListEntry[lruEntry[K, V]]) {
	if entry.Value.del != nil {
		close(entry.Value.del)
		entry.Value.del = nil
	}
}

// Main implementation /////////////////////////////////////////////////////////

// lruReplace is the LRU(c) replacement policy.  It returns an entry
// that is not in any list.
func (c *lruCache[K, V]) lruReplace() *LinkedListEntry[lruEntry[K, V]] {
	c.waitForAvail()

	// If the cache isn't full, no need to do an eviction.
	if entry := c.unused.Oldest; entry != nil {
		c.unused.Delete(entry)
		return entry
	}

	// Replace the oldest entry.
	entry := c.evictable.Oldest
	c.evictable.Delete(entry)
	delete(c.byName, entry.Value.key)
	return entry
}

// Acquire implements the 'Cache' interface.
func (c *lruCache[K, V]) Acquire(ctx context.Context, k K) *V {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := c.byName[k]
	if entry != nil {
		if entry.Value.refs == 0 {
			c.evictable.Delete(entry)
		}
		entry.Value.refs++
	} else {
		entry = c.lruReplace()

		entry.Value.key = k
		c.src.Load(ctx, k, &entry.Value.val)
		entry.Value.refs = 1

		c.byName[k] = entry
	}

	return &entry.Value.val
}

// Delete implements the 'Cache' interface.
func (c *lruCache[K, V]) Delete(k K) {
	c.mu.Lock()

	entry := c.byName[k]
	if entry == nil {
		return
	}
	if entry.Value.refs > 0 {
		// Let .Release(k) do the deletion when the
		// refcount drops to 0.
		c.unlockAndWaitForDel(entry)
		return
	}
	delete(c.byName, k)
	c.evictable.Delete(entry)
	c.unused.Store(entry)

	// No need to call c.unlockAndNotifyAvail(); if we were able
	// to delete it, it was already available.

	c.mu.Unlock()
}

// Release implements the 'Cache' interface.
func (c *lruCache[K, V]) Release(k K) {
	c.mu.Lock()

	entry := c.byName[k]
	if entry == nil || entry.Value.refs <= 0 {
		panic(fmt.Errorf("containers.lruCache.Release called on key that is not held: %v", k))
	}

	entry.Value.refs--
	if entry.Value.refs == 0 {
		if entry.Value.del != nil {
			delete(c.byName, k)
			c.unused.Store(entry)
			c.notifyOfDel(entry)
		} else {
			c.evictable.Store(entry)
		}
		c.unlockAndNotifyAvail()
	} else {
		c.mu.Unlock()
	}
}

// Flush implements the 'Cache' interface.
func (c *lruCache[K, V]) Flush(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, entry := range c.byName {
		c.src.Flush(ctx, &entry.Value.val)
	}
	for entry := c.unused.Oldest; entry != nil; entry = entry.Newer {
		c.src.Flush(ctx, &entry.Value.val)
	}
}
