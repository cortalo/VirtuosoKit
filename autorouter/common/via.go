package common

// PlaceViaCuts places cut shapes in the overlap region of shapes a and b.
// vc defines the cut geometry; cutLayer is the layer for the cut shapes;
// endExt is the via enclosure (the overlap region is inset by endExt on each side).
// Returns shapes unchanged if vc has zero dimensions or there is no valid overlap.
func PlaceViaCuts(shapes []Shape, a, b Shape, vc ViaConfig, cutLayer Layer, endExt int) []Shape {
	if vc.CutW == 0 || vc.CutH == 0 {
		return shapes
	}
	x0 := max(a.LowerLeft.X, b.LowerLeft.X) + endExt
	y0 := max(a.LowerLeft.Y, b.LowerLeft.Y) + endExt
	x1 := min(a.UpperRight.X, b.UpperRight.X) - endExt
	y1 := min(a.UpperRight.Y, b.UpperRight.Y) - endExt
	if x0 >= x1 || y0 >= y1 {
		return shapes
	}
	w, h := x1-x0, y1-y0
	cols := max(1, (w+vc.SpaceX)/(vc.CutW+vc.SpaceX))
	rows := max(1, (h+vc.SpaceY)/(vc.CutH+vc.SpaceY))
	startX := (x0+x1)/2 - (cols*vc.CutW+(cols-1)*vc.SpaceX)/2
	startY := (y0+y1)/2 - (rows*vc.CutH+(rows-1)*vc.SpaceY)/2
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			llx := startX + c*(vc.CutW+vc.SpaceX)
			lly := startY + r*(vc.CutH+vc.SpaceY)
			shapes = append(shapes, Shape{
				LowerLeft:  Point{X: llx, Y: lly},
				UpperRight: Point{X: llx + vc.CutW, Y: lly + vc.CutH},
				Layer:      cutLayer,
				NetID:      a.NetID,
			})
		}
	}
	return shapes
}
