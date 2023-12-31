// Copyright (C) 2022-2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package btrfsitem

import (
	"git.lukeshu.com/btrfs-progs-ng/lib/binstruct"
)

const (
	DEV_STAT_WRITE_ERRS = iota
	DEV_STAT_READ_ERRS
	DEV_STAT_FLUSH_ERRS
	DEV_STAT_CORRUPTION_ERRS
	DEV_STAT_GENERATION_ERRS
	DEV_STAT_VALUES_MAX
)

type DevStats struct { // trivial PERSISTENT_ITEM=249
	Values        [DEV_STAT_VALUES_MAX]int64 `bin:"off=0, siz=40"`
	binstruct.End `bin:"off=40"`
}
