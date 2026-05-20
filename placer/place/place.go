package place

import (
	"fmt"
	"placer/common"
)

type CellDB interface {
	Query(lib, cell string) (width int, err error)
}

// hasHorizontalFlip reports whether orient contains a MY (left-right mirror)
// component. The vertical component is ignored because row parity controls it.
func hasHorizontalFlip(o common.Orient) bool {
	return o == common.MY || o == common.R180 || o == common.MYR90
}

// combineOrient merges the row-level vertical flip with the horizontal flip
// extracted from the schematic orientation.
//
//	rowFlipped=false, no hFlip  → R0
//	rowFlipped=false, hFlip     → MY
//	rowFlipped=true,  no hFlip  → MX
//	rowFlipped=true,  hFlip     → R180
func combineOrient(rowFlipped bool, schOrient common.Orient) common.Orient {
	hFlip := hasHorizontalFlip(schOrient)
	switch {
	case rowFlipped && hFlip:
		return common.R180
	case rowFlipped:
		return common.MX
	case hFlip:
		return common.MY
	default:
		return common.R0
	}
}

// Place converts grouped schematic rows into tightly packed layout instances.
//
// Row orientation alternates to align power rails between adjacent rows:
//   - even rows: R0 at y = i * rowHeight  (VSS at bottom, VDD at top)
//   - odd rows:  MX at y = (i+1)*rowHeight (cell extends downward, VDD at bottom)
//
// Per-instance orient is derived by combining the row's vertical flip with the
// horizontal flip present in the schematic orientation (MY component only).
// Tap cells always follow the row orient.
//
// If tapcell is non-nil, each row starts and ends with a tap cell and an
// additional tap cell is inserted whenever the distance from the last tap cell
// exceeds tapcell.MaxSpacing. Inserted tap cells are named _TAP_R{row}_{idx}.
func Place(rows [][]common.SchematicInstance, db CellDB, rowHeight int, tapcell *common.TapcellConfig) ([]common.Instance, error) {
	var result []common.Instance

	var tapWidth int
	if tapcell != nil {
		var err error
		tapWidth, err = db.Query(tapcell.Lib, tapcell.Cell)
		if err != nil {
			return nil, fmt.Errorf("place: tapcell: %w", err)
		}
	}

	for i, row := range rows {
		rowFlipped := i%2 != 0
		var rowOrient common.Orient
		var y int
		if !rowFlipped {
			rowOrient = common.R0
			y = i * rowHeight
		} else {
			rowOrient = common.MX
			y = (i + 1) * rowHeight
		}

		x := 0
		tapIdx := 0
		lastTapEnd := 0

		placeTap := func() {
			result = append(result, common.Instance{
				Name:   fmt.Sprintf("_TAP_R%d_%d", i, tapIdx),
				Lib:    tapcell.Lib,
				Cell:   tapcell.Cell,
				X:      x,
				Y:      y,
				Orient: rowOrient,
			})
			x += tapWidth
			lastTapEnd = x
			tapIdx++
		}

		if tapcell != nil {
			placeTap()
		}

		for _, si := range row {
			width, err := db.Query(si.Lib, si.Cell)
			if err != nil {
				return nil, fmt.Errorf("place: row %d instance %q: %w", i, si.Name, err)
			}
			finalOrient := combineOrient(rowFlipped, si.Orient)
			// For MY/R180 the instance origin is the right edge (Virtuoso: x'=-x+Xinst),
			// so shift the reference point by width to keep cells abutted left-to-right.
			instX := x
			if hasHorizontalFlip(finalOrient) {
				instX = x + width
			}
			result = append(result, common.Instance{
				Name:   si.Name,
				Lib:    si.Lib,
				Cell:   si.Cell,
				X:      instX,
				Y:      y,
				Orient: finalOrient,
			})
			x += width

			if tapcell != nil && x-lastTapEnd > tapcell.MaxSpacing {
				placeTap()
			}
		}

		if tapcell != nil {
			placeTap()
		}
	}
	return result, nil
}
