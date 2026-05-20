package place

import (
	"fmt"
	"placer/common"
)

type CellDB interface {
	Query(lib, cell string) (width int, err error)
}

// Place converts grouped schematic rows into tightly packed layout instances.
//
// Row orientation alternates to align power rails between adjacent rows:
//   - even rows: R0 at y = i * rowHeight  (VSS at bottom, VDD at top)
//   - odd rows:  MX at y = (i+1)*rowHeight (cell extends downward, VDD at bottom)
//
// This causes adjacent rows to share a rail: VDD-VDD between row 0 and 1,
// VSS-VSS between row 1 and 2, and so on.
// Within each row cells are placed left to right starting at x=0.
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
		var orient common.Orient
		var y int
		if i%2 == 0 {
			orient = common.R0
			y = i * rowHeight
		} else {
			orient = common.MX
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
				Orient: orient,
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
			result = append(result, common.Instance{
				Name:   si.Name,
				Lib:    si.Lib,
				Cell:   si.Cell,
				X:      x,
				Y:      y,
				Orient: orient,
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
