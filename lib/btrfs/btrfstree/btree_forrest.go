// Copyright (C) 2022-2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package btrfstree

import (
	"context"
	"errors"
	"fmt"

	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsitem"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsprim"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsvol"
)

// LookupTreeRoot //////////////////////////////////////////////////////////////

// A TreeRoot is more-or-less a btrfsitem.Root, but simpler; returned by
// LookupTreeRoot.
type TreeRoot struct {
	ID         btrfsprim.ObjID
	RootNode   btrfsvol.LogicalAddr
	Level      uint8
	Generation btrfsprim.Generation

	RootInode  btrfsprim.ObjID // only for subvolume trees
	ParentUUID btrfsprim.UUID
	ParentGen  btrfsprim.Generation // offset of this tree's root item
}

// LookupTreeRoot is a utility function to help with implementing the
// 'Forrest' interface.
//
// The 'forrest' passed to LookupTreeRoot must handle:
//
//	forrest.ForrestLookup(ctx, btrfsprim.ROOT_TREE_OBJECTID)
//
// It is OK for forrest.ForrestLookup to recurse and call
// LookupTreeRoot, as LookupTreeRoot will not call ForrestLookup for
// ROOT_TREE_OBJECTID; so it will not be an infinite recursion.
func LookupTreeRoot(ctx context.Context, forrest Forrest, sb Superblock, treeID btrfsprim.ObjID) (*TreeRoot, error) {
	switch treeID {
	case btrfsprim.ROOT_TREE_OBJECTID:
		return &TreeRoot{
			ID:         treeID,
			RootNode:   sb.RootTree,
			Level:      sb.RootLevel,
			Generation: sb.Generation, // XXX: same generation as LOG_TREE?
		}, nil
	case btrfsprim.CHUNK_TREE_OBJECTID:
		return &TreeRoot{
			ID:         treeID,
			RootNode:   sb.ChunkTree,
			Level:      sb.ChunkLevel,
			Generation: sb.ChunkRootGeneration,
		}, nil
	case btrfsprim.TREE_LOG_OBJECTID:
		return &TreeRoot{
			ID:         treeID,
			RootNode:   sb.LogTree,
			Level:      sb.LogLevel,
			Generation: sb.Generation, // XXX: same generation as ROOT_TREE?
		}, nil
	case btrfsprim.BLOCK_GROUP_TREE_OBJECTID:
		return &TreeRoot{
			ID:         treeID,
			RootNode:   sb.BlockGroupRoot,
			Level:      sb.BlockGroupRootLevel,
			Generation: sb.BlockGroupRootGeneration,
		}, nil
	default:
		rootTree, err := forrest.ForrestLookup(ctx, btrfsprim.ROOT_TREE_OBJECTID)
		if err != nil {
			return nil, fmt.Errorf("tree %s: %w", treeID.Format(btrfsprim.ROOT_TREE_OBJECTID), err)
		}
		rootItem, err := rootTree.TreeSearch(ctx, SearchRootItem(treeID))
		if err != nil {
			if errors.Is(err, ErrNoItem) {
				err = fmt.Errorf("%w: %s", ErrNoTree, err)
			}
			return nil, fmt.Errorf("tree %s: %w", treeID.Format(btrfsprim.ROOT_TREE_OBJECTID), err)
		}
		switch rootItemBody := rootItem.Body.(type) {
		case *btrfsitem.Root:
			return &TreeRoot{
				ID:         treeID,
				RootNode:   rootItemBody.ByteNr,
				Level:      rootItemBody.Level,
				Generation: rootItemBody.Generation,

				RootInode:  rootItemBody.RootDirID,
				ParentUUID: rootItemBody.ParentUUID,
				ParentGen:  btrfsprim.Generation(rootItem.Key.Offset),
			}, nil
		case *btrfsitem.Error:
			return nil, fmt.Errorf("malformed ROOT_ITEM for tree %v: %w", treeID, rootItemBody.Err)
		default:
			panic(fmt.Errorf("should not happen: ROOT_ITEM has unexpected item type: %T", rootItemBody))
		}
	}
}

// RawForrest //////////////////////////////////////////////////////////////////

// RawForrest implements Forrest.
type RawForrest struct {
	NodeSource NodeSource
}

var _ Forrest = RawForrest{}

// RawTree is a variant of ForrestLookup that returns a concrete type
// instead of an interface.
func (forrest RawForrest) RawTree(ctx context.Context, treeID btrfsprim.ObjID) (*RawTree, error) {
	sb, err := forrest.NodeSource.Superblock()
	if err != nil {
		return nil, err
	}
	rootInfo, err := LookupTreeRoot(ctx, forrest, *sb, treeID)
	if err != nil {
		return nil, err
	}
	return &RawTree{
		Forrest:  forrest,
		TreeRoot: *rootInfo,
	}, nil
}

// ForrestLookup implements Forrest.
func (forrest RawForrest) ForrestLookup(ctx context.Context, treeID btrfsprim.ObjID) (Tree, error) {
	tree, err := forrest.RawTree(ctx, treeID)
	if err != nil {
		return nil, err
	}
	return tree, nil
}
