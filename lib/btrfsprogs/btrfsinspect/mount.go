// Copyright (C) 2022  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package btrfsinspect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/datawire/dlib/dcontext"
	"github.com/datawire/dlib/dgroup"
	"github.com/datawire/dlib/dlog"
	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"

	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsitem"
	"git.lukeshu.com/btrfs-progs-ng/lib/linux"
	"git.lukeshu.com/btrfs-progs-ng/lib/util"
)

func MountRO(ctx context.Context, fs *btrfs.FS, mountpoint string) error {
	pvs := fs.LV.PhysicalVolumes()
	if len(pvs) < 1 {
		return errors.New("no devices")
	}

	deviceName := pvs[util.SortedMapKeys(pvs)[0]].Name()
	if abs, err := filepath.Abs(deviceName); err == nil {
		deviceName = abs
	}

	rootSubvol := &subvolume{
		Subvolume: btrfs.Subvolume{
			FS:     fs,
			TreeID: btrfs.FS_TREE_OBJECTID,
		},
		DeviceName: deviceName,
		Mountpoint: mountpoint,
	}
	return rootSubvol.Run(ctx)
}

func fuseMount(ctx context.Context, mountpoint string, server fuse.Server, cfg *fuse.MountConfig) error {
	grp := dgroup.NewGroup(ctx, dgroup.GroupConfig{
		// Allow mountHandle.Join() returning to cause the
		// "unmount" goroutine to quit.
		ShutdownOnNonError: true,
	})
	mounted := uint32(1)
	grp.Go("unmount", func(ctx context.Context) error {
		<-ctx.Done()
		var err error
		var gotNil bool
		// Keep retrying, because the FS might be busy.
		for atomic.LoadUint32(&mounted) != 0 {
			if _err := fuse.Unmount(mountpoint); _err == nil {
				gotNil = true
			} else if !gotNil {
				err = _err
			}
		}
		if gotNil {
			return nil
		}
		return err
	})
	grp.Go("mount", func(ctx context.Context) error {
		defer atomic.StoreUint32(&mounted, 0)

		cfg.OpContext = ctx
		cfg.ErrorLogger = dlog.StdLogger(ctx, dlog.LogLevelError)
		cfg.DebugLogger = dlog.StdLogger(ctx, dlog.LogLevelDebug)

		mountHandle, err := fuse.Mount(mountpoint, server, cfg)
		if err != nil {
			return err
		}
		dlog.Infof(ctx, "mounted %q", mountpoint)
		return mountHandle.Join(dcontext.HardContext(ctx))
	})
	return grp.Wait()
}

type dirState struct {
	Dir *btrfs.Dir
}

type fileState struct {
	File *btrfs.File
}

type subvolume struct {
	btrfs.Subvolume
	DeviceName string
	Mountpoint string

	fuseutil.NotImplementedFileSystem
	lastHandle  uint64
	dirHandles  util.SyncMap[fuseops.HandleID, *dirState]
	fileHandles util.SyncMap[fuseops.HandleID, *fileState]

	subvolMu sync.Mutex
	subvols  map[string]struct{}
	grp      *dgroup.Group
}

func (sv *subvolume) Run(ctx context.Context) error {
	sv.grp = dgroup.NewGroup(ctx, dgroup.GroupConfig{})
	sv.grp.Go("self", func(ctx context.Context) error {
		cfg := &fuse.MountConfig{
			FSName:  sv.DeviceName,
			Subtype: "btrfs",

			ReadOnly: true,

			Options: map[string]string{
				"allow_other": "",
			},
		}
		return fuseMount(ctx, sv.Mountpoint, fuseutil.NewFileSystemServer(sv), cfg)
	})
	return sv.grp.Wait()
}

func (sv *subvolume) newHandle() fuseops.HandleID {
	return fuseops.HandleID(atomic.AddUint64(&sv.lastHandle, 1))
}

func inodeItemToFUSE(itemBody btrfsitem.Inode) fuseops.InodeAttributes {
	return fuseops.InodeAttributes{
		Size:  uint64(itemBody.Size),
		Nlink: uint32(itemBody.NLink),
		Mode:  uint32(itemBody.Mode),
		//RDev: itemBody.Rdev, // jacobsa/fuse doesn't expose rdev
		Atime: itemBody.ATime.ToStd(),
		Mtime: itemBody.MTime.ToStd(),
		Ctime: itemBody.CTime.ToStd(),
		//Crtime: itemBody.OTime,
		Uid: uint32(itemBody.UID),
		Gid: uint32(itemBody.GID),
	}
}

func (sv *subvolume) LoadDir(inode btrfs.ObjID) (val *btrfs.Dir, err error) {
	val, err = sv.Subvolume.LoadDir(inode)
	if val != nil {
		haveSubvolumes := false
		for _, index := range util.SortedMapKeys(val.ChildrenByIndex) {
			entry := val.ChildrenByIndex[index]
			if entry.Location.ItemType == btrfsitem.ROOT_ITEM_KEY {
				haveSubvolumes = true
				break
			}
		}
		if haveSubvolumes {
			abspath, _err := val.AbsPath()
			if _err != nil {
				return
			}
			sv.subvolMu.Lock()
			for _, index := range util.SortedMapKeys(val.ChildrenByIndex) {
				entry := val.ChildrenByIndex[index]
				if entry.Location.ItemType != btrfsitem.ROOT_ITEM_KEY {
					continue
				}
				if sv.subvols == nil {
					sv.subvols = make(map[string]struct{})
				}
				subMountpoint := filepath.Join(abspath, string(entry.Name))
				if _, alreadyMounted := sv.subvols[subMountpoint]; !alreadyMounted {
					sv.subvols[subMountpoint] = struct{}{}
					workerName := fmt.Sprintf("%d-%s", val.Inode, filepath.Base(subMountpoint))
					sv.grp.Go(workerName, func(ctx context.Context) error {
						subSv := &subvolume{
							Subvolume: btrfs.Subvolume{
								FS:     sv.FS,
								TreeID: entry.Location.ObjectID,
							},
							DeviceName: sv.DeviceName,
							Mountpoint: filepath.Join(sv.Mountpoint, subMountpoint[1:]),
						}
						return subSv.Run(ctx)
					})
				}
			}
			sv.subvolMu.Unlock()
		}
	}
	return
}

func (sv *subvolume) StatFS(_ context.Context, op *fuseops.StatFSOp) error {
	// See linux.git/fs/btrfs/super.c:btrfs_statfs()
	sb, err := sv.FS.Superblock()
	if err != nil {
		return err
	}

	op.IoSize = sb.SectorSize
	op.BlockSize = sb.SectorSize
	op.Blocks = sb.TotalBytes / uint64(sb.SectorSize) // TODO: adjust for RAID type
	//op.BlocksFree = TODO

	// btrfs doesn't have a fixed number of inodes
	op.Inodes = 0
	op.InodesFree = 0

	// jacobsa/fuse doesn't expose namelen, instead hard-coding it
	// to 255.  Which is fine by us, because that's what it is for
	// btrfs.

	return nil
}

func (sv *subvolume) LookUpInode(_ context.Context, op *fuseops.LookUpInodeOp) error {
	if op.Parent == fuseops.RootInodeID {
		parent, err := sv.GetRootInode()
		if err != nil {
			return err
		}
		op.Parent = fuseops.InodeID(parent)
	}

	dir, err := sv.LoadDir(btrfs.ObjID(op.Parent))
	if err != nil {
		return err
	}
	entry, ok := dir.ChildrenByName[op.Name]
	if !ok {
		return syscall.ENOENT
	}
	if entry.Location.ItemType != btrfsitem.INODE_ITEM_KEY {
		// Subvolume
		//
		// Because each subvolume has its own pool of inodes
		// (as in 2 different subvolumes can have files with
		// te same inode number), so to represent that to FUSE
		// we need to have this be a full separate mountpoint.
		//
		// I'd want to return EIO or EINTR or something here,
		// but both the FUSE userspace tools and the kernel
		// itself stat the mountpoint before mounting it, so
		// we've got to return something bogus here to let
		// that mount happen.
		op.Entry = fuseops.ChildInodeEntry{
			Child: 2, // an inode number that a real file will never have
			Attributes: fuseops.InodeAttributes{
				Nlink: 1,
				Mode:  uint32(linux.ModeFmtDir | 0700),
			},
		}
		return nil
	}
	bareInode, err := sv.LoadBareInode(entry.Location.ObjectID)
	if err != nil {
		return err
	}
	op.Entry = fuseops.ChildInodeEntry{
		Child:      fuseops.InodeID(entry.Location.ObjectID),
		Generation: fuseops.GenerationNumber(bareInode.InodeItem.Sequence),
		Attributes: inodeItemToFUSE(*bareInode.InodeItem),
	}
	return nil
}

func (sv *subvolume) GetInodeAttributes(_ context.Context, op *fuseops.GetInodeAttributesOp) error {
	if op.Inode == fuseops.RootInodeID {
		inode, err := sv.GetRootInode()
		if err != nil {
			return err
		}
		op.Inode = fuseops.InodeID(inode)
	}

	bareInode, err := sv.LoadBareInode(btrfs.ObjID(op.Inode))
	if err != nil {
		return err
	}

	op.Attributes = inodeItemToFUSE(*bareInode.InodeItem)
	return nil
}

func (sv *subvolume) OpenDir(_ context.Context, op *fuseops.OpenDirOp) error {
	if op.Inode == fuseops.RootInodeID {
		inode, err := sv.GetRootInode()
		if err != nil {
			return err
		}
		op.Inode = fuseops.InodeID(inode)
	}

	dir, err := sv.LoadDir(btrfs.ObjID(op.Inode))
	if err != nil {
		return err
	}
	handle := sv.newHandle()
	sv.dirHandles.Store(handle, &dirState{
		Dir: dir,
	})
	op.Handle = handle
	return nil
}
func (sv *subvolume) ReadDir(_ context.Context, op *fuseops.ReadDirOp) error {
	state, ok := sv.dirHandles.Load(op.Handle)
	if !ok {
		return syscall.EBADF
	}
	origOffset := op.Offset
	for _, index := range util.SortedMapKeys(state.Dir.ChildrenByIndex) {
		if index < uint64(origOffset) {
			continue
		}
		entry := state.Dir.ChildrenByIndex[index]
		n := fuseutil.WriteDirent(op.Dst[op.BytesRead:], fuseutil.Dirent{
			Offset: fuseops.DirOffset(index + 1),
			Inode:  fuseops.InodeID(entry.Location.ObjectID),
			Name:   string(entry.Name),
			Type: map[btrfsitem.FileType]fuseutil.DirentType{
				btrfsitem.FT_UNKNOWN:  fuseutil.DT_Unknown,
				btrfsitem.FT_REG_FILE: fuseutil.DT_File,
				btrfsitem.FT_DIR:      fuseutil.DT_Directory,
				btrfsitem.FT_CHRDEV:   fuseutil.DT_Char,
				btrfsitem.FT_BLKDEV:   fuseutil.DT_Block,
				btrfsitem.FT_FIFO:     fuseutil.DT_FIFO,
				btrfsitem.FT_SOCK:     fuseutil.DT_Socket,
				btrfsitem.FT_SYMLINK:  fuseutil.DT_Link,
			}[entry.Type],
		})
		if n == 0 {
			break
		}
		op.BytesRead += n
	}
	return nil
}
func (sv *subvolume) ReleaseDirHandle(_ context.Context, op *fuseops.ReleaseDirHandleOp) error {
	_, ok := sv.dirHandles.LoadAndDelete(op.Handle)
	if !ok {
		return syscall.EBADF
	}
	return nil
}

func (sv *subvolume) OpenFile(_ context.Context, op *fuseops.OpenFileOp) error {
	file, err := sv.LoadFile(btrfs.ObjID(op.Inode))
	if err != nil {
		return err
	}
	handle := sv.newHandle()
	sv.fileHandles.Store(handle, &fileState{
		File: file,
	})
	op.Handle = handle
	op.KeepPageCache = true
	return nil
}
func (sv *subvolume) ReadFile(_ context.Context, op *fuseops.ReadFileOp) error {
	state, ok := sv.fileHandles.Load(op.Handle)
	if !ok {
		return syscall.EBADF
	}

	var dat []byte
	if op.Dst != nil {
		size := util.Min(int64(len(op.Dst)), op.Size)
		dat = op.Dst[:size]
	} else {
		dat = make([]byte, op.Size)
		op.Data = [][]byte{dat}
	}

	var err error
	op.BytesRead, err = state.File.ReadAt(dat, op.Offset)
	if errors.Is(err, io.EOF) {
		err = nil
	}

	return err
}
func (sv *subvolume) ReleaseFileHandle(_ context.Context, op *fuseops.ReleaseFileHandleOp) error {
	_, ok := sv.fileHandles.LoadAndDelete(op.Handle)
	if !ok {
		return syscall.EBADF
	}
	return nil
}

func (sv *subvolume) ReadSymlink(_ context.Context, op *fuseops.ReadSymlinkOp) error {
	return syscall.ENOSYS
}

func (sv *subvolume) GetXattr(_ context.Context, op *fuseops.GetXattrOp) error { return syscall.ENOSYS }
func (sv *subvolume) ListXattr(_ context.Context, op *fuseops.ListXattrOp) error {
	return syscall.ENOSYS
}

func (sv *subvolume) Destroy() {}
