package brotli

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Utilities for building Huffman decoding tables. */
type SymbolList struct {
	storage []uint16
	offset  int
}

func SymbolListGet(sl SymbolList, i int) uint16 {
	return sl.storage[i+sl.offset]
}

func SymbolListPut(sl SymbolList, i int, val uint16) {
	sl.storage[i+sl.offset] = val
}
