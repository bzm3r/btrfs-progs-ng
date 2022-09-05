// Copyright (C) 2022  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package btrfstree

import (
	"fmt"
	"io"
	"strings"

	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsitem"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsprim"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsvol"
)

// TreePath is a path from the superblock (i.e. the root of the btrfs
// system) to the a node or item within one of the btrees in the
// system.
//
//   - The first element will always have an ItemIdx of -1.
//
//   - For .Item() callbacks, the last element will always have a
//     NodeAddr of 0.
//
// For example, a path through a tree, with the associated PathElems:
//
//	[superblock: tree=B, lvl=3, gen=6]
//	     |
//	     | <------------------------------------------ pathElem={from_tree:B, from_idx=-1,
//	     |                                                       to_addr:0x01, to_gen=6, to_lvl=3}
//	  +[0x01]-------------+
//	  | lvl=3 gen=6 own=B |
//	  +-+-+-+-+-+-+-+-+-+-+
//	  |0|1|2|3|4|5|6|7|8|9|
//	  +-+-+-+-+-+-+-+-+-+-+
//	                 |
//	                 | <------------------------------ pathElem:{from_tree:B, from_idx:7,
//	                 |                                           to_addr:0x02, to_gen:5, to_lvl:2}
//	              +[0x02]--------------+
//	              | lvl=2 gen=5 own=B  |
//	              +-+-+-+-+-+-+-+-+-+-+
//	              |0|1|2|3|4|5|6|7|8|9|
//	              +-+-+-+-+-+-+-+-+-+-+
//	                           |
//	                           | <-------------------- pathElem={from_tree:B, from_idx:6,
//	                           |                                 to_addr:0x03, to_gen:5, to_lvl:1}
//	                        +[0x03]-------------+
//	                        | lvl=1 gen=5 own=A |
//	                        +-+-+-+-+-+-+-+-+-+-+
//	                        |0|1|2|3|4|5|6|7|8|9|
//	                        +-+-+-+-+-+-+-+-+-+-+
//	                               |
//	                               | <---------------- pathElem={from_tree:A, from_idx:3,
//	                               |                             to_addr:0x04, to_gen:2, lvl:0}
//	                             +[0x04]-------------+
//	                             | lvl=0 gen=2 own=A |
//	                             +-+-+-+-+-+-+-+-+-+-+
//	                             |0|1|2|3|4|5|6|7|8|9|
//	                             +-+-+-+-+-+-+-+-+-+-+
//	                                |
//	                                | <--------------- pathElem={from_tree:A, from_idx:1,
//	                                |                            to_addr:0, to_gen: 0, to_lvl:0}
//	                              [item]
type TreePath []TreePathElem

// A TreePathElem essentially represents a KeyPointer.
type TreePathElem struct {
	// FromTree is the owning tree ID of the parent node; or the
	// well-known tree ID if this is the root.
	FromTree btrfsprim.ObjID
	// FromItemIdx is the index of this KeyPointer in the parent
	// Node; or -1 if this is the root and there is no KeyPointer.
	FromItemIdx int

	// ToNodeAddr is the address of the node that the KeyPointer
	// points at, or 0 if this is a leaf item and nothing is being
	// pointed at.
	ToNodeAddr btrfsvol.LogicalAddr
	// ToNodeGeneration is the expected generation of the node at
	// ToNodeAddr, or 0 if this is a leaf item and nothing is
	// being pointed at.
	ToNodeGeneration btrfsprim.Generation
	// ToNodeLevel is the expected level of the node at
	// ToNodeAddr, or 0 if this is a leaf item and nothing is
	// being pointed at.
	ToNodeLevel uint8
	// ToKey is btrfprim.Key{} this is the root node being pointed
	// to, the KeyPointer.Key if this is a non-root node being
	// pointed to, or the key of the leaf item being bointed to.
	ToKey btrfsprim.Key
}

func (elem TreePathElem) writeNodeTo(w io.Writer) {
	fmt.Fprintf(w, "node:%d@%v", elem.ToNodeLevel, elem.ToNodeAddr)
}

func (path TreePath) String() string {
	if len(path) == 0 {
		return "(empty-path)"
	} else {
		var ret strings.Builder
		fmt.Fprintf(&ret, "%s->", path[0].FromTree.Format(btrfsitem.ROOT_ITEM_KEY))
		if len(path) == 1 && path[0] == (TreePathElem{FromTree: path[0].FromTree, FromItemIdx: -1}) {
			ret.WriteString("(empty-path)")
		} else {
			path[0].writeNodeTo(&ret)
		}
		for _, elem := range path[1:] {
			fmt.Fprintf(&ret, "[%v]", elem.FromItemIdx)
			if elem.ToNodeAddr != 0 {
				ret.WriteString("->")
				elem.writeNodeTo(&ret)
			}
		}
		return ret.String()
	}
}

func (path TreePath) DeepCopy() TreePath {
	return append(TreePath(nil), path...)
}

func (path TreePath) Parent() TreePath {
	return path[:len(path)-1]
}

// path.Node(x) is like &path[x], but negative values of x move down
// from the end of path (similar to how lists work in many other
// languages, such as Python).
func (path TreePath) Node(x int) *TreePathElem {
	if x < 0 {
		x += len(path)
	}
	return &path[x]
}
