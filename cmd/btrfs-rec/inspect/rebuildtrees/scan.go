// Copyright (C) 2022-2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package rebuildtrees

import (
	"context"
	"fmt"
	"time"

	"github.com/datawire/dlib/dlog"

	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsitem"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsprim"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfstree"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsvol"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfsutil"
	"git.lukeshu.com/btrfs-progs-ng/lib/containers"
	"git.lukeshu.com/btrfs-progs-ng/lib/textui"
)

type SizeAndErr struct {
	Size uint64
	Err  error
}

type FlagsAndErr struct {
	NoDataSum bool
	Err       error
}

// ExtentDataRefPtr is a pointer to a *btrfsitem.ExtentDataRef,
// whether it be to an EXTENT_DATA_REF item proper, or to an inline
// ref inside of another EXTENT_ITEM or METADATA_ITEM.
type ExtentDataRefPtr struct {
	btrfsutil.ItemPtr
	RefNum int // Only for EXTENT_ITEM and METADATA_ITEM
}

type ScanDevicesResult struct {
	Graph btrfsutil.Graph

	Flags        map[btrfsutil.ItemPtr]FlagsAndErr       // INODE_ITEM
	Names        map[btrfsutil.ItemPtr][]byte            // DIR_INDEX
	Sizes        map[btrfsutil.ItemPtr]SizeAndErr        // EXTENT_CSUM and EXTENT_DATA
	DataBackrefs map[btrfsutil.ItemPtr][]btrfsprim.ObjID // EXTENT_DATA_REF, EXTENT_ITEM, and METADATA_ITEM
}

func ScanDevices(_ctx context.Context, fs *btrfs.FS, nodeList []btrfsvol.LogicalAddr) (ScanDevicesResult, error) {
	// read-superblock /////////////////////////////////////////////////////////////
	ctx := dlog.WithField(_ctx, "btrfs.inspect.rebuild-trees.read.substep", "read-superblock")
	dlog.Info(ctx, "Reading superblock...")
	sb, err := fs.Superblock()
	if err != nil {
		return ScanDevicesResult{}, err
	}

	// read-roots //////////////////////////////////////////////////////////////////
	ctx = dlog.WithField(_ctx, "btrfs.inspect.rebuild-trees.read.substep", "read-roots")
	ret := ScanDevicesResult{
		Graph: btrfsutil.NewGraph(ctx, *sb),

		Flags:        make(map[btrfsutil.ItemPtr]FlagsAndErr),
		Names:        make(map[btrfsutil.ItemPtr][]byte),
		Sizes:        make(map[btrfsutil.ItemPtr]SizeAndErr),
		DataBackrefs: make(map[btrfsutil.ItemPtr][]btrfsprim.ObjID),
	}

	// read-nodes //////////////////////////////////////////////////////////////////
	ctx = dlog.WithField(_ctx, "btrfs.inspect.rebuild-trees.read.substep", "read-nodes")
	dlog.Infof(ctx, "Reading node data from FS...")
	var stats textui.Portion[int]
	stats.D = len(nodeList)
	progressWriter := textui.NewProgress[textui.Portion[int]](
		ctx,
		dlog.LogLevelInfo,
		textui.Tunable(1*time.Second))
	progressWriter.Set(stats)
	for _, laddr := range nodeList {
		if err := ctx.Err(); err != nil {
			progressWriter.Done()
			return ScanDevicesResult{}, err
		}
		node, err := fs.AcquireNode(ctx, laddr, btrfstree.NodeExpectations{
			LAddr: containers.OptionalValue(laddr),
		})
		if err != nil {
			fs.ReleaseNode(node)
			progressWriter.Done()
			return ScanDevicesResult{}, err
		}
		ret.insertNode(node)
		fs.ReleaseNode(node)
		stats.N++
		progressWriter.Set(stats)
	}
	if stats.N != stats.D {
		panic("should not happen")
	}
	progressWriter.Done()
	dlog.Info(ctx, "... done reading node data")

	// check ///////////////////////////////////////////////////////////////////////
	ctx = dlog.WithField(_ctx, "btrfs.inspect.rebuild-trees.read.substep", "check")
	if err := ret.Graph.FinalCheck(ctx, fs); err != nil {
		return ScanDevicesResult{}, err
	}

	return ret, nil
}

func (o *ScanDevicesResult) insertNode(node *btrfstree.Node) {
	o.Graph.InsertNode(node)
	for i, item := range node.BodyLeaf {
		ptr := btrfsutil.ItemPtr{
			Node: node.Head.Addr,
			Slot: i,
		}
		switch itemBody := item.Body.(type) {
		case *btrfsitem.Inode:
			o.Flags[ptr] = FlagsAndErr{
				NoDataSum: itemBody.Flags.Has(btrfsitem.INODE_NODATASUM),
				Err:       nil,
			}
		case *btrfsitem.DirEntry:
			if item.Key.ItemType == btrfsprim.DIR_INDEX_KEY {
				o.Names[ptr] = append([]byte(nil), itemBody.Name...)
			}
		case *btrfsitem.ExtentCSum:
			o.Sizes[ptr] = SizeAndErr{
				Size: uint64(itemBody.Size()),
				Err:  nil,
			}
		case *btrfsitem.FileExtent:
			size, err := itemBody.Size()
			o.Sizes[ptr] = SizeAndErr{
				Size: uint64(size),
				Err:  err,
			}
		case *btrfsitem.Extent:
			o.DataBackrefs[ptr] = make([]btrfsprim.ObjID, len(itemBody.Refs))
			for i, ref := range itemBody.Refs {
				if refBody, ok := ref.Body.(*btrfsitem.ExtentDataRef); ok {
					o.DataBackrefs[ptr][i] = refBody.Root
				}
			}
		case *btrfsitem.Metadata:
			o.DataBackrefs[ptr] = make([]btrfsprim.ObjID, len(itemBody.Refs))
			for i, ref := range itemBody.Refs {
				if refBody, ok := ref.Body.(*btrfsitem.ExtentDataRef); ok {
					o.DataBackrefs[ptr][i] = refBody.Root
				}
			}
		case *btrfsitem.ExtentDataRef:
			o.DataBackrefs[ptr] = []btrfsprim.ObjID{itemBody.Root}
		case *btrfsitem.Error:
			switch item.Key.ItemType {
			case btrfsprim.INODE_ITEM_KEY:
				o.Flags[ptr] = FlagsAndErr{
					Err: fmt.Errorf("error decoding item: ptr=%v (tree=%v key=%v): %w",
						ptr, node.Head.Owner, item.Key, itemBody.Err),
				}
			case btrfsprim.EXTENT_CSUM_KEY, btrfsprim.EXTENT_DATA_KEY:
				o.Sizes[ptr] = SizeAndErr{
					Err: fmt.Errorf("error decoding item: ptr=%v (tree=%v key=%v): %w",
						ptr, node.Head.Owner, item.Key, itemBody.Err),
				}
			}
		}
	}
}
