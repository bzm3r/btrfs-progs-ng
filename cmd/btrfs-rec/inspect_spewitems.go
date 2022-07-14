// Copyright (C) 2022  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"fmt"
	"os"

	"github.com/datawire/dlib/dlog"
	"github.com/datawire/ocibuild/pkg/cliutil"
	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"

	"git.lukeshu.com/btrfs-progs-ng/lib/btrfs"
	"git.lukeshu.com/btrfs-progs-ng/lib/btrfsprogs/btrfsutil"
)

func init() {
	inspectors = append(inspectors, subcommand{
		Command: cobra.Command{
			Use:   "spew-items",
			Short: "Spew all items as parsed",
			Args:  cliutil.WrapPositionalArgs(cobra.NoArgs),
		},
		RunE: func(fs *btrfs.FS, cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			spew := spew.NewDefaultConfig()
			spew.DisablePointerAddresses = true

			btrfsutil.WalkAllTrees(ctx, fs, btrfsutil.WalkAllTreesHandler{
				Err: func(err *btrfsutil.WalkError) {
					dlog.Error(ctx, err)
				},
				TreeWalkHandler: btrfs.TreeWalkHandler{
					Item: func(path btrfs.TreePath, item btrfs.Item) error {
						fmt.Printf("%s = ", path)
						spew.Dump(item)
						os.Stdout.WriteString("\n")
						return nil
					},
					BadItem: func(path btrfs.TreePath, item btrfs.Item) error {
						fmt.Printf("%s = ", path)
						spew.Dump(item)
						os.Stdout.WriteString("\n")
						return nil
					},
				},
			})
			return nil
		},
	})
}