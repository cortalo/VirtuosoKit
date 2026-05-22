package router

import "errors"

var ErrNoPath = errors.New("no path found")
var ErrOutOfBound = errors.New("out of bound")
var ErrPinMisaligned = errors.New("pin XLow not aligned to M2 track grid")
