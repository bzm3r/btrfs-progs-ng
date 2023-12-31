// Copyright (C) 2022-2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package rebuildmappings

import (
	"context"
	"sort"

	"golang.org/x/exp/constraints"

	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfssum"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsvol"
	"git.lukeshu.com/btrfs-progs-ng/lib/maps"
)

func extractPhysicalSums(scanResults ScanDevicesResult) map[btrfsvol.DeviceID]btrfssum.SumRun[btrfsvol.PhysicalAddr] {
	ret := make(map[btrfsvol.DeviceID]btrfssum.SumRun[btrfsvol.PhysicalAddr], len(scanResults))
	for devID, devResults := range scanResults {
		ret[devID] = devResults.Checksums
	}
	return ret
}

type physicalRegion struct {
	Beg, End btrfsvol.PhysicalAddr
}

func listUnmappedPhysicalRegions(fs *btrfs.FS) map[btrfsvol.DeviceID][]physicalRegion {
	regions := make(map[btrfsvol.DeviceID][]physicalRegion)
	pos := make(map[btrfsvol.DeviceID]btrfsvol.PhysicalAddr)
	mappings := fs.LV.Mappings()
	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].PAddr.Compare(mappings[j].PAddr) < 0
	})
	for _, mapping := range mappings {
		if pos[mapping.PAddr.Dev] < mapping.PAddr.Addr {
			regions[mapping.PAddr.Dev] = append(regions[mapping.PAddr.Dev], physicalRegion{
				Beg: pos[mapping.PAddr.Dev],
				End: mapping.PAddr.Addr,
			})
		}
		if pos[mapping.PAddr.Dev] < mapping.PAddr.Addr.Add(mapping.Size) {
			pos[mapping.PAddr.Dev] = mapping.PAddr.Addr.Add(mapping.Size)
		}
	}
	for devID, dev := range fs.LV.PhysicalVolumes() {
		devSize := dev.Size()
		if pos[devID] < devSize {
			regions[devID] = append(regions[devID], physicalRegion{
				Beg: pos[devID],
				End: devSize,
			})
		}
	}
	return regions
}

func roundUp[T constraints.Integer](x, multiple T) T {
	return ((x + multiple - 1) / multiple) * multiple
}

func walkUnmappedPhysicalRegions(ctx context.Context,
	physicalSums map[btrfsvol.DeviceID]btrfssum.SumRun[btrfsvol.PhysicalAddr],
	gaps map[btrfsvol.DeviceID][]physicalRegion,
	fn func(btrfsvol.DeviceID, btrfssum.SumRun[btrfsvol.PhysicalAddr]) error,
) error {
	for _, devID := range maps.SortedKeys(gaps) {
		for _, gap := range gaps[devID] {
			if err := ctx.Err(); err != nil {
				return err
			}
			begAddr := roundUp(gap.Beg, btrfssum.BlockSize)
			begOff := int(begAddr/btrfssum.BlockSize) * physicalSums[devID].ChecksumSize
			endOff := int(gap.End/btrfssum.BlockSize) * physicalSums[devID].ChecksumSize
			if err := fn(devID, btrfssum.SumRun[btrfsvol.PhysicalAddr]{
				ChecksumSize: physicalSums[devID].ChecksumSize,
				Addr:         begAddr,
				Sums:         physicalSums[devID].Sums[begOff:endOff],
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
