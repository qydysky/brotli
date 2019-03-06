package brotli

/* NOLINT(build/header_guard) */
/* Copyright 2010 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* A (forgetful) hash table to the data seen by the compressor, to
   help create backward references to previous data.

   This is a hash map of fixed size (bucket_size_) to a ring buffer of
   fixed size (block_size_). The ring buffer contains the last block_size_
   index positions of the given hash key in the compressed data. */
func HashTypeLengthH5() uint {
	return 4
}

func StoreLookaheadH5() uint {
	return 4
}

/* HashBytes is the function that chooses the bucket to place the address in. */
func HashBytesH5(data []byte, shift int) uint32 {
	var h uint32 = BROTLI_UNALIGNED_LOAD32LE(data) * kHashMul32

	/* The higher bits contain more mixture from the multiplication,
	   so we take our results from there. */
	return uint32(h >> uint(shift))
}

type H5 struct {
	bucket_size_ uint
	block_size_  uint
	hash_shift_  int
	block_mask_  uint32
	num          []uint16
	buckets      []uint32
}

func SelfH5(handle HasherHandle) *H5 {
	return handle.extra.(*H5)
}

func NumH5(self *H5) []uint16 {
	return []uint16(self.num)
}

func BucketsH5(self *H5) []uint32 {
	return []uint32(self.buckets)
}

func InitializeH5(handle HasherHandle, params *BrotliEncoderParams) {
	var common *HasherCommon = GetHasherCommon(handle)
	handle.extra = new(H5)
	var self *H5 = SelfH5(handle)
	self.hash_shift_ = 32 - common.params.bucket_bits
	self.bucket_size_ = uint(1) << uint(common.params.bucket_bits)
	self.block_size_ = uint(1) << uint(common.params.block_bits)
	self.block_mask_ = uint32(self.block_size_ - 1)
	self.num = make([]uint16, self.bucket_size_)
	self.buckets = make([]uint32, self.block_size_*self.bucket_size_)
}

func PrepareH5(handle HasherHandle, one_shot bool, input_size uint, data []byte) {
	var self *H5 = SelfH5(handle)
	var num []uint16 = NumH5(self)
	var partial_prepare_threshold uint = self.bucket_size_ >> 6
	/* Partial preparation is 100 times slower (per socket). */
	if one_shot && input_size <= partial_prepare_threshold {
		var i uint
		for i = 0; i < input_size; i++ {
			var key uint32 = HashBytesH5(data[i:], self.hash_shift_)
			num[key] = 0
		}
	} else {
		for i := 0; i < int(self.bucket_size_); i++ {
			num[i] = 0
		}
	}
}

/* Look at 4 bytes at &data[ix & mask].
   Compute a hash from these, and store the value of ix at that position. */
func StoreH5(handle HasherHandle, data []byte, mask uint, ix uint) {
	var self *H5 = SelfH5(handle)
	var num []uint16 = NumH5(self)
	var key uint32 = HashBytesH5(data[ix&mask:], self.hash_shift_)
	var minor_ix uint = uint(num[key]) & uint(self.block_mask_)
	var offset uint = minor_ix + uint(key<<uint(GetHasherCommon(handle).params.block_bits))
	BucketsH5(self)[offset] = uint32(ix)
	num[key]++
}

func StoreRangeH5(handle HasherHandle, data []byte, mask uint, ix_start uint, ix_end uint) {
	var i uint
	for i = ix_start; i < ix_end; i++ {
		StoreH5(handle, data, mask, i)
	}
}

func StitchToPreviousBlockH5(handle HasherHandle, num_bytes uint, position uint, ringbuffer []byte, ringbuffer_mask uint) {
	if num_bytes >= HashTypeLengthH5()-1 && position >= 3 {
		/* Prepare the hashes for three last bytes of the last write.
		   These could not be calculated before, since they require knowledge
		   of both the previous and the current block. */
		StoreH5(handle, ringbuffer, ringbuffer_mask, position-3)

		StoreH5(handle, ringbuffer, ringbuffer_mask, position-2)
		StoreH5(handle, ringbuffer, ringbuffer_mask, position-1)
	}
}

func PrepareDistanceCacheH5(handle HasherHandle, distance_cache []int) {
	PrepareDistanceCache(distance_cache, GetHasherCommon(handle).params.num_last_distances_to_check)
}

/* Find a longest backward match of &data[cur_ix] up to the length of
   max_length and stores the position cur_ix in the hash table.

   REQUIRES: PrepareDistanceCacheH5 must be invoked for current distance cache
             values; if this method is invoked repeatedly with the same distance
             cache values, it is enough to invoke PrepareDistanceCacheH5 once.

   Does not look for matches longer than max_length.
   Does not look for matches further away than max_backward.
   Writes the best match into |out|.
   |out|->score is updated only if a better match is found. */
func FindLongestMatchH5(handle HasherHandle, dictionary *BrotliEncoderDictionary, data []byte, ring_buffer_mask uint, distance_cache []int, cur_ix uint, max_length uint, max_backward uint, gap uint, max_distance uint, out *HasherSearchResult) {
	var common *HasherCommon = GetHasherCommon(handle)
	var self *H5 = SelfH5(handle)
	var num []uint16 = NumH5(self)
	var buckets []uint32 = BucketsH5(self)
	var cur_ix_masked uint = cur_ix & ring_buffer_mask
	var min_score uint = out.score
	var best_score uint = out.score
	var best_len uint = out.len
	var i uint
	var bucket []uint32
	/* Don't accept a short copy from far away. */
	out.len = 0

	out.len_code_delta = 0

	/* Try last distance first. */
	for i = 0; i < uint(common.params.num_last_distances_to_check); i++ {
		var backward uint = uint(distance_cache[i])
		var prev_ix uint = uint(cur_ix - backward)
		if prev_ix >= cur_ix {
			continue
		}

		if backward > max_backward {
			continue
		}

		prev_ix &= ring_buffer_mask

		if cur_ix_masked+best_len > ring_buffer_mask || prev_ix+best_len > ring_buffer_mask || data[cur_ix_masked+best_len] != data[prev_ix+best_len] {
			continue
		}
		{
			var len uint = FindMatchLengthWithLimit(data[prev_ix:], data[cur_ix_masked:], max_length)
			if len >= 3 || (len == 2 && i < 2) {
				/* Comparing for >= 2 does not change the semantics, but just saves for
				   a few unnecessary binary logarithms in backward reference score,
				   since we are not interested in such short matches. */
				var score uint = BackwardReferenceScoreUsingLastDistance(uint(len))
				if best_score < score {
					if i != 0 {
						score -= BackwardReferencePenaltyUsingLastDistance(i)
					}
					if best_score < score {
						best_score = score
						best_len = uint(len)
						out.len = best_len
						out.distance = backward
						out.score = best_score
					}
				}
			}
		}
	}
	{
		var key uint32 = HashBytesH5(data[cur_ix_masked:], self.hash_shift_)
		bucket = buckets[key<<uint(common.params.block_bits):]
		var down uint
		if uint(num[key]) > self.block_size_ {
			down = uint(num[key]) - self.block_size_
		} else {
			down = 0
		}
		for i = uint(num[key]); i > down; {
			var prev_ix uint
			i--
			prev_ix = uint(bucket[uint32(i)&self.block_mask_])
			var backward uint = cur_ix - prev_ix
			if backward > max_backward {
				break
			}

			prev_ix &= ring_buffer_mask
			if cur_ix_masked+best_len > ring_buffer_mask || prev_ix+best_len > ring_buffer_mask || data[cur_ix_masked+best_len] != data[prev_ix+best_len] {
				continue
			}
			{
				var len uint = FindMatchLengthWithLimit(data[prev_ix:], data[cur_ix_masked:], max_length)
				if len >= 4 {
					/* Comparing for >= 3 does not change the semantics, but just saves
					   for a few unnecessary binary logarithms in backward reference
					   score, since we are not interested in such short matches. */
					var score uint = BackwardReferenceScore(uint(len), backward)
					if best_score < score {
						best_score = score
						best_len = uint(len)
						out.len = best_len
						out.distance = backward
						out.score = best_score
					}
				}
			}
		}

		bucket[uint32(num[key])&self.block_mask_] = uint32(cur_ix)
		num[key]++
	}

	if min_score == out.score {
		SearchInStaticDictionary(dictionary, handle, data[cur_ix_masked:], max_length, max_backward+gap, max_distance, out, false)
	}
}
