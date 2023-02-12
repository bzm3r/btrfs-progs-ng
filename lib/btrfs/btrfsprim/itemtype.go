// Code generated by Make.  DO NOT EDIT.

package btrfsprim

import "fmt"

type ItemType uint8

const (
	BLOCK_GROUP_ITEM_KEY     ItemType = 192
	CHUNK_ITEM_KEY           ItemType = 228
	DEV_EXTENT_KEY           ItemType = 204
	DEV_ITEM_KEY             ItemType = 216
	DIR_INDEX_KEY            ItemType = 96
	DIR_ITEM_KEY             ItemType = 84
	EXTENT_CSUM_KEY          ItemType = 128
	EXTENT_DATA_KEY          ItemType = 108
	EXTENT_DATA_REF_KEY      ItemType = 178
	EXTENT_ITEM_KEY          ItemType = 168
	FREE_SPACE_BITMAP_KEY    ItemType = 200
	FREE_SPACE_EXTENT_KEY    ItemType = 199
	FREE_SPACE_INFO_KEY      ItemType = 198
	INODE_ITEM_KEY           ItemType = 1
	INODE_REF_KEY            ItemType = 12
	METADATA_ITEM_KEY        ItemType = 169
	ORPHAN_ITEM_KEY          ItemType = 48
	PERSISTENT_ITEM_KEY      ItemType = 249
	QGROUP_INFO_KEY          ItemType = 242
	QGROUP_LIMIT_KEY         ItemType = 244
	QGROUP_RELATION_KEY      ItemType = 246
	QGROUP_STATUS_KEY        ItemType = 240
	ROOT_BACKREF_KEY         ItemType = 144
	ROOT_ITEM_KEY            ItemType = 132
	ROOT_REF_KEY             ItemType = 156
	SHARED_BLOCK_REF_KEY     ItemType = 182
	SHARED_DATA_REF_KEY      ItemType = 184
	TREE_BLOCK_REF_KEY       ItemType = 176
	UNTYPED_KEY              ItemType = 0
	UUID_RECEIVED_SUBVOL_KEY ItemType = 252
	UUID_SUBVOL_KEY          ItemType = 251
	XATTR_ITEM_KEY           ItemType = 24
)

func (t ItemType) String() string {
	switch t {
	case BLOCK_GROUP_ITEM_KEY:
		return "BLOCK_GROUP_ITEM"
	case CHUNK_ITEM_KEY:
		return "CHUNK_ITEM"
	case DEV_EXTENT_KEY:
		return "DEV_EXTENT"
	case DEV_ITEM_KEY:
		return "DEV_ITEM"
	case DIR_INDEX_KEY:
		return "DIR_INDEX"
	case DIR_ITEM_KEY:
		return "DIR_ITEM"
	case EXTENT_CSUM_KEY:
		return "EXTENT_CSUM"
	case EXTENT_DATA_KEY:
		return "EXTENT_DATA"
	case EXTENT_DATA_REF_KEY:
		return "EXTENT_DATA_REF"
	case EXTENT_ITEM_KEY:
		return "EXTENT_ITEM"
	case FREE_SPACE_BITMAP_KEY:
		return "FREE_SPACE_BITMAP"
	case FREE_SPACE_EXTENT_KEY:
		return "FREE_SPACE_EXTENT"
	case FREE_SPACE_INFO_KEY:
		return "FREE_SPACE_INFO"
	case INODE_ITEM_KEY:
		return "INODE_ITEM"
	case INODE_REF_KEY:
		return "INODE_REF"
	case METADATA_ITEM_KEY:
		return "METADATA_ITEM"
	case ORPHAN_ITEM_KEY:
		return "ORPHAN_ITEM"
	case PERSISTENT_ITEM_KEY:
		return "PERSISTENT_ITEM"
	case QGROUP_INFO_KEY:
		return "QGROUP_INFO"
	case QGROUP_LIMIT_KEY:
		return "QGROUP_LIMIT"
	case QGROUP_RELATION_KEY:
		return "QGROUP_RELATION"
	case QGROUP_STATUS_KEY:
		return "QGROUP_STATUS"
	case ROOT_BACKREF_KEY:
		return "ROOT_BACKREF"
	case ROOT_ITEM_KEY:
		return "ROOT_ITEM"
	case ROOT_REF_KEY:
		return "ROOT_REF"
	case SHARED_BLOCK_REF_KEY:
		return "SHARED_BLOCK_REF"
	case SHARED_DATA_REF_KEY:
		return "SHARED_DATA_REF"
	case TREE_BLOCK_REF_KEY:
		return "TREE_BLOCK_REF"
	case UNTYPED_KEY:
		return "UNTYPED"
	case UUID_RECEIVED_SUBVOL_KEY:
		return "UUID_KEY_RECEIVED_SUBVOL"
	case UUID_SUBVOL_KEY:
		return "UUID_KEY_SUBVOL"
	case XATTR_ITEM_KEY:
		return "XATTR_ITEM"
	default:
		return fmt.Sprintf("%d", t)
	}
}
