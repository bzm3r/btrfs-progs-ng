// Code generated by Make.  DO NOT EDIT.

package btrfsitem

import (
	"reflect"

	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsprim"
)

const (
	BLOCK_GROUP_ITEM_KEY     = btrfsprim.BLOCK_GROUP_ITEM_KEY
	CHUNK_ITEM_KEY           = btrfsprim.CHUNK_ITEM_KEY
	DEV_EXTENT_KEY           = btrfsprim.DEV_EXTENT_KEY
	DEV_ITEM_KEY             = btrfsprim.DEV_ITEM_KEY
	DIR_INDEX_KEY            = btrfsprim.DIR_INDEX_KEY
	DIR_ITEM_KEY             = btrfsprim.DIR_ITEM_KEY
	EXTENT_CSUM_KEY          = btrfsprim.EXTENT_CSUM_KEY
	EXTENT_DATA_KEY          = btrfsprim.EXTENT_DATA_KEY
	EXTENT_DATA_REF_KEY      = btrfsprim.EXTENT_DATA_REF_KEY
	EXTENT_ITEM_KEY          = btrfsprim.EXTENT_ITEM_KEY
	FREE_SPACE_BITMAP_KEY    = btrfsprim.FREE_SPACE_BITMAP_KEY
	FREE_SPACE_EXTENT_KEY    = btrfsprim.FREE_SPACE_EXTENT_KEY
	FREE_SPACE_INFO_KEY      = btrfsprim.FREE_SPACE_INFO_KEY
	INODE_ITEM_KEY           = btrfsprim.INODE_ITEM_KEY
	INODE_REF_KEY            = btrfsprim.INODE_REF_KEY
	METADATA_ITEM_KEY        = btrfsprim.METADATA_ITEM_KEY
	ORPHAN_ITEM_KEY          = btrfsprim.ORPHAN_ITEM_KEY
	PERSISTENT_ITEM_KEY      = btrfsprim.PERSISTENT_ITEM_KEY
	QGROUP_RELATION_KEY      = btrfsprim.QGROUP_RELATION_KEY
	ROOT_BACKREF_KEY         = btrfsprim.ROOT_BACKREF_KEY
	ROOT_ITEM_KEY            = btrfsprim.ROOT_ITEM_KEY
	ROOT_REF_KEY             = btrfsprim.ROOT_REF_KEY
	SHARED_BLOCK_REF_KEY     = btrfsprim.SHARED_BLOCK_REF_KEY
	SHARED_DATA_REF_KEY      = btrfsprim.SHARED_DATA_REF_KEY
	TREE_BLOCK_REF_KEY       = btrfsprim.TREE_BLOCK_REF_KEY
	UNTYPED_KEY              = btrfsprim.UNTYPED_KEY
	UUID_RECEIVED_SUBVOL_KEY = btrfsprim.UUID_RECEIVED_SUBVOL_KEY
	UUID_SUBVOL_KEY          = btrfsprim.UUID_SUBVOL_KEY
	XATTR_ITEM_KEY           = btrfsprim.XATTR_ITEM_KEY
)

var keytype2gotype = map[Type]reflect.Type{
	BLOCK_GROUP_ITEM_KEY:     reflect.TypeOf(BlockGroup{}),
	CHUNK_ITEM_KEY:           reflect.TypeOf(Chunk{}),
	DEV_EXTENT_KEY:           reflect.TypeOf(DevExtent{}),
	DEV_ITEM_KEY:             reflect.TypeOf(Dev{}),
	DIR_INDEX_KEY:            reflect.TypeOf(DirEntry{}),
	DIR_ITEM_KEY:             reflect.TypeOf(DirEntry{}),
	EXTENT_CSUM_KEY:          reflect.TypeOf(ExtentCSum{}),
	EXTENT_DATA_KEY:          reflect.TypeOf(FileExtent{}),
	EXTENT_DATA_REF_KEY:      reflect.TypeOf(ExtentDataRef{}),
	EXTENT_ITEM_KEY:          reflect.TypeOf(Extent{}),
	FREE_SPACE_BITMAP_KEY:    reflect.TypeOf(FreeSpaceBitmap{}),
	FREE_SPACE_EXTENT_KEY:    reflect.TypeOf(Empty{}),
	FREE_SPACE_INFO_KEY:      reflect.TypeOf(FreeSpaceInfo{}),
	INODE_ITEM_KEY:           reflect.TypeOf(Inode{}),
	INODE_REF_KEY:            reflect.TypeOf(InodeRefs{}),
	METADATA_ITEM_KEY:        reflect.TypeOf(Metadata{}),
	ORPHAN_ITEM_KEY:          reflect.TypeOf(Empty{}),
	PERSISTENT_ITEM_KEY:      reflect.TypeOf(DevStats{}),
	QGROUP_RELATION_KEY:      reflect.TypeOf(Empty{}),
	ROOT_BACKREF_KEY:         reflect.TypeOf(RootRef{}),
	ROOT_ITEM_KEY:            reflect.TypeOf(Root{}),
	ROOT_REF_KEY:             reflect.TypeOf(RootRef{}),
	SHARED_BLOCK_REF_KEY:     reflect.TypeOf(Empty{}),
	SHARED_DATA_REF_KEY:      reflect.TypeOf(SharedDataRef{}),
	TREE_BLOCK_REF_KEY:       reflect.TypeOf(Empty{}),
	UUID_RECEIVED_SUBVOL_KEY: reflect.TypeOf(UUIDMap{}),
	UUID_SUBVOL_KEY:          reflect.TypeOf(UUIDMap{}),
	XATTR_ITEM_KEY:           reflect.TypeOf(DirEntry{}),
}
var untypedObjID2gotype = map[btrfsprim.ObjID]reflect.Type{
	btrfsprim.FREE_SPACE_OBJECTID: reflect.TypeOf(FreeSpaceHeader{}),
}

func (BlockGroup) isItem()      {}
func (Chunk) isItem()           {}
func (Dev) isItem()             {}
func (DevExtent) isItem()       {}
func (DevStats) isItem()        {}
func (DirEntry) isItem()        {}
func (Empty) isItem()           {}
func (Extent) isItem()          {}
func (ExtentCSum) isItem()      {}
func (ExtentDataRef) isItem()   {}
func (FileExtent) isItem()      {}
func (FreeSpaceBitmap) isItem() {}
func (FreeSpaceHeader) isItem() {}
func (FreeSpaceInfo) isItem()   {}
func (Inode) isItem()           {}
func (InodeRefs) isItem()       {}
func (Metadata) isItem()        {}
func (Root) isItem()            {}
func (RootRef) isItem()         {}
func (SharedDataRef) isItem()   {}
func (UUIDMap) isItem()         {}
