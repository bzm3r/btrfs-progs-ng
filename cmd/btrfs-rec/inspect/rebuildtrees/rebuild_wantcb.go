// Copyright (C) 2022-2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package rebuildtrees

import (
	"bytes"
	"context"
	"fmt"

	"github.com/datawire/dlib/dlog"

	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsitem"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsprim"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfssum"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsvol"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfsutil"
	"git.lukeshu.com/btrfs-progs-ng/lib/containers"
)

type graphCallbacks struct {
	*rebuilder
}

// FSErr implements btrfscheck.GraphCallbacks.
func (graphCallbacks) FSErr(ctx context.Context, e error) {
	dlog.Errorf(ctx, "filesystem error: %v", e)
}

// Want implements btrfscheck.GraphCallbacks.
func (o graphCallbacks) Want(ctx context.Context, reason string, treeID btrfsprim.ObjID, objID btrfsprim.ObjID, typ btrfsprim.ItemType) {
	wantKey := wantWithTree{
		TreeID: treeID,
		Key: want{
			ObjectID:   objID,
			ItemType:   typ,
			OffsetType: offsetAny,
		},
	}
	ctx = withWant(ctx, logFieldItemWant, reason, wantKey)
	o._want(ctx, wantKey)
}

func (o *rebuilder) _want(ctx context.Context, wantKey wantWithTree) (key btrfsprim.Key, ok bool) {
	tree, err := o.rebuilt.RebuiltTree(ctx, wantKey.TreeID)
	if err != nil {
		o.enqueueRetry(wantKey.TreeID)
		return btrfsprim.Key{}, false
	}

	tgt := wantKey.Key.Key()

	// check if we already have it

	key, _, ok = tree.RebuiltAcquireItems(ctx).Search(func(key btrfsprim.Key, _ btrfsutil.ItemPtr) int {
		key.Offset = 0
		return tgt.Compare(key)
	})
	tree.RebuiltReleaseItems()
	if ok {
		return key, true
	}

	// OK, we need to insert it

	if o.hasAugment(wantKey) {
		return btrfsprim.Key{}, false
	}
	wants := make(containers.Set[btrfsvol.LogicalAddr])
	tree.RebuiltAcquirePotentialItems(ctx).Subrange(
		func(k btrfsprim.Key, _ btrfsutil.ItemPtr) int {
			k.Offset = 0
			return tgt.Compare(k)
		},
		func(_ btrfsprim.Key, v btrfsutil.ItemPtr) bool {
			wants.InsertFrom(tree.RebuiltLeafToRoots(ctx, v.Node))
			return true
		})
	tree.RebuiltReleasePotentialItems()
	o.wantAugment(ctx, wantKey, wants)
	return btrfsprim.Key{}, false
}

// WantOff implements btrfscheck.GraphCallbacks.
func (o graphCallbacks) WantOff(ctx context.Context, reason string, treeID btrfsprim.ObjID, objID btrfsprim.ObjID, typ btrfsprim.ItemType, off uint64) {
	wantKey := wantWithTree{
		TreeID: treeID,
		Key: want{
			ObjectID:   objID,
			ItemType:   typ,
			OffsetType: offsetExact,
			OffsetLow:  off,
		},
	}
	ctx = withWant(ctx, logFieldItemWant, reason, wantKey)
	o._wantOff(ctx, wantKey)
}

func (o *rebuilder) _wantOff(ctx context.Context, wantKey wantWithTree) (ok bool) {
	tree, err := o.rebuilt.RebuiltTree(ctx, wantKey.TreeID)
	if err != nil {
		o.enqueueRetry(wantKey.TreeID)
		return false
	}

	tgt := wantKey.Key.Key()

	// check if we already have it

	_, ok = tree.RebuiltAcquireItems(ctx).Load(tgt)
	tree.RebuiltReleaseItems()
	if ok {
		return true
	}

	// OK, we need to insert it

	if o.hasAugment(wantKey) {
		return false
	}
	wants := make(containers.Set[btrfsvol.LogicalAddr])
	tree.RebuiltAcquirePotentialItems(ctx).Subrange(
		func(k btrfsprim.Key, _ btrfsutil.ItemPtr) int { return tgt.Compare(k) },
		func(_ btrfsprim.Key, v btrfsutil.ItemPtr) bool {
			wants.InsertFrom(tree.RebuiltLeafToRoots(ctx, v.Node))
			return true
		})
	tree.RebuiltReleasePotentialItems()
	o.wantAugment(ctx, wantKey, wants)
	return false
}

// WantDirIndex implements btrfscheck.GraphCallbacks.
func (o graphCallbacks) WantDirIndex(ctx context.Context, reason string, treeID btrfsprim.ObjID, objID btrfsprim.ObjID, name []byte) {
	wantKey := wantWithTree{
		TreeID: treeID,
		Key: want{
			ObjectID:   objID,
			ItemType:   btrfsitem.DIR_INDEX_KEY,
			OffsetType: offsetName,
			OffsetName: string(name),
		},
	}
	ctx = withWant(ctx, logFieldItemWant, reason, wantKey)

	tree, err := o.rebuilt.RebuiltTree(ctx, treeID)
	if err != nil {
		o.enqueueRetry(treeID)
		return
	}

	tgt := wantKey.Key.Key()

	// check if we already have it

	found := false
	tree.RebuiltAcquireItems(ctx).Subrange(
		func(key btrfsprim.Key, _ btrfsutil.ItemPtr) int {
			key.Offset = 0
			return tgt.Compare(key)
		},
		func(_ btrfsprim.Key, ptr btrfsutil.ItemPtr) bool {
			if itemName, ok := o.scan.Names[ptr]; ok && bytes.Equal(itemName, name) {
				found = true
			}
			return !found
		})
	tree.RebuiltReleaseItems()
	if found {
		return
	}

	// OK, we need to insert it

	if o.hasAugment(wantKey) {
		return
	}
	wants := make(containers.Set[btrfsvol.LogicalAddr])
	tree.RebuiltAcquirePotentialItems(ctx).Subrange(
		func(key btrfsprim.Key, _ btrfsutil.ItemPtr) int {
			key.Offset = 0
			return tgt.Compare(key)
		},
		func(_ btrfsprim.Key, ptr btrfsutil.ItemPtr) bool {
			if itemName, ok := o.scan.Names[ptr]; ok && bytes.Equal(itemName, name) {
				wants.InsertFrom(tree.RebuiltLeafToRoots(ctx, ptr.Node))
			}
			return true
		})
	tree.RebuiltReleasePotentialItems()
	o.wantAugment(ctx, wantKey, wants)
}

func (o graphCallbacks) _walkRange(
	ctx context.Context,
	items *containers.SortedMap[btrfsprim.Key, btrfsutil.ItemPtr],
	treeID, objID btrfsprim.ObjID, typ btrfsprim.ItemType,
	beg, end uint64,
	fn func(key btrfsprim.Key, ptr btrfsutil.ItemPtr, beg, end uint64),
) {
	min := btrfsprim.Key{
		ObjectID: objID,
		ItemType: typ,
		Offset:   0, // *NOT* `beg`
	}
	max := btrfsprim.Key{
		ObjectID: objID,
		ItemType: typ,
		Offset:   end - 1,
	}
	items.Subrange(
		func(runKey btrfsprim.Key, _ btrfsutil.ItemPtr) int {
			switch {
			case min.Compare(runKey) < 0:
				return 1
			case max.Compare(runKey) > 0:
				return -1
			default:
				return 0
			}
		},
		func(runKey btrfsprim.Key, runPtr btrfsutil.ItemPtr) bool {
			runSizeAndErr, ok := o.scan.Sizes[runPtr]
			if !ok {
				panic(fmt.Errorf("should not happen: %v (%v) did not have a size recorded",
					runPtr, keyAndTree{TreeID: treeID, Key: runKey}))
			}
			if runSizeAndErr.Err != nil {
				o.FSErr(ctx, fmt.Errorf("get size: %v (%v): %w",
					runPtr, keyAndTree{TreeID: treeID, Key: runKey},
					runSizeAndErr.Err))
				return true
			}
			runSize := runSizeAndErr.Size
			if runSize == 0 {
				return true
			}
			runBeg := runKey.Offset
			runEnd := runBeg + runSize
			if runEnd <= beg {
				return true
			}

			fn(runKey, runPtr, runBeg, runEnd)
			return true
		})
}

type gap struct {
	// range is [Beg,End)
	Beg, End uint64
}

// Compare implements containers.Ordered.
func (a gap) Compare(b gap) int {
	return containers.NativeCompare(a.Beg, b.Beg)
}

func (o graphCallbacks) _wantRange(
	ctx context.Context, reason string,
	treeID btrfsprim.ObjID, objID btrfsprim.ObjID, typ btrfsprim.ItemType,
	beg, end uint64,
) {
	wantKey := wantWithTree{
		TreeID: treeID,
		Key: want{
			ObjectID:   objID,
			ItemType:   typ,
			OffsetType: offsetAny,
		},
	}
	ctx = withWant(ctx, logFieldItemWant, reason, wantKey)
	wantKey.Key.OffsetType = offsetRange

	tree, err := o.rebuilt.RebuiltTree(ctx, treeID)
	if err != nil {
		o.enqueueRetry(treeID)
		return
	}

	// Step 1: Build a listing of the gaps.
	//
	// Start with a gap of the whole range, then subtract each run
	// from it.
	gaps := new(containers.RBTree[gap])
	gaps.Insert(gap{
		Beg: beg,
		End: end,
	})
	o._walkRange(
		ctx,
		tree.RebuiltAcquireItems(ctx),
		treeID, objID, typ, beg, end,
		func(runKey btrfsprim.Key, _ btrfsutil.ItemPtr, runBeg, runEnd uint64) {
			var overlappingGaps []*containers.RBNode[gap]
			gaps.Subrange(
				func(gap gap) int {
					switch {
					case gap.End <= runBeg:
						return 1
					case runEnd <= gap.Beg:
						return -1
					default:
						return 0
					}
				},
				func(node *containers.RBNode[gap]) bool {
					overlappingGaps = append(overlappingGaps, node)
					return true
				})
			if len(overlappingGaps) == 0 {
				return
			}
			gapsBeg := overlappingGaps[0].Value.Beg
			gapsEnd := overlappingGaps[len(overlappingGaps)-1].Value.End
			for _, gap := range overlappingGaps {
				gaps.Delete(gap)
			}
			if gapsBeg < runBeg {
				gaps.Insert(gap{
					Beg: gapsBeg,
					End: runBeg,
				})
			}
			if gapsEnd > runEnd {
				gaps.Insert(gap{
					Beg: runEnd,
					End: gapsEnd,
				})
			}
		})
	tree.RebuiltReleaseItems()

	// Step 2: Fill each gap.
	if gaps.Len() == 0 {
		return
	}
	potentialItems := tree.RebuiltAcquirePotentialItems(ctx)
	gaps.Range(func(rbNode *containers.RBNode[gap]) bool {
		gap := rbNode.Value
		last := gap.Beg
		o._walkRange(
			ctx,
			potentialItems,
			treeID, objID, typ, gap.Beg, gap.End,
			func(k btrfsprim.Key, v btrfsutil.ItemPtr, runBeg, runEnd uint64) {
				// TODO: This is dumb and greedy.
				if last < runBeg {
					// log an error
					wantKey.Key.OffsetLow = last
					wantKey.Key.OffsetHigh = runBeg
					wantCtx := withWant(ctx, logFieldItemWant, reason, wantKey)
					o.wantAugment(wantCtx, wantKey, nil)
				}
				wantKey.Key.OffsetLow = gap.Beg
				wantKey.Key.OffsetHigh = gap.End
				wantCtx := withWant(ctx, logFieldItemWant, reason, wantKey)
				o.wantAugment(wantCtx, wantKey, tree.RebuiltLeafToRoots(wantCtx, v.Node))
				last = runEnd
			})
		if last < gap.End {
			// log an error
			wantKey.Key.OffsetLow = last
			wantKey.Key.OffsetHigh = gap.End
			wantCtx := withWant(ctx, logFieldItemWant, reason, wantKey)
			o.wantAugment(wantCtx, wantKey, nil)
		}
		return true
	})
	tree.RebuiltReleasePotentialItems()
}

// WantCSum implements btrfscheck.GraphCallbacks.
//
// interval is [beg, end)
func (o graphCallbacks) WantCSum(ctx context.Context, reason string, inodeTree, inode btrfsprim.ObjID, beg, end btrfsvol.LogicalAddr) {
	inodeWant := wantWithTree{
		TreeID: inodeTree,
		Key: want{
			ObjectID:   inode,
			ItemType:   btrfsitem.INODE_ITEM_KEY,
			OffsetType: offsetExact,
			OffsetLow:  0,
		},
	}
	inodeCtx := withWant(ctx, logFieldItemWant, reason, inodeWant)
	if !o._wantOff(inodeCtx, inodeWant) {
		o.enqueueRetry(inodeTree)
		return
	}
	tree := discardErr(o.rebuilt.RebuiltTree(inodeCtx, inodeTree))
	inodePtr, ok := tree.RebuiltAcquireItems(inodeCtx).Load(inodeWant.Key.Key())
	tree.RebuiltReleaseItems()
	if !ok {
		panic(fmt.Errorf("should not happen: could not load key: %v", inodeWant))
	}
	inodeFlags, ok := o.scan.Flags[inodePtr]
	if !ok {
		panic(fmt.Errorf("should not happen: INODE_ITEM did not have flags recorded"))
	}
	if inodeFlags.Err != nil {
		o.FSErr(inodeCtx, inodeFlags.Err)
		return
	}

	if inodeFlags.NoDataSum {
		return
	}

	o._wantRange(
		ctx, reason,
		btrfsprim.CSUM_TREE_OBJECTID, btrfsprim.EXTENT_CSUM_OBJECTID, btrfsprim.EXTENT_CSUM_KEY,
		uint64(roundDown(beg, btrfssum.BlockSize)), uint64(roundUp(end, btrfssum.BlockSize)))
}

// WantFileExt implements btrfscheck.GraphCallbacks.
func (o graphCallbacks) WantFileExt(ctx context.Context, reason string, treeID btrfsprim.ObjID, ino btrfsprim.ObjID, size int64) {
	o._wantRange(
		ctx, reason,
		treeID, ino, btrfsprim.EXTENT_DATA_KEY,
		0, uint64(size))
}
