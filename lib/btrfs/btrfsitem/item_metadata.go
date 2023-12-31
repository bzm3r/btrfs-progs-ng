// Copyright (C) 2022-2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package btrfsitem

import (
	"git.lukeshu.com/btrfs-progs-ng/lib/binstruct"
)

// Metadata items map from regions in the logical address space to
// regions in a file.
//
// Metadata is like Extent, but doesn't have .Info.
//
// Compare with:
//
//   - Extents, which are the same as Metadata, but have an extra
//     .Info member.
//   - FileExtents, which map from regions in a file to regions in the
//     logical address space.
//
// Key:
//
//	key.objectid = laddr of the extent
//	key.offset   = length of the extent
type Metadata struct { // complex METADATA_ITEM=169
	Head ExtentHeader
	Refs []ExtentInlineRef
}

func (o *Metadata) Free() {
	for i := range o.Refs {
		if o.Refs[i].Body != nil {
			o.Refs[i].Body.Free()
		}
		o.Refs[i] = ExtentInlineRef{}
	}
	extentInlineRefPool.Put(o.Refs)
	*o = Metadata{}
	metadataPool.Put(o)
}

func (o Metadata) Clone() Metadata {
	ret := o
	ret.Refs = extentInlineRefPool.Get(len(o.Refs))
	copy(ret.Refs, o.Refs)
	for i := range ret.Refs {
		if o.Refs[i].Body != nil {
			ret.Refs[i].Body = o.Refs[i].Body.CloneItem()
		}
	}
	return ret
}

func (o *Metadata) UnmarshalBinary(dat []byte) (int, error) {
	*o = Metadata{}
	n, err := binstruct.Unmarshal(dat, &o.Head)
	if err != nil {
		return n, err
	}
	if n < len(dat) {
		o.Refs = extentInlineRefPool.Get(1)[:0]
	}
	for n < len(dat) {
		var ref ExtentInlineRef
		_n, err := binstruct.Unmarshal(dat[n:], &ref)
		n += _n
		o.Refs = append(o.Refs, ref)
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (o Metadata) MarshalBinary() ([]byte, error) {
	dat, err := binstruct.Marshal(o.Head)
	if err != nil {
		return dat, err
	}
	for _, ref := range o.Refs {
		bs, err := binstruct.Marshal(ref)
		dat = append(dat, bs...)
		if err != nil {
			return dat, err
		}
	}
	return dat, nil
}
