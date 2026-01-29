package lib

import "iter"

func CollectSeq[V any](seq iter.Seq[V]) []V {
	result := make([]V, 0)
	for v := range seq {
		result = append(result, v)
	}
	return result
}
