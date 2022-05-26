package btrfs

import (
	"fmt"
	"reflect"
	"time"

	"lukeshu.com/btrfs-tools/pkg/binstruct"
)

type (
	PhysicalAddr int64
	LogicalAddr  int64
	Generation   uint64
)

type Key struct {
	ObjectID      ObjID    `bin:"off=0, siz=8"` // Each tree has its own set of Object IDs.
	ItemType      ItemType `bin:"off=8, siz=1"`
	Offset        uint64   `bin:"off=9, siz=8"` // The meaning depends on the item type.
	binstruct.End `bin:"off=11"`
}

type Time struct {
	Sec           int64  `bin:"off=0, siz=8"` // Number of seconds since 1970-01-01T00:00:00Z.
	NSec          uint64 `bin:"off=8, siz=4"` // Number of nanoseconds since the beginning of the second.
	binstruct.End `bin:"off=c"`
}

func (t Time) ToStd() time.Time {
	return time.Unix(t.Sec, int64(t.NSec))
}

type Superblock struct {
	Checksum   CSum         `bin:"off=0,  siz=20"` // Checksum of everything past this field (from 20 to 1000)
	FSUUID     UUID         `bin:"off=20, siz=10"` // FS UUID
	Self       PhysicalAddr `bin:"off=30, siz=8"`  // physical address of this block (different for mirrors)
	Flags      uint64       `bin:"off=38, siz=8"`  // flags
	Magic      [8]byte      `bin:"off=40, siz=8"`  // magic ('_BHRfS_M')
	Generation Generation   `bin:"off=48, siz=8"`

	RootTree  LogicalAddr `bin:"off=50, siz=8"` // logical address of the root tree root
	ChunkTree LogicalAddr `bin:"off=58, siz=8"` // logical address of the chunk tree root
	LogTree   LogicalAddr `bin:"off=60, siz=8"` // logical address of the log tree root

	LogRootTransID  uint64 `bin:"off=68, siz=8"` // log_root_transid
	TotalBytes      uint64 `bin:"off=70, siz=8"` // total_bytes
	BytesUsed       uint64 `bin:"off=78, siz=8"` // bytes_used
	RootDirObjectID ObjID  `bin:"off=80, siz=8"` // root_dir_objectid (usually 6)
	NumDevices      uint64 `bin:"off=88, siz=8"` // num_devices

	SectorSize        uint32 `bin:"off=90, siz=4"`
	NodeSize          uint32 `bin:"off=94, siz=4"`
	LeafSize          uint32 `bin:"off=98, siz=4"` // unused; must be the same as NodeSize
	StripeSize        uint32 `bin:"off=9c, siz=4"`
	SysChunkArraySize uint32 `bin:"off=a0, siz=4"`

	ChunkRootGeneration Generation    `bin:"off=a4, siz=8"`
	CompatFlags         uint64        `bin:"off=ac, siz=8"` // compat_flags
	CompatROFlags       uint64        `bin:"off=b4, siz=8"` // compat_ro_flags - only implementations that support the flags can write to the filesystem
	IncompatFlags       IncompatFlags `bin:"off=bc, siz=8"` // incompat_flags - only implementations that support the flags can use the filesystem
	ChecksumType        uint16        `bin:"off=c4, siz=2"` // csum_type - Btrfs currently uses the CRC32c little-endian hash function with seed -1.

	RootLevel  uint8 `bin:"off=c6, siz=1"` // root_level
	ChunkLevel uint8 `bin:"off=c7, siz=1"` // chunk_root_level
	LogLevel   uint8 `bin:"off=c8, siz=1"` // log_root_level

	DevItem            DevItem     `bin:"off=c9,  siz=62"`  // DEV_ITEM data for this device
	Label              [0x100]byte `bin:"off=12b, siz=100"` // label (may not contain '/' or '\\')
	CacheGeneration    Generation  `bin:"off=22b, siz=8"`
	UUIDTreeGeneration uint64      `bin:"off=233, siz=8"` // uuid_tree_generation

	// FeatureIncompatMetadataUUID
	MetadataUUID UUID `bin:"off=23b, siz=10"`

	// FeatureIncompatExtentTreeV2
	NumGlobalRoots uint64 `bin:"off=24b, siz=8"`

	// FeatureIncompatExtentTreeV2
	BlockGroupRoot           LogicalAddr `bin:"off=253, siz=8"`
	BlockGroupRootGeneration Generation  `bin:"off=25b, siz=8"`
	BlockGroupRootLevel      uint8       `bin:"off=263, siz=1"`

	Reserved [199]byte `bin:"off=264, siz=c7"` // future expansion

	SysChunkArray [0x800]byte   `bin:"off=32b, siz=800"` // sys_chunk_array:(n bytes valid) Contains (KEY . CHUNK_ITEM) pairs for all SYSTEM chunks. This is needed to bootstrap the mapping from logical addresses to physical.
	SuperRoots    [4]RootBackup `bin:"off=b2b, siz=2a0"`

	// Padded to 4096 bytes
	Padding       [565]byte `bin:"off=dcb, siz=235"`
	binstruct.End `bin:"off=1000"`
}

func (sb Superblock) CalculateChecksum() (CSum, error) {
	data, err := binstruct.Marshal(sb)
	if err != nil {
		return CSum{}, err
	}
	return CRC32c(data[0x20:]), nil
}

func (sb Superblock) ValidateChecksum() error {
	stored := sb.Checksum
	calced, err := sb.CalculateChecksum()
	if err != nil {
		return err
	}
	if !calced.Equal(stored) {
		return fmt.Errorf("superblock checksum mismatch: stored=%s calculated=%s",
			stored, calced)
	}
	return nil
}

func (a Superblock) Equal(b Superblock) bool {
	a.Checksum = CSum{}
	a.Self = 0

	b.Checksum = CSum{}
	b.Self = 0

	return reflect.DeepEqual(a, b)
}

func (sb Superblock) EffectiveMetadataUUID() UUID {
	if !sb.IncompatFlags.Has(FeatureIncompatMetadataUUID) {
		return sb.FSUUID
	}
	return sb.MetadataUUID
}

type SysChunk struct {
	Key           `bin:"off=0, siz=11"`
	Chunk         `bin:"off=11, siz=30"`
	binstruct.End `bin:"off=41"`
}

func (sb Superblock) ParseSysChunkArray() ([]SysChunk, error) {
	dat := sb.SysChunkArray[:sb.SysChunkArraySize]
	var ret []SysChunk
	for len(dat) > 0 {
		var pair SysChunk
		if err := binstruct.Unmarshal(dat, &pair); err != nil {
			return nil, err
		}
		dat = dat[0x41:]

		for i := 0; i < int(pair.Chunk.NumStripes); i++ {
			var stripe Stripe
			if err := binstruct.Unmarshal(dat, &stripe); err != nil {
				return nil, err
			}
			pair.Chunk.Stripes = append(pair.Chunk.Stripes, stripe)
			dat = dat[0x20:]
		}

		ret = append(ret, pair)
	}
	return ret, nil
}

type RootBackup struct {
	TreeRoot    ObjID      `bin:"off=0, siz=8"`
	TreeRootGen Generation `bin:"off=8, siz=8"`

	ChunkRoot    ObjID      `bin:"off=10, siz=8"`
	ChunkRootGen Generation `bin:"off=18, siz=8"`

	ExtentRoot    ObjID      `bin:"off=20, siz=8"`
	ExtentRootGen Generation `bin:"off=28, siz=8"`

	FSRoot    ObjID      `bin:"off=30, siz=8"`
	FSRootGen Generation `bin:"off=38, siz=8"`

	DevRoot    ObjID      `bin:"off=40, siz=8"`
	DevRootGen Generation `bin:"off=48, siz=8"`

	ChecksumRoot    ObjID      `bin:"off=50, siz=8"`
	ChecksumRootGen Generation `bin:"off=58, siz=8"`

	TotalBytes uint64 `bin:"off=60, siz=8"`
	BytesUsed  uint64 `bin:"off=68, siz=8"`
	NumDevices uint64 `bin:"off=70, siz=8"`

	Unused [8 * 4]byte `bin:"off=78, siz=20"`

	TreeRootLevel     uint8 `bin:"off=98, siz=1"`
	ChunkRootLevel    uint8 `bin:"off=99, siz=1"`
	ExtentRootLevel   uint8 `bin:"off=9a, siz=1"`
	FSRootLevel       uint8 `bin:"off=9b, siz=1"`
	DevRootLevel      uint8 `bin:"off=9c, siz=1"`
	ChecksumRootLevel uint8 `bin:"off=9d, siz=1"`

	Padding       [10]byte `bin:"off=9e, siz=a"`
	binstruct.End `bin:"off=a8"`
}

type Node interface {
	GetNodeHeader() Ref[LogicalAddr, NodeHeader]
}

type NodeHeader struct {
	Checksum      CSum        `bin:"off=0,  siz=20"` // Checksum of everything after this field (from 20 to the end of the node)
	MetadataUUID  UUID        `bin:"off=20, siz=10"` // FS UUID
	Addr          LogicalAddr `bin:"off=30, siz=8"`  // Logical address of this node
	Flags         NodeFlags   `bin:"off=38, siz=7"`
	BackrefRev    uint8       `bin:"off=3f, siz=1"`
	ChunkTreeUUID UUID        `bin:"off=40, siz=10"` // Chunk tree UUID
	Generation    Generation  `bin:"off=50, siz=8"`  // Generation
	Owner         ObjID       `bin:"off=58, siz=8"`  // The ID of the tree that contains this node
	NumItems      uint32      `bin:"off=60, siz=4"`  // Number of items
	Level         uint8       `bin:"off=64, siz=1"`  // Level (0 for leaf nodes)
	binstruct.End `bin:"off=65"`

	Size     uint32 `bin:"-"` // superblock.NodeSize
	MaxItems uint32 `bin:"-"` // Maximum possible value of NumItems
}

type InternalNode struct {
	Header Ref[LogicalAddr, NodeHeader]
	Body   []Ref[LogicalAddr, KeyPointer]
}

func (in *InternalNode) GetNodeHeader() Ref[LogicalAddr, NodeHeader] {
	return in.Header
}

type KeyPointer struct {
	Key           Key         `bin:"off=0, siz=11"`
	BlockPtr      LogicalAddr `bin:"off=11, siz=8"`
	Generation    Generation  `bin:"off=19, siz=8"`
	binstruct.End `bin:"off=21"`
}

type LeafNode struct {
	Header Ref[LogicalAddr, NodeHeader]
	Body   []Ref[LogicalAddr, Item]
}

func (ln *LeafNode) GetNodeHeader() Ref[LogicalAddr, NodeHeader] {
	return ln.Header
}

func (ln *LeafNode) FreeSpace() uint32 {
	freeSpace := ln.Header.Data.Size
	freeSpace -= 0x65
	for _, item := range ln.Body {
		freeSpace -= 0x19
		freeSpace -= item.Data.DataSize
	}
	return freeSpace
}

type Item struct {
	Key           Key    `bin:"off=0, siz=11"`
	DataOffset    uint32 `bin:"off=11, siz=4"` // relative to the end of the header (0x65)
	DataSize      uint32 `bin:"off=15, siz=4"`
	binstruct.End `bin:"off=19"`
	Data          Ref[LogicalAddr, []byte] `bin:"-"`
}

type DevItem struct {
	DeviceID ObjID `bin:"off=0,    siz=8"` // device ID

	NumBytes     uint64 `bin:"off=8,    siz=8"` // number of bytes
	NumBytesUsed uint64 `bin:"off=10,   siz=8"` // number of bytes used

	IOOptimalAlign uint32 `bin:"off=18,   siz=4"` // optimal I/O align
	IOOptimalWidth uint32 `bin:"off=1c,   siz=4"` // optimal I/O width
	IOMinSize      uint32 `bin:"off=20,   siz=4"` // minimal I/O size (sector size)

	Type        uint64     `bin:"off=24,   siz=8"` // type
	Generation  Generation `bin:"off=2c,   siz=8"` // generation
	StartOffset uint64     `bin:"off=34,   siz=8"` // start offset
	DevGroup    uint32     `bin:"off=3c,   siz=4"` // dev group
	SeekSpeed   uint8      `bin:"off=40,   siz=1"` // seek speed
	Bandwidth   uint8      `bin:"off=41,   siz=1"` // bandwidth

	DevUUID UUID `bin:"off=42,   siz=10"` // device UUID
	FSUUID  UUID `bin:"off=52,   siz=10"` // FS UUID

	binstruct.End `bin:"off=62"`
}

type Chunk struct {
	// Maps logical address to physical.
	Size           uint64 `bin:"off=0,  siz=8"` // size of chunk (bytes)
	Owner          ObjID  `bin:"off=8,  siz=8"` // root referencing this chunk (2)
	StripeLen      uint64 `bin:"off=10, siz=8"` // stripe length
	Type           uint64 `bin:"off=18, siz=8"` // type (same as flags for block group?)
	IOOptimalAlign uint32 `bin:"off=20, siz=4"` // optimal io alignment
	IOOptimalWidth uint32 `bin:"off=24, siz=4"` // optimal io width
	IoMinSize      uint32 `bin:"off=28, siz=4"` // minimal io size (sector size)
	NumStripes     uint16 `bin:"off=2c, siz=2"` // number of stripes
	SubStripes     uint16 `bin:"off=2e, siz=2"` // sub stripes
	binstruct.End  `bin:"off=30"`
	Stripes        []Stripe `bin:"-"`
}

type Stripe struct {
	// Stripes follow (for each number of stripes):
	DeviceID      ObjID  `bin:"off=0,  siz=8"`  // device ID
	Offset        uint64 `bin:"off=8,  siz=8"`  // offset
	DeviceUUID    UUID   `bin:"off=10, siz=10"` // device UUID
	binstruct.End `bin:"off=20"`
}
