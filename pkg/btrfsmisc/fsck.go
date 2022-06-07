package btrfsmisc

import (
	"errors"
	"fmt"

	"lukeshu.com/btrfs-tools/pkg/btrfs"
	"lukeshu.com/btrfs-tools/pkg/util"
)

// ScanForNodes mimics btrfs-progs
// cmds/rescue-chunk-recover.c:scan_one_device(), except it doesn't do
// anything but log when it finds a node.
func ScanForNodes(dev *btrfs.Device, sb btrfs.Superblock, fn func(*util.Ref[btrfs.PhysicalAddr, btrfs.Node], error), prog func(btrfs.PhysicalAddr)) error {
	devSize, err := dev.Size()
	if err != nil {
		return err
	}

	if sb.NodeSize < sb.SectorSize {
		return fmt.Errorf("node_size(%d) < sector_size(%d)",
			sb.NodeSize, sb.SectorSize)
	}

	for pos := btrfs.PhysicalAddr(0); pos+btrfs.PhysicalAddr(sb.NodeSize) < devSize; pos += btrfs.PhysicalAddr(sb.SectorSize) {
		if util.InSlice(pos, btrfs.SuperblockAddrs) {
			//fmt.Printf("sector@%d is a superblock\n", pos)
			continue
		}

		if prog != nil {
			prog(pos)
		}

		nodeRef, err := btrfs.ReadNode[btrfs.PhysicalAddr](dev, sb, pos, nil)
		if err != nil && errors.Is(err, btrfs.ErrNotANode) {
			continue
		}
		fn(nodeRef, err)

		pos += btrfs.PhysicalAddr(sb.NodeSize) - btrfs.PhysicalAddr(sb.SectorSize)
	}

	if prog != nil {
		prog(devSize)
	}

	return nil
}
