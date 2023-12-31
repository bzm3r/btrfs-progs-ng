// Copyright (C) 2022-2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package btrfssum

import (
	"io"
	"strings"

	"git.lukeshu.com/go/lowmemjson"

	"git.lukeshu.com/btrfs-progs-ng/lib/jsonutil"
	"git.lukeshu.com/btrfs-progs-ng/lib/textui"
)

type ShortSum string

var (
	_ lowmemjson.Encodable = ShortSum("")
	_ lowmemjson.Decodable = (*ShortSum)(nil)
)

func (sum ShortSum) ToFullSum() CSum {
	var ret CSum
	copy(ret[:], sum)
	return ret
}

func (sum ShortSum) EncodeJSON(w io.Writer) error {
	return jsonutil.EncodeSplitHexString(w, sum, textui.Tunable(80))
}

func (sum *ShortSum) DecodeJSON(r io.RuneScanner) error {
	var out strings.Builder
	if err := jsonutil.DecodeSplitHexString(r, &out); err != nil {
		return err
	}
	*sum = ShortSum(out.String())
	return nil
}
