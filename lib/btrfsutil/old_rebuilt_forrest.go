// Copyright (C) 2022-2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package btrfsutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/datawire/dlib/derror"
	"github.com/datawire/dlib/dlog"

	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsitem"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsprim"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfstree"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsvol"
	"git.lukeshu.com/btrfs-progs-ng/lib/containers"
)

type oldRebuiltTree struct {
	forrest *OldRebuiltForrest

	ID         btrfsprim.ObjID
	ParentUUID btrfsprim.UUID
	ParentGen  btrfsprim.Generation // offset of this tree's root item

	RootErr error
	Items   *containers.RBTree[oldRebuiltTreeValue]
	Errors  *containers.IntervalTree[btrfsprim.Key, oldRebuiltTreeError]
}

var _ btrfstree.Tree = oldRebuiltTree{}

type oldRebuiltTreeError struct {
	Min btrfsprim.Key
	Max btrfsprim.Key
	Err error
}

func (e oldRebuiltTreeError) Error() string {
	return fmt.Sprintf("keys %v-%v: %v", e.Min, e.Max, e.Err)
}

func (e oldRebuiltTreeError) Unwrap() error {
	return e.Err
}

type oldRebuiltTreeValue struct {
	Key      btrfsprim.Key
	ItemSize uint32

	Node nodeInfo
	Slot int
}

type nodeInfo struct {
	LAddr      btrfsvol.LogicalAddr
	Level      uint8
	Generation btrfsprim.Generation
	Owner      btrfsprim.ObjID
	MinItem    btrfsprim.Key
	MaxItem    btrfsprim.Key
}

// Compare implements containers.Ordered.
func (a oldRebuiltTreeValue) Compare(b oldRebuiltTreeValue) int {
	return a.Key.Compare(b.Key)
}

func newOldRebuiltTree() oldRebuiltTree {
	return oldRebuiltTree{
		Items: new(containers.RBTree[oldRebuiltTreeValue]),
		Errors: &containers.IntervalTree[btrfsprim.Key, oldRebuiltTreeError]{
			MinFn: func(err oldRebuiltTreeError) btrfsprim.Key {
				return err.Min
			},
			MaxFn: func(err oldRebuiltTreeError) btrfsprim.Key {
				return err.Max
			},
		},
	}
}

type OldRebuiltForrest struct {
	ctx   context.Context //nolint:containedctx // don't have an option while keeping the same API
	inner *btrfs.FS

	// btrfsprim.ROOT_TREE_OBJECTID
	rootTreeMu sync.Mutex
	rootTree   *oldRebuiltTree
	// for all other trees
	treesMu sync.Mutex
	trees   map[btrfsprim.ObjID]oldRebuiltTree
}

var _ btrfstree.TreeOperator = (*OldRebuiltForrest)(nil)

// NewOldRebuiltForrest wraps a *btrfs.FS to support looking up
// information from broken trees.
//
// Of the btrfstree.TreeOperator methods:
//
//   - TreeWalk works on broken trees
//   - TreeLookup relies on the tree being properly ordered (which a
//     broken tree might not be).
//   - TreeSearch relies on the tree being properly ordered (which a
//     broken tree might not be).
//   - TreeSearchAll relies on the tree being properly ordered (which a
//     broken tree might not be), and a bad node may cause it to not
//     return a truncated list of results.
//
// NewOldRebuiltForrest attempts to remedy these deficiencies by using
// .TreeWalk to build an out-of-FS index of all of the items in the
// tree, and re-implements TreeLookup, TreeSearch, and TreeSearchAll
// using that index.
func NewOldRebuiltForrest(ctx context.Context, inner *btrfs.FS) *OldRebuiltForrest {
	return &OldRebuiltForrest{
		ctx:   ctx,
		inner: inner,
	}
}

// RebuiltTree returns a handle for an individual tree.  An error is
// indicated by the ret.RootErr member.
func (bt *OldRebuiltForrest) RebuiltTree(ctx context.Context, treeID btrfsprim.ObjID) oldRebuiltTree {
	if treeID == btrfsprim.ROOT_TREE_OBJECTID {
		bt.rootTreeMu.Lock()
		defer bt.rootTreeMu.Unlock()
		if bt.rootTree != nil {
			return *bt.rootTree
		}
	} else {
		bt.treesMu.Lock()
		defer bt.treesMu.Unlock()
		if bt.trees == nil {
			bt.trees = make(map[btrfsprim.ObjID]oldRebuiltTree)
		}
		if cacheEntry, exists := bt.trees[treeID]; exists {
			return cacheEntry
		}
	}

	cacheEntry := newOldRebuiltTree()
	cacheEntry.forrest = bt
	cacheEntry.ID = treeID
	dlog.Infof(ctx, "indexing tree %v...", treeID)
	bt.rawTreeWalk(ctx, treeID, &cacheEntry)
	dlog.Infof(ctx, "... done indexing tree %v", treeID)

	if treeID == btrfsprim.ROOT_TREE_OBJECTID {
		bt.rootTree = &cacheEntry
	} else {
		bt.trees[treeID] = cacheEntry
	}
	return cacheEntry
}

func discardOK[T any](x T, _ bool) T { return x }

func (bt *OldRebuiltForrest) rawTreeWalk(ctx context.Context, treeID btrfsprim.ObjID, cacheEntry *oldRebuiltTree) {
	sb, err := bt.inner.Superblock()
	if err != nil {
		cacheEntry.RootErr = err
		return
	}
	root, err := btrfstree.LookupTreeRoot(ctx, bt, *sb, treeID)
	if err != nil {
		cacheEntry.RootErr = err
		return
	}
	tree := &btrfstree.RawTree{
		Forrest:  btrfstree.TreeOperatorImpl{NodeSource: bt.inner},
		TreeRoot: *root,
	}

	cacheEntry.ParentUUID = root.ParentUUID
	cacheEntry.ParentGen = root.ParentGen

	var curNode nodeInfo
	cbs := btrfstree.TreeWalkHandler{
		BadNode: func(path btrfstree.Path, node *btrfstree.Node, err error) bool {
			_, nodeExp, _ := path.NodeExpectations(ctx, false)
			cacheEntry.Errors.Insert(oldRebuiltTreeError{
				Min: nodeExp.MinItem.Val,
				Max: nodeExp.MaxItem.Val,
				Err: err,
			})
			return false
		},
		Node: func(path btrfstree.Path, node *btrfstree.Node) {
			curNode = nodeInfo{
				LAddr:      node.Head.Addr,
				Level:      node.Head.Level,
				Generation: node.Head.Generation,
				Owner:      node.Head.Owner,
				MinItem:    discardOK(node.MinItem()),
				MaxItem:    discardOK(node.MaxItem()),
			}
		},
		Item: func(path btrfstree.Path, item btrfstree.Item) {
			if cacheEntry.Items.Search(func(v oldRebuiltTreeValue) int { return item.Key.Compare(v.Key) }) != nil {
				// This is a panic because I'm not really sure what the best way to
				// handle this is, and so if this happens I want the program to crash
				// and force me to figure out how to handle it.
				panic(fmt.Errorf("dup key=%v in tree=%v", item.Key, treeID))
			}
			cacheEntry.Items.Insert(oldRebuiltTreeValue{
				Key:      item.Key,
				ItemSize: item.BodySize,

				Node: curNode,
				Slot: path[len(path)-1].(btrfstree.PathItem).FromSlot, //nolint:forcetypeassert // has to be
			})
		},
	}
	cbs.BadItem = cbs.Item

	tree.TreeWalk(ctx, cbs)
}

func (tree oldRebuiltTree) addErrs(fn func(btrfsprim.Key, uint32) int, err error) error {
	var errs derror.MultiError
	tree.Errors.Subrange(
		func(k btrfsprim.Key) int { return fn(k, 0) },
		func(v oldRebuiltTreeError) bool {
			errs = append(errs, v)
			return true
		})
	if len(errs) == 0 {
		return err
	}
	if err != nil {
		errs = append(errs, err)
	}
	return errs
}

func (bt *OldRebuiltForrest) readNode(nodeInfo nodeInfo) *btrfstree.Node {
	node, err := bt.inner.AcquireNode(bt.ctx, nodeInfo.LAddr, btrfstree.NodeExpectations{
		LAddr:      containers.OptionalValue(nodeInfo.LAddr),
		Level:      containers.OptionalValue(nodeInfo.Level),
		Generation: containers.OptionalValue(nodeInfo.Generation),
		Owner: func(treeID btrfsprim.ObjID, gen btrfsprim.Generation) error {
			if treeID != nodeInfo.Owner || gen != nodeInfo.Generation {
				return fmt.Errorf("expected owner=%v generation=%v but claims to have owner=%v generation=%v",
					nodeInfo.Owner, nodeInfo.Generation,
					treeID, gen)
			}
			return nil
		},
		MinItem: containers.OptionalValue(nodeInfo.MinItem),
		MaxItem: containers.OptionalValue(nodeInfo.MaxItem),
	})
	if err != nil {
		panic(fmt.Errorf("should not happen: i/o error: %w", err))
	}

	return node
}

// TreeLookup implements btrfstree.TreeOperator.
func (bt *OldRebuiltForrest) TreeLookup(treeID btrfsprim.ObjID, key btrfsprim.Key) (btrfstree.Item, error) {
	return bt.RebuiltTree(bt.ctx, treeID).treeLookup(bt.ctx, key)
}

func (tree oldRebuiltTree) treeLookup(ctx context.Context, key btrfsprim.Key) (btrfstree.Item, error) {
	return tree.treeSearch(ctx, btrfstree.SearchExactKey(key))
}

// TreeSearch implements btrfstree.TreeOperator.
func (bt *OldRebuiltForrest) TreeSearch(treeID btrfsprim.ObjID, searcher btrfstree.TreeSearcher) (btrfstree.Item, error) {
	return bt.RebuiltTree(bt.ctx, treeID).treeSearch(bt.ctx, searcher)
}

// TreeSearch implements btrfstree.Tree.
func (tree oldRebuiltTree) treeSearch(_ context.Context, searcher btrfstree.TreeSearcher) (btrfstree.Item, error) {
	if tree.RootErr != nil {
		return btrfstree.Item{}, tree.RootErr
	}

	indexItem := tree.Items.Search(func(indexItem oldRebuiltTreeValue) int {
		return searcher.Search(indexItem.Key, indexItem.ItemSize)
	})
	if indexItem == nil {
		return btrfstree.Item{}, fmt.Errorf("item with %s: %w", searcher, tree.addErrs(searcher.Search, btrfstree.ErrNoItem))
	}

	node := tree.forrest.readNode(indexItem.Value.Node)
	defer tree.forrest.inner.ReleaseNode(node)

	item := node.BodyLeaf[indexItem.Value.Slot]
	item.Body = item.Body.CloneItem()

	// Since we were only asked to return 1 item, it isn't
	// necessary to augment this `nil` with tree.addErrs().
	return item, nil
}

// TreeSearchAll implements btrfstree.TreeOperator.
func (bt *OldRebuiltForrest) TreeSearchAll(treeID btrfsprim.ObjID, searcher btrfstree.TreeSearcher) ([]btrfstree.Item, error) {
	tree := bt.RebuiltTree(bt.ctx, treeID)
	if tree.RootErr != nil {
		return nil, tree.RootErr
	}

	var ret []btrfstree.Item
	err := tree.treeSubrange(bt.ctx, 1, searcher, func(item btrfstree.Item) bool {
		item.Body = item.Body.CloneItem()
		ret = append(ret, item)
		return true
	})

	return ret, err
}

func (tree oldRebuiltTree) treeSubrange(_ context.Context, min int, searcher btrfstree.TreeSearcher, handleFn func(btrfstree.Item) bool) error {
	var node *btrfstree.Node
	var cnt int
	tree.Items.Subrange(
		func(indexItem oldRebuiltTreeValue) int {
			return searcher.Search(indexItem.Key, indexItem.ItemSize)
		},
		func(rbNode *containers.RBNode[oldRebuiltTreeValue]) bool {
			cnt++
			if node == nil || node.Head.Addr != rbNode.Value.Node.LAddr {
				tree.forrest.inner.ReleaseNode(node)
				node = tree.forrest.readNode(rbNode.Value.Node)
			}
			return handleFn(node.BodyLeaf[rbNode.Value.Slot])
		})
	tree.forrest.inner.ReleaseNode(node)

	var err error
	if cnt < min {
		err = btrfstree.ErrNoItem
	}
	err = tree.addErrs(searcher.Search, err)
	if err != nil {
		err = fmt.Errorf("items with %s: %w", searcher, err)
	}
	return err
}

// TreeWalk implements btrfstree.TreeOperator.  It doesn't actually
// visit nodes or keypointers (just items).
func (bt *OldRebuiltForrest) TreeWalk(ctx context.Context, treeID btrfsprim.ObjID, errHandle func(*btrfstree.TreeError), cbs btrfstree.TreeWalkHandler) {
	tree := bt.RebuiltTree(ctx, treeID)
	if tree.RootErr != nil {
		errHandle(&btrfstree.TreeError{
			Path: btrfstree.Path{btrfstree.PathRoot{TreeID: treeID}},
			Err:  tree.RootErr,
		})
		return
	}
	tree.treeWalk(ctx, cbs)
}

func (tree oldRebuiltTree) treeWalk(ctx context.Context, cbs btrfstree.TreeWalkHandler) {
	if cbs.Item == nil && cbs.BadItem == nil {
		return
	}
	var node *btrfstree.Node
	tree.Items.Range(func(indexItem *containers.RBNode[oldRebuiltTreeValue]) bool {
		if ctx.Err() != nil {
			return false
		}
		if tree.forrest.ctx.Err() != nil {
			return false
		}
		if node == nil || node.Head.Addr != indexItem.Value.Node.LAddr {
			tree.forrest.inner.ReleaseNode(node)
			node = tree.forrest.readNode(indexItem.Value.Node)
		}
		item := node.BodyLeaf[indexItem.Value.Slot]

		itemPath := btrfstree.Path{
			btrfstree.PathRoot{
				Tree:         tree,
				TreeID:       tree.ID,
				ToAddr:       indexItem.Value.Node.LAddr,
				ToGeneration: indexItem.Value.Node.Generation,
				ToLevel:      indexItem.Value.Node.Level,
			},
			btrfstree.PathItem{
				FromTree: indexItem.Value.Node.Owner,
				FromSlot: indexItem.Value.Slot,
				ToKey:    indexItem.Value.Key,
			},
		}
		switch item.Body.(type) {
		case *btrfsitem.Error:
			if cbs.BadItem != nil {
				cbs.BadItem(itemPath, item)
			}
		default:
			if cbs.Item != nil {
				cbs.Item(itemPath, item)
			}
		}
		return ctx.Err() == nil
	})
	tree.forrest.inner.ReleaseNode(node)
}

// Superblock implements btrfs.ReadableFS.
func (bt *OldRebuiltForrest) Superblock() (*btrfstree.Superblock, error) {
	return bt.inner.Superblock()
}

// ReadAt implements diskio.ReaderAt (and btrfs.ReadableFS).
func (bt *OldRebuiltForrest) ReadAt(p []byte, off btrfsvol.LogicalAddr) (int, error) {
	return bt.inner.ReadAt(p, off)
}

// TreeCheckOwner implements btrfstree.Tree.
func (tree oldRebuiltTree) TreeCheckOwner(ctx context.Context, failOpen bool, owner btrfsprim.ObjID, gen btrfsprim.Generation) error {
	var uuidTree oldRebuiltTree
	for {
		// Main.
		if owner == tree.ID {
			return nil
		}
		if tree.ParentUUID == (btrfsprim.UUID{}) {
			return fmt.Errorf("owner=%v is not acceptable in this tree",
				owner)
		}
		if gen > tree.ParentGen {
			return fmt.Errorf("claimed owner=%v might be acceptable in this tree (if generation<=%v) but not with claimed generation=%v",
				owner, tree.ParentGen, gen)
		}

		// Loop update.
		if uuidTree.forrest == nil {
			uuidTree = tree.forrest.RebuiltTree(ctx, btrfsprim.UUID_TREE_OBJECTID)
			if uuidTree.RootErr != nil {
				return nil //nolint:nilerr // fail open
			}
		}
		parentIDItem, err := uuidTree.treeLookup(ctx, btrfsitem.UUIDToKey(tree.ParentUUID))
		if err != nil {
			if failOpen {
				return nil
			}
			return fmt.Errorf("unable to determine whether owner=%v generation=%v is acceptable: %w",
				owner, gen, err)
		}
		switch parentIDBody := parentIDItem.Body.(type) {
		case *btrfsitem.UUIDMap:
			tree = tree.forrest.RebuiltTree(ctx, parentIDBody.ObjID)
			if tree.RootErr != nil {
				if failOpen {
					return nil
				}
				return fmt.Errorf("unable to determine whether owner=%v generation=%v is acceptable: %w",
					owner, gen, tree.RootErr)
			}
		case *btrfsitem.Error:
			if failOpen {
				return nil
			}
			return fmt.Errorf("unable to determine whether owner=%v generation=%v is acceptable: %w",
				owner, gen, parentIDBody.Err)
		default:
			// This is a panic because the item decoder should not emit UUID_SUBVOL items as anything but
			// btrfsitem.UUIDMap or btrfsitem.Error without this code also being updated.
			panic(fmt.Errorf("should not happen: UUID_SUBVOL item has unexpected type: %T", parentIDBody))
		}
	}
}
