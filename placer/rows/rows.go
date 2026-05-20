package rows

import (
	"placer/common"
	"sort"
)

func totalWidth(group []common.SchematicInstance) int {
	w := 0
	for _, inst := range group {
		w += inst.Width
	}
	return w
}

// Group clusters instances into rows by their Y coordinate.
// A gap > threshold between consecutive Y values starts a new row.
// Within each row instances are sorted by X. Rows are sorted bottom to top.
func Group(instances []common.SchematicInstance, threshold float64) [][]common.SchematicInstance {
	if len(instances) == 0 {
		return nil
	}

	sorted := make([]common.SchematicInstance, len(instances))
	copy(sorted, instances)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Y != sorted[j].Y {
			return sorted[i].Y < sorted[j].Y
		}
		return sorted[i].X < sorted[j].X
	})

	var result [][]common.SchematicInstance
	row := []common.SchematicInstance{sorted[0]}
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Y-sorted[i-1].Y > threshold {
			result = append(result, row)
			row = nil
		}
		row = append(row, sorted[i])
	}
	result = append(result, row)

	for _, r := range result {
		sort.Slice(r, func(i, j int) bool {
			return r[i].X < r[j].X
		})
	}

	return result
}

// SplitByWidth re-partitions each group into sub-groups whose total cell width
// (inst.Width) does not exceed targetWidth, preserving instance order.
// If a single instance is wider than targetWidth it is placed alone.
func SplitByWidth(groups [][]common.SchematicInstance, targetWidth int) [][]common.SchematicInstance {
	var result [][]common.SchematicInstance
	for _, group := range groups {
		result = append(result, splitGroup(group, targetWidth)...)
	}
	return result
}

// RepackByWidth packs groups into bins of at most targetWidth using
// First Fit Decreasing, minimising the number of bins (= total slack).
// Groups already wider than targetWidth are placed alone.
func RepackByWidth(groups [][]common.SchematicInstance, targetWidth int) [][]common.SchematicInstance {
	if len(groups) == 0 {
		return nil
	}

	type item struct {
		group []common.SchematicInstance
		width int
	}
	items := make([]item, len(groups))
	for i, g := range groups {
		items[i] = item{group: g, width: totalWidth(g)}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].width > items[j].width
	})

	type bin struct {
		insts []common.SchematicInstance
		used  int
	}
	var bins []bin

	for _, it := range items {
		placed := false
		for b := range bins {
			if bins[b].used+it.width <= targetWidth {
				bins[b].insts = append(bins[b].insts, it.group...)
				bins[b].used += it.width
				placed = true
				break
			}
		}
		if !placed {
			bins = append(bins, bin{insts: append([]common.SchematicInstance(nil), it.group...), used: it.width})
		}
	}

	result := make([][]common.SchematicInstance, len(bins))
	for i, b := range bins {
		result[i] = b.insts
	}
	return result
}

func splitGroup(group []common.SchematicInstance, targetWidth int) [][]common.SchematicInstance {
	var result [][]common.SchematicInstance
	var current []common.SchematicInstance
	currentWidth := 0

	for _, inst := range group {
		if len(current) == 0 {
			current = append(current, inst)
			currentWidth = inst.Width
		} else if currentWidth+inst.Width <= targetWidth {
			current = append(current, inst)
			currentWidth += inst.Width
		} else {
			result = append(result, current)
			current = []common.SchematicInstance{inst}
			currentWidth = inst.Width
		}
	}
	if len(current) > 0 {
		result = append(result, current)
	}
	return result
}
