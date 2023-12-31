// Copyright (C) 2022-2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

// Package btrfsvol contains core logical-volume-management layer of
// btrfs.
package btrfsvol

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"

	"github.com/datawire/dlib/derror"

	"git.lukeshu.com/btrfs-progs-ng/lib/containers"
	"git.lukeshu.com/btrfs-progs-ng/lib/diskio"
	"git.lukeshu.com/btrfs-progs-ng/lib/maps"
)

type LogicalVolume[PhysicalVolume diskio.File[PhysicalAddr]] struct {
	name string

	id2pv map[DeviceID]PhysicalVolume

	logical2physical *containers.RBTree[chunkMapping]
	physical2logical map[DeviceID]*containers.RBTree[devextMapping]
}

var _ diskio.File[LogicalAddr] = (*LogicalVolume[diskio.File[PhysicalAddr]])(nil)

func (lv *LogicalVolume[PhysicalVolume]) init() {
	if lv.id2pv == nil {
		lv.id2pv = make(map[DeviceID]PhysicalVolume)
	}
	if lv.logical2physical == nil {
		lv.logical2physical = new(containers.RBTree[chunkMapping])
	}
	if lv.physical2logical == nil {
		lv.physical2logical = make(map[DeviceID]*containers.RBTree[devextMapping], len(lv.id2pv))
	}
	for devid := range lv.id2pv {
		if !maps.HasKey(lv.physical2logical, devid) {
			lv.physical2logical[devid] = new(containers.RBTree[devextMapping])
		}
	}
}

func (lv *LogicalVolume[PhysicalVolume]) SetName(name string) {
	lv.name = name
}

func (lv *LogicalVolume[PhysicalVolume]) Name() string {
	return lv.name
}

func (lv *LogicalVolume[PhysicalVolume]) Size() LogicalAddr {
	lv.init()
	lastChunk := lv.logical2physical.Max()
	if lastChunk == nil {
		return 0
	}
	return lastChunk.Value.LAddr.Add(lastChunk.Value.Size)
}

func (lv *LogicalVolume[PhysicalVolume]) Close() error {
	var errs derror.MultiError
	for _, dev := range lv.id2pv {
		if err := dev.Close(); err != nil && err == nil {
			errs = append(errs, err)
		}
	}
	if errs != nil {
		return errs
	}
	return nil
}

func (lv *LogicalVolume[PhysicalVolume]) AddPhysicalVolume(id DeviceID, dev PhysicalVolume) error {
	lv.init()
	if other, exists := lv.id2pv[id]; exists {
		return fmt.Errorf("(%p).AddPhysicalVolume: cannot add physical volume %q: already have physical volume %q with id=%v",
			lv, dev.Name(), other.Name(), id)
	}
	lv.id2pv[id] = dev
	lv.physical2logical[id] = new(containers.RBTree[devextMapping])
	return nil
}

func (lv *LogicalVolume[PhysicalVolume]) PhysicalVolumes() map[DeviceID]PhysicalVolume {
	dup := make(map[DeviceID]PhysicalVolume, len(lv.id2pv))
	for k, v := range lv.id2pv {
		dup[k] = v
	}
	return dup
}

func (lv *LogicalVolume[PhysicalVolume]) ClearMappings() {
	lv.logical2physical = nil
	lv.physical2logical = nil
}

type Mapping struct {
	LAddr      LogicalAddr
	PAddr      QualifiedPhysicalAddr
	Size       AddrDelta
	SizeLocked bool                                 `json:",omitempty"`
	Flags      containers.Optional[BlockGroupFlags] `json:",omitempty"`
}

func (lv *LogicalVolume[PhysicalVolume]) CouldAddMapping(m Mapping) bool {
	return lv.addMapping(m, true) == nil
}

func (lv *LogicalVolume[PhysicalVolume]) AddMapping(m Mapping) error {
	return lv.addMapping(m, false)
}

func (lv *LogicalVolume[PhysicalVolume]) addMapping(m Mapping, dryRun bool) error {
	lv.init()
	// sanity check
	if !maps.HasKey(lv.id2pv, m.PAddr.Dev) {
		return fmt.Errorf("(%p).AddMapping: do not have a physical volume with id=%v",
			lv, m.PAddr.Dev)
	}

	// logical2physical
	newChunk := chunkMapping{
		LAddr:      m.LAddr,
		PAddrs:     []QualifiedPhysicalAddr{m.PAddr},
		Size:       m.Size,
		SizeLocked: m.SizeLocked,
		Flags:      m.Flags,
	}
	var logicalOverlaps []chunkMapping
	numOverlappingStripes := 0
	lv.logical2physical.Subrange(newChunk.compareRange, func(node *containers.RBNode[chunkMapping]) bool {
		logicalOverlaps = append(logicalOverlaps, node.Value)
		numOverlappingStripes += len(node.Value.PAddrs)
		return true
	})
	var err error
	newChunk, err = newChunk.union(logicalOverlaps...)
	if err != nil {
		return fmt.Errorf("(%p).AddMapping: %w", lv, err)
	}

	// physical2logical
	newExt := devextMapping{
		PAddr:      m.PAddr.Addr,
		LAddr:      m.LAddr,
		Size:       m.Size,
		SizeLocked: m.SizeLocked,
		Flags:      m.Flags,
	}
	var physicalOverlaps []devextMapping
	lv.physical2logical[m.PAddr.Dev].Subrange(newExt.compareRange, func(node *containers.RBNode[devextMapping]) bool {
		physicalOverlaps = append(physicalOverlaps, node.Value)
		return true
	})
	newExt, err = newExt.union(physicalOverlaps...)
	if err != nil {
		return fmt.Errorf("(%p).AddMapping: %w", lv, err)
	}

	if newChunk.Flags != newExt.Flags {
		// If these don't match up, it's a bug in this code.
		panic(fmt.Errorf("should not happen: newChunk.Flags:%+v != newExt.Flags:%+v",
			newChunk.Flags, newExt.Flags))
	}
	switch {
	case len(physicalOverlaps) == numOverlappingStripes:
		// normal case
	case len(physicalOverlaps) < numOverlappingStripes:
		// .Flags = DUP or RAID{X}
		if newChunk.Flags.OK && newChunk.Flags.Val&BLOCK_GROUP_RAID_MASK == 0 {
			return fmt.Errorf("multiple stripes but flags=%v does not allow multiple stripes",
				newChunk.Flags.Val)
		}
	case len(physicalOverlaps) > numOverlappingStripes:
		// This should not happen because calling .AddMapping
		// should update the two in lockstep; if these don't
		// match up, it's a bug in this code.
		panic(fmt.Errorf("should not happen: len(physicalOverlaps):%d != numOverlappingStripes:%d",
			len(physicalOverlaps), numOverlappingStripes))
	}

	if dryRun {
		return nil
	}

	// optimize
	if len(logicalOverlaps) == 1 && reflect.DeepEqual(newChunk, logicalOverlaps[0]) &&
		len(physicalOverlaps) == 1 && reflect.DeepEqual(newExt, physicalOverlaps[0]) {
		return nil
	}

	// logical2physical
	for _, chunk := range logicalOverlaps {
		lv.logical2physical.Delete(lv.logical2physical.Search(chunk.Compare))
	}
	lv.logical2physical.Insert(newChunk)

	// physical2logical
	for _, ext := range physicalOverlaps {
		lv.physical2logical[m.PAddr.Dev].Delete(lv.physical2logical[m.PAddr.Dev].Search(ext.Compare))
	}
	lv.physical2logical[m.PAddr.Dev].Insert(newExt)

	// sanity check
	//
	// This is in-theory unnescessary, but that assumes that I
	// made no mistakes in my algorithm above.
	if os.Getenv("PARANOID") != "" {
		if err := lv.fsck(); err != nil {
			return err
		}
	}

	// done
	return nil
}

func (lv *LogicalVolume[PhysicalVolume]) fsck() error {
	physical2logical := make(map[DeviceID]*containers.RBTree[devextMapping])
	var err error
	lv.logical2physical.Range(func(node *containers.RBNode[chunkMapping]) bool {
		chunk := node.Value
		for _, stripe := range chunk.PAddrs {
			if !maps.HasKey(lv.id2pv, stripe.Dev) {
				err = fmt.Errorf("(%p).fsck: chunk references physical volume %v which does not exist",
					lv, stripe.Dev)
				return false
			}
			if !maps.HasKey(physical2logical, stripe.Dev) {
				physical2logical[stripe.Dev] = new(containers.RBTree[devextMapping])
			}
			physical2logical[stripe.Dev].Insert(devextMapping{
				PAddr: stripe.Addr,
				LAddr: chunk.LAddr,
				Size:  chunk.Size,
				Flags: chunk.Flags,
			})
		}
		return true
	})
	if err != nil {
		return err
	}

	if len(lv.physical2logical) != len(physical2logical) {
		return fmt.Errorf("(%p).fsck: skew between chunk tree and devext tree",
			lv)
	}
	for devid := range lv.physical2logical {
		if !lv.physical2logical[devid].Equal(physical2logical[devid]) {
			return fmt.Errorf("(%p).fsck: skew between chunk tree and devext tree",
				lv)
		}
	}

	return nil
}

func (lv *LogicalVolume[PhysicalVolume]) Mappings() []Mapping {
	var ret []Mapping
	lv.logical2physical.Range(func(node *containers.RBNode[chunkMapping]) bool {
		chunk := node.Value
		for _, slice := range chunk.PAddrs {
			ret = append(ret, Mapping{
				LAddr: chunk.LAddr,
				PAddr: slice,
				Size:  chunk.Size,
				Flags: chunk.Flags,
			})
		}
		return true
	})
	return ret
}

func (lv *LogicalVolume[PhysicalVolume]) Resolve(laddr LogicalAddr) (paddrs containers.Set[QualifiedPhysicalAddr], maxlen AddrDelta) {
	node := lv.logical2physical.Search(func(chunk chunkMapping) int {
		return chunkMapping{LAddr: laddr, Size: 1}.compareRange(chunk)
	})
	if node == nil {
		return nil, 0
	}

	chunk := node.Value

	offsetWithinChunk := laddr.Sub(chunk.LAddr)
	paddrs = make(containers.Set[QualifiedPhysicalAddr])
	maxlen = chunk.Size - offsetWithinChunk
	for _, stripe := range chunk.PAddrs {
		paddrs.Insert(stripe.Add(offsetWithinChunk))
	}

	return paddrs, maxlen
}

func (lv *LogicalVolume[PhysicalVolume]) ResolveAny(laddr LogicalAddr, size AddrDelta) (LogicalAddr, QualifiedPhysicalAddr) {
	node := lv.logical2physical.Search(func(chunk chunkMapping) int {
		return chunkMapping{LAddr: laddr, Size: size}.compareRange(chunk)
	})
	if node == nil {
		return -1, QualifiedPhysicalAddr{0, -1}
	}
	return node.Value.LAddr, node.Value.PAddrs[0]
}

func (lv *LogicalVolume[PhysicalVolume]) UnResolve(paddr QualifiedPhysicalAddr) LogicalAddr {
	node := lv.physical2logical[paddr.Dev].Search(func(ext devextMapping) int {
		return devextMapping{PAddr: paddr.Addr, Size: 1}.compareRange(ext)
	})
	if node == nil {
		return -1
	}

	ext := node.Value

	offsetWithinExt := paddr.Addr.Sub(ext.PAddr)
	return ext.LAddr.Add(offsetWithinExt)
}

func (lv *LogicalVolume[PhysicalVolume]) ReadAt(dat []byte, laddr LogicalAddr) (int, error) {
	done := 0
	for done < len(dat) {
		n, err := lv.maybeShortReadAt(dat[done:], laddr+LogicalAddr(done))
		done += n
		if err != nil {
			return done, err
		}
	}
	return done, nil
}

var ErrCouldNotMap = errors.New("could not map logical address")

func (lv *LogicalVolume[PhysicalVolume]) maybeShortReadAt(dat []byte, laddr LogicalAddr) (int, error) {
	paddrs, maxlen := lv.Resolve(laddr)
	if len(paddrs) == 0 {
		return 0, fmt.Errorf("read: %w %v", ErrCouldNotMap, laddr)
	}
	if AddrDelta(len(dat)) > maxlen {
		dat = dat[:maxlen]
	}

	buf := dat
	first := true
	for paddr := range paddrs {
		dev, ok := lv.id2pv[paddr.Dev]
		if !ok {
			return 0, fmt.Errorf("device=%v does not exist", paddr.Dev)
		}
		if !first {
			buf = make([]byte, len(buf))
		}
		if _, err := dev.ReadAt(buf, paddr.Addr); err != nil {
			return 0, fmt.Errorf("read device=%v paddr=%v: %w", paddr.Dev, paddr.Addr, err)
		}
		if !first && !bytes.Equal(dat, buf) {
			return 0, fmt.Errorf("inconsistent stripes at laddr=%v len=%v", laddr, len(dat))
		}
		first = false
	}
	return len(dat), nil
}

func (lv *LogicalVolume[PhysicalVolume]) WriteAt(dat []byte, laddr LogicalAddr) (int, error) {
	done := 0
	for done < len(dat) {
		n, err := lv.maybeShortWriteAt(dat[done:], laddr+LogicalAddr(done))
		done += n
		if err != nil {
			return done, err
		}
	}
	return done, nil
}

func (lv *LogicalVolume[PhysicalVolume]) maybeShortWriteAt(dat []byte, laddr LogicalAddr) (int, error) {
	paddrs, maxlen := lv.Resolve(laddr)
	if len(paddrs) == 0 {
		return 0, fmt.Errorf("write: %w %v", ErrCouldNotMap, laddr)
	}
	if AddrDelta(len(dat)) > maxlen {
		dat = dat[:maxlen]
	}

	for paddr := range paddrs {
		dev, ok := lv.id2pv[paddr.Dev]
		if !ok {
			return 0, fmt.Errorf("device=%v does not exist", paddr.Dev)
		}
		if _, err := dev.WriteAt(dat, paddr.Addr); err != nil {
			return 0, fmt.Errorf("write device=%v paddr=%v: %w", paddr.Dev, paddr.Addr, err)
		}
	}
	return len(dat), nil
}
