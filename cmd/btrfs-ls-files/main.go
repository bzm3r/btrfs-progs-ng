package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/datawire/dlib/derror"

	"lukeshu.com/btrfs-tools/pkg/btrfs"
	"lukeshu.com/btrfs-tools/pkg/btrfs/btrfsitem"
	"lukeshu.com/btrfs-tools/pkg/btrfs/btrfsvol"
	"lukeshu.com/btrfs-tools/pkg/btrfsmisc"
	"lukeshu.com/btrfs-tools/pkg/util"
)

func main() {
	if err := Main(os.Args[1:]...); err != nil {
		fmt.Fprintf(os.Stderr, "%v: error: %v\n", os.Args[0], err)
		os.Exit(1)
	}
}

func Main(imgfilenames ...string) (err error) {
	maybeSetErr := func(_err error) {
		if _err != nil && err == nil {
			err = _err
		}
	}

	fs, err := btrfsmisc.Open(os.O_RDONLY, imgfilenames...)
	if err != nil {
		return err
	}
	defer func() {
		maybeSetErr(fs.Close())
	}()

	sb, err := fs.Superblock()
	if err != nil {
		return err
	}

	fsTreeRoot, err := fs.TreeLookup(sb.Data.RootTree, btrfs.Key{
		ObjectID: btrfs.FS_TREE_OBJECTID,
		ItemType: btrfsitem.ROOT_ITEM_KEY,
		Offset:   0,
	})
	if err != nil {
		return fmt.Errorf("look up FS_TREE: %w", err)
	}
	fsTreeRootBody := fsTreeRoot.Body.(btrfsitem.Root)
	fsTree := fsTreeRootBody.ByteNr

	printDir(fs, fsTree, "", "", "/", fsTreeRootBody.RootDirID)
	return nil
}

const (
	tS = "    "
	tl = "│   "
	tT = "├── "
	tL = "└── "
)

func printDir(fs *btrfs.FS, fsTree btrfsvol.LogicalAddr, prefix0, prefix1, dirName string, dirInode btrfs.ObjID) {
	var errs derror.MultiError
	items, err := fs.TreeSearchAll(fsTree, func(key btrfs.Key) int {
		return util.CmpUint(dirInode, key.ObjectID)
	})
	if err != nil {
		errs = append(errs, fmt.Errorf("read dir: %w", err))
	}
	var dirInodeDat btrfsitem.Inode
	var dirInodeDatOK bool
	membersByIndex := make(map[uint64]btrfsitem.DirEntry)
	membersByNameHash := make(map[uint64]btrfsitem.DirEntry)
	for _, item := range items {
		switch item.Head.Key.ItemType {
		case btrfsitem.INODE_ITEM_KEY:
			if dirInodeDatOK {
				errs = append(errs, fmt.Errorf("read dir: multiple inodes"))
				continue
			}
			dirInodeDat = item.Body.(btrfsitem.Inode)
			dirInodeDatOK = true
		case btrfsitem.INODE_REF_KEY:
			// TODO
		case btrfsitem.DIR_ITEM_KEY:
			body := item.Body.(btrfsitem.DirEntries)
			if len(body) != 1 {
				errs = append(errs, fmt.Errorf("read dir: multiple direntries in single dir_item?"))
				continue
			}
			for _, entry := range body {
				namehash := btrfsitem.NameHash(entry.Name)
				if namehash != item.Head.Key.Offset {
					errs = append(errs, fmt.Errorf("read dir: direntry crc32c mismatch: key=%#x crc32c(%q)=%#x",
						item.Head.Key.Offset, entry.Name, namehash))
					continue
				}
				if other, exists := membersByNameHash[namehash]; exists {
					errs = append(errs, fmt.Errorf("read dir: multiple instances of direntry crc32c(%q|%q)=%#x",
						other.Name, entry.Name, namehash))
					continue
				}
				membersByNameHash[btrfsitem.NameHash(entry.Name)] = entry
			}
		case btrfsitem.DIR_INDEX_KEY:
			for i, entry := range item.Body.(btrfsitem.DirEntries) {
				index := item.Head.Key.Offset + uint64(i)
				if _, exists := membersByIndex[index]; exists {
					errs = append(errs, fmt.Errorf("read dir: multiple instances of direntry index %v", index))
					continue
				}
				membersByIndex[index] = entry
			}
		case btrfsitem.XATTR_ITEM_KEY:
		default:
			panic(fmt.Errorf("TODO: handle item type %v", item.Head.Key.ItemType))
		}
	}
	fmt.Printf("%s%q\t[ino=%d\t",
		prefix0, dirName, dirInode)
	if dirInodeDatOK {
		fmt.Printf("uid=%d\tgid=%d\tsize=%d]\n",
			dirInodeDat.UID, dirInodeDat.GID, dirInodeDat.Size)
	} else {
		fmt.Printf("error=read dir: no inode data\n")
	}
	for i, index := range util.SortedMapKeys(membersByIndex) {
		entry := membersByIndex[index]
		namehash := btrfsitem.NameHash(entry.Name)
		if other, ok := membersByNameHash[namehash]; ok {
			if !reflect.DeepEqual(entry, other) {
				errs = append(errs, fmt.Errorf("read dir: index=%d disagrees with crc32c(%q)=%#x",
					index, entry.Name, namehash))
			}
			delete(membersByNameHash, namehash)
		} else {
			errs = append(errs, fmt.Errorf("read dir: no DIR_ITEM crc32c(%q)=%#x for DIR_INDEX index=%d",
				entry.Name, namehash, index))
		}
		prefix := tT
		if (i == len(membersByIndex)-1) && (len(membersByNameHash) == 0) && (len(errs) == 0) {
			prefix = tL
		}
		printItem(fs, fsTree, prefix1+prefix, prefix1+tS, string(entry.Name), entry.Location)
	}
	for _, namehash := range util.SortedMapKeys(membersByNameHash) {
		entry := membersByNameHash[namehash]
		errs = append(errs, fmt.Errorf("read dir: no DIR_INDEX for DIR_ITEM crc32c(%q)=%#x",
			entry.Name, namehash))
		printItem(fs, fsTree, prefix1+tT, prefix1+tS, string(entry.Name), entry.Location)
	}
	for i, err := range errs {
		prefix := tT
		if i == len(errs)-1 {
			prefix = tL
		}
		fmt.Printf("%s%s%s\n", prefix1, prefix, strings.ReplaceAll(err.Error(), "\n", prefix1+tS+"\n"))
	}
}

func printItem(fs *btrfs.FS, fsTree btrfsvol.LogicalAddr, prefix0, prefix1, name string, location btrfs.Key) {
	fmt.Printf("%s%q\t[location=%v]\n", prefix0, name, location)
}
