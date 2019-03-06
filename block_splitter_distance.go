package brotli

/* NOLINT(build/header_guard) */
/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/
func InitialEntropyCodesDistance(data []uint16, length uint, stride uint, num_histograms uint, histograms []HistogramDistance) {
	var seed uint32 = 7
	var block_length uint = length / num_histograms
	var i uint
	ClearHistogramsDistance(histograms, num_histograms)
	for i = 0; i < num_histograms; i++ {
		var pos uint = length * i / num_histograms
		if i != 0 {
			pos += uint(MyRand(&seed) % uint32(block_length))
		}

		if pos+stride >= length {
			pos = length - stride - 1
		}

		HistogramAddVectorDistance(&histograms[i], data[pos:], stride)
	}
}

func RandomSampleDistance(seed *uint32, data []uint16, length uint, stride uint, sample *HistogramDistance) {
	var pos uint = 0
	if stride >= length {
		stride = length
	} else {
		pos = uint(MyRand(seed) % uint32(length-stride+1))
	}

	HistogramAddVectorDistance(sample, data[pos:], stride)
}

func RefineEntropyCodesDistance(data []uint16, length uint, stride uint, num_histograms uint, histograms []HistogramDistance) {
	var iters uint = kIterMulForRefining*length/stride + kMinItersForRefining
	var seed uint32 = 7
	var iter uint
	iters = ((iters + num_histograms - 1) / num_histograms) * num_histograms
	for iter = 0; iter < iters; iter++ {
		var sample HistogramDistance
		HistogramClearDistance(&sample)
		RandomSampleDistance(&seed, data, length, stride, &sample)
		HistogramAddHistogramDistance(&histograms[iter%num_histograms], &sample)
	}
}

/* Assigns a block id from the range [0, num_histograms) to each data element
   in data[0..length) and fills in block_id[0..length) with the assigned values.
   Returns the number of blocks, i.e. one plus the number of block switches. */
func FindBlocksDistance(data []uint16, length uint, block_switch_bitcost float64, num_histograms uint, histograms []HistogramDistance, insert_cost []float64, cost []float64, switch_signal []byte, block_id []byte) uint {
	var data_size uint = HistogramDataSizeDistance()
	var bitmaplen uint = (num_histograms + 7) >> 3
	var num_blocks uint = 1
	var i uint
	var j uint
	assert(num_histograms <= 256)
	if num_histograms <= 1 {
		for i = 0; i < length; i++ {
			block_id[i] = 0
		}

		return 1
	}

	for i := 0; i < int(data_size*num_histograms); i++ {
		insert_cost[i] = 0
	}
	for i = 0; i < num_histograms; i++ {
		insert_cost[i] = FastLog2(uint(uint32(histograms[i].total_count_)))
	}

	for i = data_size; i != 0; {
		i--
		for j = 0; j < num_histograms; j++ {
			insert_cost[i*num_histograms+j] = insert_cost[j] - BitCost(uint(histograms[j].data_[i]))
		}
	}

	for i := 0; i < int(num_histograms); i++ {
		cost[i] = 0
	}
	for i := 0; i < int(length*bitmaplen); i++ {
		switch_signal[i] = 0
	}

	/* After each iteration of this loop, cost[k] will contain the difference
	   between the minimum cost of arriving at the current byte position using
	   entropy code k, and the minimum cost of arriving at the current byte
	   position. This difference is capped at the block switch cost, and if it
	   reaches block switch cost, it means that when we trace back from the last
	   position, we need to switch here. */
	for i = 0; i < length; i++ {
		var byte_ix uint = i
		var ix uint = byte_ix * bitmaplen
		var insert_cost_ix uint = uint(data[byte_ix]) * num_histograms
		var min_cost float64 = 1e99
		var block_switch_cost float64 = block_switch_bitcost
		var k uint
		for k = 0; k < num_histograms; k++ {
			/* We are coding the symbol in data[byte_ix] with entropy code k. */
			cost[k] += insert_cost[insert_cost_ix+k]

			if cost[k] < min_cost {
				min_cost = cost[k]
				block_id[byte_ix] = byte(k)
			}
		}

		/* More blocks for the beginning. */
		if byte_ix < 2000 {
			block_switch_cost *= 0.77 + 0.07*float64(byte_ix)/2000
		}

		for k = 0; k < num_histograms; k++ {
			cost[k] -= min_cost
			if cost[k] >= block_switch_cost {
				var mask byte = byte(1 << (k & 7))
				cost[k] = block_switch_cost
				assert(k>>3 < bitmaplen)
				switch_signal[ix+(k>>3)] |= mask
				/* Trace back from the last position and switch at the marked places. */
			}
		}
	}
	{
		var byte_ix uint = length - 1
		var ix uint = byte_ix * bitmaplen
		var cur_id byte = block_id[byte_ix]
		for byte_ix > 0 {
			var mask byte = byte(1 << (cur_id & 7))
			assert(uint(cur_id)>>3 < bitmaplen)
			byte_ix--
			ix -= bitmaplen
			if switch_signal[ix+uint(cur_id>>3)]&mask != 0 {
				if cur_id != block_id[byte_ix] {
					cur_id = block_id[byte_ix]
					num_blocks++
				}
			}

			block_id[byte_ix] = cur_id
		}
	}

	return num_blocks
}

var RemapBlockIdsDistance_kInvalidId uint16 = 256

func RemapBlockIdsDistance(block_ids []byte, length uint, new_id []uint16, num_histograms uint) uint {
	var next_id uint16 = 0
	var i uint
	for i = 0; i < num_histograms; i++ {
		new_id[i] = RemapBlockIdsDistance_kInvalidId
	}

	for i = 0; i < length; i++ {
		assert(uint(block_ids[i]) < num_histograms)
		if new_id[block_ids[i]] == RemapBlockIdsDistance_kInvalidId {
			new_id[block_ids[i]] = next_id
			next_id++
		}
	}

	for i = 0; i < length; i++ {
		block_ids[i] = byte(new_id[block_ids[i]])
		assert(uint(block_ids[i]) < num_histograms)
	}

	assert(uint(next_id) <= num_histograms)
	return uint(next_id)
}

func BuildBlockHistogramsDistance(data []uint16, length uint, block_ids []byte, num_histograms uint, histograms []HistogramDistance) {
	var i uint
	ClearHistogramsDistance(histograms, num_histograms)
	for i = 0; i < length; i++ {
		HistogramAddDistance(&histograms[block_ids[i]], uint(data[i]))
	}
}

var ClusterBlocksDistance_kInvalidIndex uint32 = BROTLI_UINT32_MAX

func ClusterBlocksDistance(data []uint16, length uint, num_blocks uint, block_ids []byte, split *BlockSplit) {
	var histogram_symbols []uint32 = make([]uint32, num_blocks)
	var block_lengths []uint32 = make([]uint32, num_blocks)
	var expected_num_clusters uint = CLUSTERS_PER_BATCH * (num_blocks + HISTOGRAMS_PER_BATCH - 1) / HISTOGRAMS_PER_BATCH
	var all_histograms_size uint = 0
	var all_histograms_capacity uint = expected_num_clusters
	var all_histograms []HistogramDistance = make([]HistogramDistance, all_histograms_capacity)
	var cluster_size_size uint = 0
	var cluster_size_capacity uint = expected_num_clusters
	var cluster_size []uint32 = make([]uint32, cluster_size_capacity)
	var num_clusters uint = 0
	var histograms []HistogramDistance = make([]HistogramDistance, brotli_min_size_t(num_blocks, HISTOGRAMS_PER_BATCH))
	var max_num_pairs uint = HISTOGRAMS_PER_BATCH * HISTOGRAMS_PER_BATCH / 2
	var pairs_capacity uint = max_num_pairs + 1
	var pairs []HistogramPair = make([]HistogramPair, pairs_capacity)
	var pos uint = 0
	var clusters []uint32
	var num_final_clusters uint
	var new_index []uint32
	var i uint
	var sizes = [HISTOGRAMS_PER_BATCH]uint32{0}
	var new_clusters = [HISTOGRAMS_PER_BATCH]uint32{0}
	var symbols = [HISTOGRAMS_PER_BATCH]uint32{0}
	var remap = [HISTOGRAMS_PER_BATCH]uint32{0}

	for i := 0; i < int(num_blocks); i++ {
		block_lengths[i] = 0
	}
	{
		var block_idx uint = 0
		for i = 0; i < length; i++ {
			assert(block_idx < num_blocks)
			block_lengths[block_idx]++
			if i+1 == length || block_ids[i] != block_ids[i+1] {
				block_idx++
			}
		}

		assert(block_idx == num_blocks)
	}

	for i = 0; i < num_blocks; i += HISTOGRAMS_PER_BATCH {
		var num_to_combine uint = brotli_min_size_t(num_blocks-i, HISTOGRAMS_PER_BATCH)
		var num_new_clusters uint
		var j uint
		for j = 0; j < num_to_combine; j++ {
			var k uint
			HistogramClearDistance(&histograms[j])
			for k = 0; uint32(k) < block_lengths[i+j]; k++ {
				HistogramAddDistance(&histograms[j], uint(data[pos]))
				pos++
			}

			histograms[j].bit_cost_ = BrotliPopulationCostDistance(&histograms[j])
			new_clusters[j] = uint32(j)
			symbols[j] = uint32(j)
			sizes[j] = 1
		}

		num_new_clusters = BrotliHistogramCombineDistance(histograms, sizes[:], symbols[:], new_clusters[:], []HistogramPair(pairs), num_to_combine, num_to_combine, HISTOGRAMS_PER_BATCH, max_num_pairs)
		if all_histograms_capacity < (all_histograms_size + num_new_clusters) {
			var _new_size uint
			if all_histograms_capacity == 0 {
				_new_size = all_histograms_size + num_new_clusters
			} else {
				_new_size = all_histograms_capacity
			}
			var new_array []HistogramDistance
			for _new_size < (all_histograms_size + num_new_clusters) {
				_new_size *= 2
			}
			new_array = make([]HistogramDistance, _new_size)
			if all_histograms_capacity != 0 {
				copy(new_array, all_histograms[:all_histograms_capacity])
			}

			all_histograms = new_array
			all_histograms_capacity = _new_size
		}

		brotli_ensure_capacity_uint32_t(&cluster_size, &cluster_size_capacity, cluster_size_size+num_new_clusters)
		for j = 0; j < num_new_clusters; j++ {
			all_histograms[all_histograms_size] = histograms[new_clusters[j]]
			all_histograms_size++
			cluster_size[cluster_size_size] = sizes[new_clusters[j]]
			cluster_size_size++
			remap[new_clusters[j]] = uint32(j)
		}

		for j = 0; j < num_to_combine; j++ {
			histogram_symbols[i+j] = uint32(num_clusters) + remap[symbols[j]]
		}

		num_clusters += num_new_clusters
		assert(num_clusters == cluster_size_size)
		assert(num_clusters == all_histograms_size)
	}

	histograms = nil

	max_num_pairs = brotli_min_size_t(64*num_clusters, (num_clusters/2)*num_clusters)
	if pairs_capacity < max_num_pairs+1 {
		pairs = nil
		pairs = make([]HistogramPair, (max_num_pairs + 1))
	}

	clusters = make([]uint32, num_clusters)
	for i = 0; i < num_clusters; i++ {
		clusters[i] = uint32(i)
	}

	num_final_clusters = BrotliHistogramCombineDistance(all_histograms, cluster_size, histogram_symbols, clusters, pairs, num_clusters, num_blocks, BROTLI_MAX_NUMBER_OF_BLOCK_TYPES, max_num_pairs)
	pairs = nil
	cluster_size = nil

	new_index = make([]uint32, num_clusters)
	for i = 0; i < num_clusters; i++ {
		new_index[i] = ClusterBlocksDistance_kInvalidIndex
	}
	pos = 0
	{
		var next_index uint32 = 0
		for i = 0; i < num_blocks; i++ {
			var histo HistogramDistance
			var j uint
			var best_out uint32
			var best_bits float64
			HistogramClearDistance(&histo)
			for j = 0; uint32(j) < block_lengths[i]; j++ {
				HistogramAddDistance(&histo, uint(data[pos]))
				pos++
			}

			if i == 0 {
				best_out = histogram_symbols[0]
			} else {
				best_out = histogram_symbols[i-1]
			}
			best_bits = BrotliHistogramBitCostDistanceDistance(&histo, &all_histograms[best_out])
			for j = 0; j < num_final_clusters; j++ {
				var cur_bits float64 = BrotliHistogramBitCostDistanceDistance(&histo, &all_histograms[clusters[j]])
				if cur_bits < best_bits {
					best_bits = cur_bits
					best_out = clusters[j]
				}
			}

			histogram_symbols[i] = best_out
			if new_index[best_out] == ClusterBlocksDistance_kInvalidIndex {
				new_index[best_out] = next_index
				next_index++
			}
		}
	}

	clusters = nil
	all_histograms = nil
	brotli_ensure_capacity_uint8_t(&split.types, &split.types_alloc_size, num_blocks)
	brotli_ensure_capacity_uint32_t(&split.lengths, &split.lengths_alloc_size, num_blocks)
	{
		var cur_length uint32 = 0
		var block_idx uint = 0
		var max_type byte = 0
		for i = 0; i < num_blocks; i++ {
			cur_length += block_lengths[i]
			if i+1 == num_blocks || histogram_symbols[i] != histogram_symbols[i+1] {
				var id byte = byte(new_index[histogram_symbols[i]])
				split.types[block_idx] = id
				split.lengths[block_idx] = cur_length
				max_type = brotli_max_uint8_t(max_type, id)
				cur_length = 0
				block_idx++
			}
		}

		split.num_blocks = block_idx
		split.num_types = uint(max_type) + 1
	}

	new_index = nil
	block_lengths = nil
	histogram_symbols = nil
}

func SplitByteVectorDistance(data []uint16, length uint, literals_per_histogram uint, max_histograms uint, sampling_stride_length uint, block_switch_cost float64, params *BrotliEncoderParams, split *BlockSplit) {
	var data_size uint = HistogramDataSizeDistance()
	var num_histograms uint = length/literals_per_histogram + 1
	var histograms []HistogramDistance
	if num_histograms > max_histograms {
		num_histograms = max_histograms
	}

	if length == 0 {
		split.num_types = 1
		return
	} else if length < kMinLengthForBlockSplitting {
		brotli_ensure_capacity_uint8_t(&split.types, &split.types_alloc_size, split.num_blocks+1)
		brotli_ensure_capacity_uint32_t(&split.lengths, &split.lengths_alloc_size, split.num_blocks+1)
		split.num_types = 1
		split.types[split.num_blocks] = 0
		split.lengths[split.num_blocks] = uint32(length)
		split.num_blocks++
		return
	}

	histograms = make([]HistogramDistance, num_histograms)

	/* Find good entropy codes. */
	InitialEntropyCodesDistance(data, length, sampling_stride_length, num_histograms, histograms)

	RefineEntropyCodesDistance(data, length, sampling_stride_length, num_histograms, histograms)
	{
		var block_ids []byte = make([]byte, length)
		var num_blocks uint = 0
		var bitmaplen uint = (num_histograms + 7) >> 3
		var insert_cost []float64 = make([]float64, (data_size * num_histograms))
		var cost []float64 = make([]float64, num_histograms)
		var switch_signal []byte = make([]byte, (length * bitmaplen))
		var new_id []uint16 = make([]uint16, num_histograms)
		var iters uint
		if params.quality < HQ_ZOPFLIFICATION_QUALITY {
			iters = 3
		} else {
			iters = 10
		}
		/* Find a good path through literals with the good entropy codes. */

		var i uint
		for i = 0; i < iters; i++ {
			num_blocks = FindBlocksDistance(data, length, block_switch_cost, num_histograms, histograms, insert_cost, cost, switch_signal, block_ids)
			num_histograms = RemapBlockIdsDistance(block_ids, length, new_id, num_histograms)
			BuildBlockHistogramsDistance(data, length, block_ids, num_histograms, histograms)
		}

		insert_cost = nil
		cost = nil
		switch_signal = nil
		new_id = nil
		histograms = nil
		ClusterBlocksDistance(data, length, num_blocks, block_ids, split)
		block_ids = nil
	}
}
