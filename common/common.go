package common

// program wide limits

var MinArea = 1
var MaxNodes = 450
var MaxDrawOp = 3
var PaletteSize = 0
var PaletteStep = 1

var ProgramOverheadLines = 6

var DisplayTilePixels = 32
var DisplayBlockName = "tile-logic-display"
var ProcessorBlockName = "micro-processor"

type Pixel [4]uint8

func (p Pixel) IsEqual(t Pixel) bool {
	return p[0] == t[0] && p[1] == t[1] && p[2] == t[2] && p[3] == t[3]
}

func (p Pixel) Copy() Pixel {
	return Pixel{p[0], p[1], p[2], p[3]}
}

func (p Pixel) Order(t Pixel) bool {
	for i := 0; i < 4; i++ {
		if p[i] != t[i] {
			return p[i] < t[i]
		}
	}
	return false
}

func ColorDistanceSq(a, b Pixel) int {
	dr := int(a[0]) - int(b[0])
	dg := int(a[1]) - int(b[1])
	db := int(a[2]) - int(b[2])

	return dr*dr + dg*dg + db*db
}

type ColorCount struct {
	P Pixel
	C int
}
