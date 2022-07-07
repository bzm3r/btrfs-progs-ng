package btrfsitem

import (
	"lukeshu.com/btrfs-tools/pkg/binstruct"
	"lukeshu.com/btrfs-tools/pkg/btrfs/internal"
)

// The Key for this item is a UUID, and the item is a subvolume IDs
// that that UUID maps to.
//
// key.objectid = first half of UUID
// key.offset = second half of UUID
type UUIDMap struct { // UUID_SUBVOL=251 UUID_RECEIVED_SUBVOL=252
	ObjID         internal.ObjID `bin:"off=0, siz=8"`
	binstruct.End `bin:"off=8"`
}
