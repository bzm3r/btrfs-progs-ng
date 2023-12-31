// Copyright (C) 2022-2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"os"

	"github.com/datawire/dlib/dlog"
	"github.com/datawire/ocibuild/pkg/cliutil"
	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"

	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfsprim"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs/btrfstree"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfsutil"
	"git.lukeshu.com/btrfs-progs-ng/lib/textui"
)

func init() {
	inspectors.AddCommand(&cobra.Command{
		Use:   "spew-items",
		Short: "Spew all items as parsed",
		Args:  cliutil.WrapPositionalArgs(cobra.NoArgs),
		RunE: runWithReadableFS(func(fs btrfs.ReadableFS, cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			spew := spew.NewDefaultConfig()
			spew.DisablePointerAddresses = true

			btrfsutil.WalkAllTrees(ctx, fs, btrfsutil.WalkAllTreesHandler{
				BadTree: func(name string, id btrfsprim.ObjID, err error) {
					dlog.Errorf(ctx, "%v: %v", name, err)
				},
				Tree: btrfstree.TreeWalkHandler{
					Item: func(path btrfstree.Path, item btrfstree.Item) {
						textui.Fprintf(os.Stdout, "%s = ", path)
						spew.Dump(item)
						_, _ = os.Stdout.WriteString("\n")
					},
					BadItem: func(path btrfstree.Path, item btrfstree.Item) {
						textui.Fprintf(os.Stdout, "%s = ", path)
						spew.Dump(item)
						_, _ = os.Stdout.WriteString("\n")
					},
				},
			})
			return nil
		}),
	})
}
