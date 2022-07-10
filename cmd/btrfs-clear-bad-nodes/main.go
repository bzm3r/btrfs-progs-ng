package main

import (
	"errors"
	"fmt"
	"os"

	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsvol"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfsmisc"
	"git.lukeshu.com/btrfs-progs-ng/lib/util"
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

	fs, err := btrfsmisc.Open(os.O_RDWR, imgfilenames...)
	if err != nil {
		return err
	}
	defer func() {
		maybeSetErr(fs.Close())
	}()

	var uuidsInited bool
	var metadataUUID, chunkTreeUUID btrfs.UUID

	var treeName string
	var treeID btrfs.ObjID
	btrfsmisc.WalkAllTrees(fs, btrfsmisc.WalkAllTreesHandler{
		PreTree: func(name string, id btrfs.ObjID) {
			treeName = name
			treeID = id
		},
		Err: func(err error) {
			fmt.Printf("error: %v\n", err)
		},
		UnsafeNodes: true,
		TreeWalkHandler: btrfs.TreeWalkHandler{
			Node: func(path btrfs.TreePath, node *util.Ref[btrfsvol.LogicalAddr, btrfs.Node], err error) error {
				if err == nil {
					if !uuidsInited {
						metadataUUID = node.Data.Head.MetadataUUID
						chunkTreeUUID = node.Data.Head.ChunkTreeUUID
						uuidsInited = true
					}
					return nil
				}
				if !errors.Is(err, btrfs.ErrNotANode) {
					err = btrfsmisc.WalkErr{
						TreeName: treeName,
						Path:     path,
						Err:      err,
					}
					fmt.Printf("error: %v\n", err)
					return nil
				}
				origErr := err
				if !uuidsInited {
					// TODO(lukeshu): Is there a better way to get the chunk
					// tree UUID?
					return fmt.Errorf("cannot repair node@%v: not (yet?) sure what the chunk tree UUID is", node.Addr)
				}
				node.Data = btrfs.Node{
					Size:         node.Data.Size,
					ChecksumType: node.Data.ChecksumType,
					Head: btrfs.NodeHeader{
						//Checksum:   filled below,
						MetadataUUID:  metadataUUID,
						Addr:          node.Addr,
						Flags:         btrfs.NodeWritten,
						BackrefRev:    btrfs.MixedBackrefRev,
						ChunkTreeUUID: chunkTreeUUID,
						Generation:    0,
						Owner:         treeID,
						NumItems:      0,
						Level:         path[len(path)-1].NodeLevel,
					},
				}
				node.Data.Head.Checksum, err = node.Data.CalculateChecksum()
				if err != nil {
					return btrfsmisc.WalkErr{
						TreeName: treeName,
						Path:     path,
						Err:      err,
					}
				}
				if err := node.Write(); err != nil {
					return err
				}

				fmt.Printf("fixed node@%v (err was %v)\n", node.Addr, origErr)
				return nil
			},
		},
	})

	return nil
}
