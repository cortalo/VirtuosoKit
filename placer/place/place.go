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
func Place(rows [][]common.SchematicInstance, db CellDB, rowHeight int) ([]common.Instance, error) {
	var result []common.Instance
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
		}
	}
	return result, nil
}
