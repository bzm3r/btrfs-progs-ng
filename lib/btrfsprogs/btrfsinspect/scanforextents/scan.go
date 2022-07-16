// Copyright (C) 2022  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package scanforextents

import (
	"context"
	"errors"
	"fmt"

	"github.com/datawire/dlib/dlog"

	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsvol"
	"git.lukeshu.com/btrfs-progs-ng/lib/containers"
	"git.lukeshu.com/btrfs-progs-ng/lib/maps"
)

func ScanForExtents(ctx context.Context, fs *btrfs.FS, blockgroups *BlockGroupTree, sums AllSums) error {
	sb, err := fs.Superblock()
	if err != nil {
		return err
	}

	dlog.Info(ctx, "Reverse-indexing and validating logical sums...")
	sum2laddrs := make(map[ShortSum][]btrfsvol.LogicalAddr)
	if err := sums.WalkLogical(func(laddr btrfsvol.LogicalAddr, expShortSum ShortSum) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		readSum, err := ChecksumLogical(fs, sb.ChecksumType, laddr)
		if err != nil {
			if errors.Is(err, btrfsvol.ErrCouldNotMap) {
				sum2laddrs[expShortSum] = append(sum2laddrs[expShortSum], laddr)
				return nil
			}
			return err
		}
		readShortSum := ShortSum(readSum[:sums.ChecksumSize])
		if readShortSum != expShortSum {
			return fmt.Errorf("checksum mismatch at laddr=%v: CSUM_TREE=%x != read=%x",
				laddr, []byte(expShortSum), []byte(readShortSum))
		}
		return nil
	}); err != nil {
		return err
	}
	dlog.Info(ctx, "... done reverse-indexing and validating")

	dlog.Info(ctx, "Cross-referencing sums (and blockgroups) to re-construct mappings...")
	newMappings := &ExtentMappings{
		InLV:          &fs.LV,
		InBlockGroups: blockgroups,
		InSums:        sums,
		InReverseSums: sum2laddrs,
	}
	devs := fs.LV.PhysicalVolumes()
	gaps := ListPhysicalGaps(fs)
	for _, devID := range maps.SortedKeys(gaps) {
		if err := newMappings.ScanOneDev(ctx,
			devID, devs[devID].Name(),
			gaps[devID],
		); err != nil {
			return err
		}
	}
	dlog.Info(ctx, "... done cross-referencing")

	dlog.Info(ctx, "Applying those mappings...")
	for laddr, mappings := range newMappings.OutSum2mappings {
		if len(mappings) > 1 {
			dlog.Errorf(ctx, "multiple possibilities for laddr=%v :", laddr)
			for _, mapping := range mappings {
				dlog.Errorf(ctx, "  - %#v", *mapping)
			}
			continue
		}
		if err := fs.LV.AddMapping(*mappings[0]); err != nil {
			dlog.Error(ctx, err)
		}
	}
	dlog.Info(ctx, "... done applying")

	return nil
}

type ExtentMappings struct {
	// input
	InLV          *btrfsvol.LogicalVolume[*btrfs.Device]
	InBlockGroups *BlockGroupTree
	InSums        AllSums
	InReverseSums map[ShortSum][]btrfsvol.LogicalAddr

	// state
	internedMappings map[btrfsvol.Mapping]*btrfsvol.Mapping

	// output
	OutSum2mappings map[ShortSum][]*btrfsvol.Mapping
}

func (em *ExtentMappings) considerMapping(ctx context.Context, laddr btrfsvol.LogicalAddr, paddr btrfsvol.QualifiedPhysicalAddr) (btrfsvol.Mapping, bool) {
	blockgroup := LookupBlockGroup(em.InBlockGroups, laddr, csumBlockSize)
	if blockgroup == nil {
		return btrfsvol.Mapping{
			LAddr: laddr,
			PAddr: paddr,
			Size:  csumBlockSize,
		}, true
	}
	mapping := btrfsvol.Mapping{
		LAddr: blockgroup.LAddr,
		PAddr: btrfsvol.QualifiedPhysicalAddr{
			Dev:  paddr.Dev,
			Addr: paddr.Addr.Add(laddr.Sub(blockgroup.LAddr)),
		},
		Size:       blockgroup.Size,
		SizeLocked: true,
		Flags: containers.Optional[btrfsvol.BlockGroupFlags]{
			OK:  true,
			Val: blockgroup.Flags,
		},
	}
	if !em.InLV.CouldAddMapping(mapping) {
		return btrfsvol.Mapping{}, false
	}

	for offset := btrfsvol.AddrDelta(0); offset <= mapping.Size; offset += csumBlockSize {
		expCSum, ok := em.InSums.SumForLAddr(mapping.LAddr.Add(offset))
		if !ok {
			continue
		}
		actCSum, _ := em.InSums.SumForPAddr(mapping.PAddr.Add(offset))
		if actCSum != expCSum {
			return btrfsvol.Mapping{}, false
		}
	}
	return mapping, true
}

func (em *ExtentMappings) addMapping(sum ShortSum, mapping btrfsvol.Mapping) {
	interned := em.internedMappings[mapping]
	if interned == nil {
		interned = &mapping
		em.internedMappings[mapping] = interned
	}

	em.OutSum2mappings[sum] = append(em.OutSum2mappings[sum], interned)
}

func (em *ExtentMappings) ScanOneDev(
	ctx context.Context,
	devID btrfsvol.DeviceID, devName string,
	gaps []PhysicalGap,
) error {
	if em.internedMappings == nil {
		em.internedMappings = make(map[btrfsvol.Mapping]*btrfsvol.Mapping)
	}
	if em.OutSum2mappings == nil {
		em.OutSum2mappings = make(map[ShortSum][]*btrfsvol.Mapping)
	}

	dlog.Infof(ctx, "... dev[%q] Scanning for extents...", devName)

	lastProgress := -1
	potentialMappings := 0
	return WalkGaps(ctx, gaps, csumBlockSize,
		func(curBlock, totalBlocks int64) {
			pct := int(100 * float64(curBlock) / float64(totalBlocks))
			if pct != lastProgress || curBlock == totalBlocks {
				dlog.Infof(ctx, "... dev[%q] scanned %v%% (constructed %v potential mappings)",
					devName, pct, potentialMappings)
				lastProgress = pct
			}
		},
		func(paddr btrfsvol.PhysicalAddr) error {
			qpaddr := btrfsvol.QualifiedPhysicalAddr{
				Dev:  devID,
				Addr: paddr,
			}
			sum, _ := em.InSums.SumForPAddr(qpaddr)
			for _, laddr := range em.InReverseSums[sum] {
				mapping, ok := em.considerMapping(ctx, laddr, qpaddr)
				if !ok {
					continue
				}
				em.addMapping(sum, mapping)
				potentialMappings++
			}

			return nil
		},
	)
}
