package btrfsitem

import (
	"fmt"
	"reflect"

	"lukeshu.com/btrfs-tools/pkg/binstruct"
	"lukeshu.com/btrfs-tools/pkg/btrfs/internal"
)

type Type = internal.ItemType

type Item interface {
	isItem()
}

type Error struct {
	Dat []byte
	Err error
}

func (Error) isItem() {}

func (o Error) MarshalBinary() ([]byte, error) {
	return o.Dat, nil
}

func (o *Error) UnmarshalBinary(dat []byte) (int, error) {
	o.Dat = dat
	return len(dat), nil
}

// Rather than returning a separate error  value, return an Error item.
func UnmarshalItem(key internal.Key, dat []byte) Item {
	var gotyp reflect.Type
	if key.ItemType == UNTYPED_KEY {
		var ok bool
		gotyp, ok = untypedObjID2gotype[key.ObjectID]
		if !ok {
			return Error{
				Dat: dat,
				Err: fmt.Errorf("btrfsitem.UnmarshalItem({ItemType:%v, ObjectID:%v}, dat): unknown object ID for untyped item",
					key.ItemType, key.ObjectID),
			}
		}
	} else {
		var ok bool
		gotyp, ok = keytype2gotype[key.ItemType]
		if !ok {
			return Error{
				Dat: dat,
				Err: fmt.Errorf("btrfsitem.UnmarshalItem({ItemType:%v}, dat): unknown item type", key.ItemType),
			}
		}
	}
	retPtr := reflect.New(gotyp)
	n, err := binstruct.Unmarshal(dat, retPtr.Interface())
	if err != nil {
		return Error{
			Dat: dat,
			Err: fmt.Errorf("btrfsitem.UnmarshalItem({ItemType:%v}, dat): %w", key.ItemType, err),
		}

	}
	if n < len(dat) {
		return Error{
			Dat: dat,
			Err: fmt.Errorf("btrfsitem.UnmarshalItem({ItemType:%v}, dat): left over data: got %d bytes but only consumed %d",
				key.ItemType, len(dat), n),
		}
	}
	return retPtr.Elem().Interface().(Item)
}
