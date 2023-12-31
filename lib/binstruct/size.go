// Copyright (C) 2022-2023  Luke Shumaker <lukeshu@lukeshu.com>
//
// SPDX-License-Identifier: GPL-2.0-or-later

package binstruct

import (
	"errors"
	"fmt"
	"reflect"
)

type StaticSizer interface {
	BinaryStaticSize() int
}

func StaticSize(obj any) int {
	sz, err := staticSize(reflect.TypeOf(obj))
	if err != nil {
		panic(err)
	}
	return sz
}

var (
	staticSizerType = reflect.TypeOf((*StaticSizer)(nil)).Elem()
	marshalerType   = reflect.TypeOf((*Marshaler)(nil)).Elem()
	unmarshalerType = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
)

const (
	sizeof8  = 1
	sizeof16 = 2
	sizeof32 = 4
	sizeof64 = 8
)

func staticSize(typ reflect.Type) (int, error) {
	if typ.Implements(staticSizerType) {
		//nolint:forcetypeassert // Already did a type check via reflection.
		return reflect.New(typ).Elem().Interface().(StaticSizer).BinaryStaticSize(), nil
	}
	if typ.Implements(marshalerType) || typ.Implements(unmarshalerType) {
		// If you implement binstruct.Marshaler or binstruct.Unmarshaler,
		// then you must implement if you wish to be statically sized.
		return 0, &InvalidTypeError{
			Type: typ,
			Err:  errors.New("does not implement binstruct.StaticSizer but does implement binstruct.Marshaler or binstruct.Unmarshaler"),
		}
	}
	switch typ.Kind() {
	case reflect.Uint8, reflect.Int8:
		return sizeof8, nil
	case reflect.Uint16, reflect.Int16:
		return sizeof16, nil
	case reflect.Uint32, reflect.Int32:
		return sizeof32, nil
	case reflect.Uint64, reflect.Int64:
		return sizeof64, nil
	case reflect.Ptr:
		return staticSize(typ.Elem())
	case reflect.Array:
		elemSize, err := staticSize(typ.Elem())
		if err != nil {
			return 0, err
		}
		return elemSize * typ.Len(), nil
	case reflect.Struct:
		return getStructHandler(typ).Size, nil
	default:
		return 0, &InvalidTypeError{
			Type: typ,
			Err: fmt.Errorf("does not implement binfmt.StaticSizer and kind=%v is not a supported statically-sized kind",
				typ.Kind()),
		}
	}
}
