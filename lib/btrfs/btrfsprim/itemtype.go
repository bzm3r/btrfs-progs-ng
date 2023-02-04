// Code generated by Make.  DO NOT EDIT.

package btrfsprim

import "fmt"

type ItemType uint8

const (
	BLOCK_GROUP_ITEM_KEY     = ItemType(192)
	CHUNK_ITEM_KEY           = ItemType(228)
	DEV_EXTENT_KEY           = ItemType(204)
	DEV_ITEM_KEY             = ItemType(216)
	DIR_INDEX_KEY            = ItemType(96)
	DIR_ITEM_KEY             = ItemType(84)
	EXTENT_CSUM_KEY          = ItemType(128)
	EXTENT_DATA_KEY          = ItemType(108)
	EXTENT_DATA_REF_KEY      = ItemType(178)
	EXTENT_ITEM_KEY          = ItemType(168)
	FREE_SPACE_BITMAP_KEY    = ItemType(200)
	FREE_SPACE_EXTENT_KEY    = ItemType(199)
	FREE_SPACE_INFO_KEY      = ItemType(198)
	INODE_ITEM_KEY           = ItemType(1)
	INODE_REF_KEY            = ItemType(12)
	METADATA_ITEM_KEY        = ItemType(169)
	ORPHAN_ITEM_KEY          = ItemType(48)
	PERSISTENT_ITEM_KEY      = ItemType(249)
	QGROUP_RELATION_KEY      = ItemType(246)
	ROOT_BACKREF_KEY         = ItemType(144)
	ROOT_ITEM_KEY            = ItemType(132)
	ROOT_REF_KEY             = ItemType(156)
	SHARED_BLOCK_REF_KEY     = ItemType(182)
	SHARED_DATA_REF_KEY      = ItemType(184)
	TREE_BLOCK_REF_KEY       = ItemType(176)
	UNTYPED_KEY              = ItemType(0)
	UUID_RECEIVED_SUBVOL_KEY = ItemType(252)
	UUID_SUBVOL_KEY          = ItemType(251)
	XATTR_ITEM_KEY           = ItemType(24)
)

func (t ItemType) String() string {
	names := map[ItemType]string{
		BLOCK_GROUP_ITEM_KEY:     "BLOCK_GROUP_ITEM",
		CHUNK_ITEM_KEY:           "CHUNK_ITEM",
		DEV_EXTENT_KEY:           "DEV_EXTENT",
		DEV_ITEM_KEY:             "DEV_ITEM",
		DIR_INDEX_KEY:            "DIR_INDEX",
		DIR_ITEM_KEY:             "DIR_ITEM",
		EXTENT_CSUM_KEY:          "EXTENT_CSUM",
		EXTENT_DATA_KEY:          "EXTENT_DATA",
		EXTENT_DATA_REF_KEY:      "EXTENT_DATA_REF",
		EXTENT_ITEM_KEY:          "EXTENT_ITEM",
		FREE_SPACE_BITMAP_KEY:    "FREE_SPACE_BITMAP",
		FREE_SPACE_EXTENT_KEY:    "FREE_SPACE_EXTENT",
		FREE_SPACE_INFO_KEY:      "FREE_SPACE_INFO",
		INODE_ITEM_KEY:           "INODE_ITEM",
		INODE_REF_KEY:            "INODE_REF",
		METADATA_ITEM_KEY:        "METADATA_ITEM",
		ORPHAN_ITEM_KEY:          "ORPHAN_ITEM",
		PERSISTENT_ITEM_KEY:      "PERSISTENT_ITEM",
		QGROUP_RELATION_KEY:      "QGROUP_RELATION",
		ROOT_BACKREF_KEY:         "ROOT_BACKREF",
		ROOT_ITEM_KEY:            "ROOT_ITEM",
		ROOT_REF_KEY:             "ROOT_REF",
		SHARED_BLOCK_REF_KEY:     "SHARED_BLOCK_REF",
		SHARED_DATA_REF_KEY:      "SHARED_DATA_REF",
		TREE_BLOCK_REF_KEY:       "TREE_BLOCK_REF",
		UNTYPED_KEY:              "UNTYPED",
		UUID_RECEIVED_SUBVOL_KEY: "UUID_KEY_RECEIVED_SUBVOL",
		UUID_SUBVOL_KEY:          "UUID_KEY_SUBVOL",
		XATTR_ITEM_KEY:           "XATTR_ITEM",
	}
	if name, ok := names[t]; ok {
		return name
	}
	return fmt.Sprintf("%d", t)
}
