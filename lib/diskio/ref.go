// Copyright (C) 2022  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package diskio

import (
	"fmt"

	"git.lukeshu.com/btrfs-progs-ng/lib/binstruct"
)

type Ref[A ~int64, T any] struct {
	File File[A]
	Addr A
	Data T
}

func (r *Ref[A, T]) Read() error {
	size := binstruct.StaticSize(r.Data)
	buf := make([]byte, size)
	if _, err := r.File.ReadAt(buf, r.Addr); err != nil {
		return err
	}
	n, err := binstruct.Unmarshal(buf, &r.Data)
	if err != nil {
		return err
	}
	if n != size {
		return fmt.Errorf("util.Ref[%T].Read: left over data: read %v bytes but only consumed %v",
			r.Data, size, n)
	}
	return nil
}

func (r *Ref[A, T]) Write() error {
	buf, err := binstruct.Marshal(r.Data)
	if err != nil {
		return err
	}
	if _, err = r.File.WriteAt(buf, r.Addr); err != nil {
		return err
	}
	return nil
}
