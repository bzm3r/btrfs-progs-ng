// Copyright (C) 2022  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package containers

import (
	"errors"
)

type OrderedKV[K Ordered[K], V any] struct {
	K K
	V V
}

type SortedMap[K Ordered[K], V any] struct {
	inner RBTree[K, OrderedKV[K, V]]
}

func (m *SortedMap[K, V]) init() {
	if m.inner.KeyFn == nil {
		m.inner.KeyFn = m.keyFn
	}
}

func (m *SortedMap[K, V]) keyFn(kv OrderedKV[K, V]) K {
	return kv.K
}

func (m *SortedMap[K, V]) Delete(key K) {
	m.init()
	m.inner.Delete(key)
}

func (m *SortedMap[K, V]) Load(key K) (value V, ok bool) {
	m.init()
	node := m.inner.Lookup(key)
	if node == nil {
		var zero V
		return zero, false
	}
	return node.Value.V, true
}

var errStop = errors.New("stop")

func (m *SortedMap[K, V]) Range(f func(key K, value V) bool) {
	m.init()
	_ = m.inner.Walk(func(node *RBNode[OrderedKV[K, V]]) error {
		if f(node.Value.K, node.Value.V) {
			return nil
		} else {
			return errStop
		}
	})
}

func (m *SortedMap[K, V]) Subrange(rangeFn func(K, V) int, handleFn func(K, V) bool) {
	m.init()
	kvs := m.inner.SearchRange(func(kv OrderedKV[K, V]) int {
		return rangeFn(kv.K, kv.V)
	})
	for _, kv := range kvs {
		if !handleFn(kv.K, kv.V) {
			break
		}
	}
}

func (m *SortedMap[K, V]) Store(key K, value V) {
	m.init()
	m.inner.Insert(OrderedKV[K, V]{
		K: key,
		V: value,
	})
}

func (m *SortedMap[K, V]) Search(fn func(K, V) int) (K, V, bool) {
	node := m.inner.Search(func(kv OrderedKV[K, V]) int {
		return fn(kv.K, kv.V)
	})
	if node == nil {
		var zeroK K
		var zeroV V
		return zeroK, zeroV, false
	}
	return node.Value.K, node.Value.V, true
}

func (m *SortedMap[K, V]) SearchAll(fn func(K, V) int) []OrderedKV[K, V] {
	return m.inner.SearchRange(func(kv OrderedKV[K, V]) int {
		return fn(kv.K, kv.V)
	})
}

func (m *SortedMap[K, V]) Len() int {
	return m.inner.Len()
}
