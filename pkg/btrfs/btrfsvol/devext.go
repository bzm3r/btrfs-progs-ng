package btrfsvol

import (
	"fmt"

	"lukeshu.com/btrfs-tools/pkg/util"
)

// physical => logical
type devextMapping struct {
	PAddr      PhysicalAddr
	LAddr      LogicalAddr
	Size       AddrDelta
	SizeLocked bool
	Flags      *BlockGroupFlags
}

// return -1 if 'a' is wholly to the left of 'b'
// return 0 if there is some overlap between 'a' and 'b'
// return 1 if 'a is wholly to the right of 'b'
func (a devextMapping) cmpRange(b devextMapping) int {
	switch {
	case a.PAddr.Add(a.Size) <= b.PAddr:
		// 'a' is wholly to the left of 'b'.
		return -1
	case b.PAddr.Add(b.Size) <= a.PAddr:
		// 'a' is wholly to the right of 'b'.
		return 1
	default:
		// There is some overlap.
		return 0
	}
}

func (a devextMapping) union(rest ...devextMapping) (devextMapping, error) {
	// sanity check
	for _, ext := range rest {
		if a.cmpRange(ext) != 0 {
			return devextMapping{}, fmt.Errorf("devexts don't overlap")
		}
	}
	exts := append([]devextMapping{a}, rest...)
	// figure out the physical range (.PAddr and .Size)
	beg := exts[0].PAddr
	end := beg.Add(exts[0].Size)
	for _, ext := range exts {
		beg = util.Min(beg, ext.PAddr)
		end = util.Max(end, ext.PAddr.Add(ext.Size))
	}
	ret := devextMapping{
		PAddr: beg,
		Size:  end.Sub(beg),
	}
	for _, ext := range exts {
		if ext.SizeLocked {
			ret.SizeLocked = true
			if ret.Size != ext.Size {
				return devextMapping{}, fmt.Errorf("member devext has locked size=%v, but union would have size=%v",
					ext.Size, ret.Size)
			}
		}
	}
	// figure out the logical range (.LAddr)
	first := true
	for _, ext := range exts {
		offsetWithinRet := ext.PAddr.Sub(ret.PAddr)
		laddr := ext.LAddr.Add(-offsetWithinRet)
		if first {
			ret.LAddr = laddr
		} else if laddr != ret.LAddr {
			return ret, fmt.Errorf("devexts don't agree on laddr: %v != %v", ret.LAddr, laddr)
		}
	}
	// figure out the flags (.Flags)
	for _, ext := range exts {
		if ext.Flags == nil {
			continue
		}
		if ret.Flags == nil {
			val := *ext.Flags
			ret.Flags = &val
		}
		if *ret.Flags != *ext.Flags {
			return ret, fmt.Errorf("mismatch flags: %v != %v", *ret.Flags, *ext.Flags)
		}
	}
	// done
	return ret, nil
}
