package rows

import (
	"placer/common"
	"sort"
)

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
