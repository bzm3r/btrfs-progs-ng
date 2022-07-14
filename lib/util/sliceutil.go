// Copyright (C) 2022  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package util

import (
	"sort"

	"golang.org/x/exp/constraints"
)

func InSlice[T comparable](needle T, haystack []T) bool {
	for _, straw := range haystack {
		if needle == straw {
			return true
		}
	}
	return false
}

func RemoveAllFromSlice[T comparable](haystack []T, needle T) []T {
	for i, straw := range haystack {
		if needle == straw {
			return append(
				haystack[:i],
				RemoveAllFromSlice(haystack[i+1:], needle)...)
		}
	}
	return haystack
}

func RemoveAllFromSliceFunc[T any](haystack []T, f func(T) bool) []T {
	for i, straw := range haystack {
		if f(straw) {
			return append(
				haystack[:i],
				RemoveAllFromSliceFunc(haystack[i+1:], f)...)
		}
	}
	return haystack
}

func ReverseSlice[T any](slice []T) {
	for i := 0; i < len(slice)/2; i++ {
		j := (len(slice) - 1) - i
		slice[i], slice[j] = slice[j], slice[i]
	}
}

func Max[T constraints.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func Min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func SortSlice[T constraints.Ordered](slice []T) {
	sort.Slice(slice, func(i, j int) bool {
		return slice[i] < slice[j]
	})
}
