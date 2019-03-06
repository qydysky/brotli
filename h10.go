package brotli

/* NOLINT(build/header_guard) */
/* Copyright 2016 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* A (forgetful) hash table where each hash bucket contains a binary tree of
   sequences whose first 4 bytes share the same hash code.
   Each sequence is 128 long and is identified by its starting
   position in the input data. The binary tree is sorted by the lexicographic
   order of the sequences, and it is also a max-heap with respect to the
   starting positions. */
func HashTypeLengthH10() uint {
	return 4
}

func StoreLookaheadH10() uint {
	return 128
}

func HashBytesH10(data []byte) uint32 {
	var h uint32 = BROTLI_UNALIGNED_LOAD32LE(data) * kHashMul32

	/* The higher bits contain more mixture from the multiplication,
	   so we take our results from there. */
	return h >> (32 - 17)
}

type H10 struct {
	window_mask_ uint
	buckets_     [1 << 17]uint32
	invalid_pos_ uint32
	forest       []uint32
}

func SelfH10(handle HasherHandle) *H10 {
	return handle.extra.(*H10)
}

func ForestH10(self *H10) []uint32 {
	return []uint32(self.forest)
}

func InitializeH10(handle HasherHandle, params *BrotliEncoderParams) {
	handle.extra = new(H10)
	var self *H10 = SelfH10(handle)
	self.window_mask_ = (1 << params.lgwin) - 1
	self.invalid_pos_ = uint32(0 - self.window_mask_)
	var num_nodes uint = uint(1) << params.lgwin
	self.forest = make([]uint32, 2*num_nodes)
}

func PrepareH10(handle HasherHandle, one_shot bool, input_size uint, data []byte) {
	var self *H10 = SelfH10(handle)
	var invalid_pos uint32 = self.invalid_pos_
	var i uint32
	for i = 0; i < 1<<17; i++ {
		self.buckets_[i] = invalid_pos
	}
}

func LeftChildIndexH10(self *H10, pos uint) uint {
	return 2 * (pos & self.window_mask_)
}

func RightChildIndexH10(self *H10, pos uint) uint {
	return 2*(pos&self.window_mask_) + 1
}

/* Stores the hash of the next 4 bytes and in a single tree-traversal, the
   hash bucket's binary tree is searched for matches and is re-rooted at the
   current position.

   If less than 128 data is available, the hash bucket of the
   current position is searched for matches, but the state of the hash table
   is not changed, since we can not know the final sorting order of the
   current (incomplete) sequence.

   This function must be called with increasing cur_ix positions. */
func StoreAndFindMatchesH10(self *H10, data []byte, cur_ix uint, ring_buffer_mask uint, max_length uint, max_backward uint, best_len *uint, matches []BackwardMatch) []BackwardMatch {
	var cur_ix_masked uint = cur_ix & ring_buffer_mask
	var max_comp_len uint = brotli_min_size_t(max_length, 128)
	var should_reroot_tree bool = (max_length >= 128)
	var key uint32 = HashBytesH10(data[cur_ix_masked:])
	var forest []uint32 = ForestH10(self)
	var prev_ix uint = uint(self.buckets_[key])
	var node_left uint = LeftChildIndexH10(self, cur_ix)
	var node_right uint = RightChildIndexH10(self, cur_ix)
	var best_len_left uint = 0
	var best_len_right uint = 0
	var depth_remaining uint
	/* The forest index of the rightmost node of the left subtree of the new
	   root, updated as we traverse and re-root the tree of the hash bucket. */

	/* The forest index of the leftmost node of the right subtree of the new
	   root, updated as we traverse and re-root the tree of the hash bucket. */

	/* The match length of the rightmost node of the left subtree of the new
	   root, updated as we traverse and re-root the tree of the hash bucket. */

	/* The match length of the leftmost node of the right subtree of the new
	   root, updated as we traverse and re-root the tree of the hash bucket. */
	if should_reroot_tree {
		self.buckets_[key] = uint32(cur_ix)
	}

	for depth_remaining = 64; ; depth_remaining-- {
		var backward uint = cur_ix - prev_ix
		var prev_ix_masked uint = prev_ix & ring_buffer_mask
		if backward == 0 || backward > max_backward || depth_remaining == 0 {
			if should_reroot_tree {
				forest[node_left] = self.invalid_pos_
				forest[node_right] = self.invalid_pos_
			}

			break
		}
		{
			var cur_len uint = brotli_min_size_t(best_len_left, best_len_right)
			var len uint
			assert(cur_len <= 128)
			len = cur_len + FindMatchLengthWithLimit(data[cur_ix_masked+cur_len:], data[prev_ix_masked+cur_len:], max_length-cur_len)
			if matches != nil && len > *best_len {
				*best_len = uint(len)
				InitBackwardMatch(&matches[0], backward, uint(len))
				matches = matches[1:]
			}

			if len >= max_comp_len {
				if should_reroot_tree {
					forest[node_left] = forest[LeftChildIndexH10(self, prev_ix)]
					forest[node_right] = forest[RightChildIndexH10(self, prev_ix)]
				}

				break
			}

			if data[cur_ix_masked+len] > data[prev_ix_masked+len] {
				best_len_left = uint(len)
				if should_reroot_tree {
					forest[node_left] = uint32(prev_ix)
				}

				node_left = RightChildIndexH10(self, prev_ix)
				prev_ix = uint(forest[node_left])
			} else {
				best_len_right = uint(len)
				if should_reroot_tree {
					forest[node_right] = uint32(prev_ix)
				}

				node_right = LeftChildIndexH10(self, prev_ix)
				prev_ix = uint(forest[node_right])
			}
		}
	}

	return matches
}

/* Finds all backward matches of &data[cur_ix & ring_buffer_mask] up to the
   length of max_length and stores the position cur_ix in the hash table.

   Sets *num_matches to the number of matches found, and stores the found
   matches in matches[0] to matches[*num_matches - 1]. The matches will be
   sorted by strictly increasing length and (non-strictly) increasing
   distance. */
func FindAllMatchesH10(handle HasherHandle, dictionary *BrotliEncoderDictionary, data []byte, ring_buffer_mask uint, cur_ix uint, max_length uint, max_backward uint, gap uint, params *BrotliEncoderParams, matches []BackwardMatch) uint {
	var orig_matches []BackwardMatch = matches
	var cur_ix_masked uint = cur_ix & ring_buffer_mask
	var best_len uint = 1
	var short_match_max_backward uint
	if params.quality != HQ_ZOPFLIFICATION_QUALITY {
		short_match_max_backward = 16
	} else {
		short_match_max_backward = 64
	}
	var stop uint = cur_ix - short_match_max_backward
	var dict_matches [BROTLI_MAX_STATIC_DICTIONARY_MATCH_LEN + 1]uint32
	var i uint
	if cur_ix < short_match_max_backward {
		stop = 0
	}
	for i = cur_ix - 1; i > stop && best_len <= 2; i-- {
		var prev_ix uint = i
		var backward uint = cur_ix - prev_ix
		if backward > max_backward {
			break
		}

		prev_ix &= ring_buffer_mask
		if data[cur_ix_masked] != data[prev_ix] || data[cur_ix_masked+1] != data[prev_ix+1] {
			continue
		}
		{
			var len uint = FindMatchLengthWithLimit(data[prev_ix:], data[cur_ix_masked:], max_length)
			if len > best_len {
				best_len = uint(len)
				InitBackwardMatch(&matches[0], backward, uint(len))
				matches = matches[1:]
			}
		}
	}

	if best_len < max_length {
		matches = StoreAndFindMatchesH10(SelfH10(handle), data, cur_ix, ring_buffer_mask, max_length, max_backward, &best_len, matches)
	}

	for i = 0; i <= BROTLI_MAX_STATIC_DICTIONARY_MATCH_LEN; i++ {
		dict_matches[i] = kInvalidMatch
	}
	{
		var minlen uint = brotli_max_size_t(4, best_len+1)
		if BrotliFindAllStaticDictionaryMatches(dictionary, data[cur_ix_masked:], minlen, max_length, dict_matches[0:]) {
			var maxlen uint = brotli_min_size_t(BROTLI_MAX_STATIC_DICTIONARY_MATCH_LEN, max_length)
			var l uint
			for l = minlen; l <= maxlen; l++ {
				var dict_id uint32 = dict_matches[l]
				if dict_id < kInvalidMatch {
					var distance uint = max_backward + gap + uint(dict_id>>5) + 1
					if distance <= params.dist.max_distance {
						InitDictionaryBackwardMatch(&matches[0], distance, l, uint(dict_id&31))
						matches = matches[1:]
					}
				}
			}
		}
	}

	return uint(-cap(matches) + cap(orig_matches))
}

/* Stores the hash of the next 4 bytes and re-roots the binary tree at the
   current sequence, without returning any matches.
   REQUIRES: ix + 128 <= end-of-current-block */
func StoreH10(handle HasherHandle, data []byte, mask uint, ix uint) {
	var self *H10 = SelfH10(handle)
	var max_backward uint = self.window_mask_ - BROTLI_WINDOW_GAP + 1
	/* Maximum distance is window size - 16, see section 9.1. of the spec. */
	StoreAndFindMatchesH10(self, data, ix, mask, 128, max_backward, nil, nil)
}

func StoreRangeH10(handle HasherHandle, data []byte, mask uint, ix_start uint, ix_end uint) {
	var i uint = ix_start
	var j uint = ix_start
	if ix_start+63 <= ix_end {
		i = ix_end - 63
	}

	if ix_start+512 <= i {
		for ; j < i; j += 8 {
			StoreH10(handle, data, mask, j)
		}
	}

	for ; i < ix_end; i++ {
		StoreH10(handle, data, mask, i)
	}
}

func StitchToPreviousBlockH10(handle HasherHandle, num_bytes uint, position uint, ringbuffer []byte, ringbuffer_mask uint) {
	var self *H10 = SelfH10(handle)
	if num_bytes >= HashTypeLengthH10()-1 && position >= 128 {
		var i_start uint = position - 128 + 1
		var i_end uint = brotli_min_size_t(position, i_start+num_bytes)
		/* Store the last `128 - 1` positions in the hasher.
		   These could not be calculated before, since they require knowledge
		   of both the previous and the current block. */

		var i uint
		for i = i_start; i < i_end; i++ {
			/* Maximum distance is window size - 16, see section 9.1. of the spec.
			   Furthermore, we have to make sure that we don't look further back
			   from the start of the next block than the window size, otherwise we
			   could access already overwritten areas of the ring-buffer. */
			var max_backward uint = self.window_mask_ - brotli_max_size_t(BROTLI_WINDOW_GAP-1, position-i)

			/* We know that i + 128 <= position + num_bytes, i.e. the
			   end of the current block and that we have at least
			   128 tail in the ring-buffer. */
			StoreAndFindMatchesH10(self, ringbuffer, i, ringbuffer_mask, 128, max_backward, nil, nil)
		}
	}
}

/* MAX_NUM_MATCHES == 64 + MAX_TREE_SEARCH_DEPTH */
const MAX_NUM_MATCHES_H10 = 128
