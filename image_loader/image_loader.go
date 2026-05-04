package imageloader

import (
	"fmt"
	"im2mlog/common"
	"im2mlog/drawtree"
	"image"
	"image/png"
	"os"
	"sort"

	"golang.org/x/image/draw"
)

func BuildTopPalette(img [][]common.Pixel, maxColors int) []common.Pixel {
	if maxColors <= 0 {
		return nil
	}

	counts := make(map[common.Pixel]int)

	for y := range img {
		for x := range img[y] {
			counts[img[y][x]]++
		}
	}

	items := make([]common.ColorCount, 0, len(counts))
	for p, c := range counts {
		items = append(items, common.ColorCount{
			P: p,
			C: c,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].C > items[j].C
	})

	if maxColors > len(items) {
		maxColors = len(items)
	}

	palette := make([]common.Pixel, 0, maxColors)
	for i := 0; i < maxColors; i++ {
		palette = append(palette, items[i].P)
	}

	return palette
}

func NearestColor(p common.Pixel, palette []common.Pixel) common.Pixel {
	if len(palette) == 0 {
		return p
	}

	best := palette[0]
	bestDist := common.ColorDistanceSq(p, best)

	for i := 1; i < len(palette); i++ {
		d := common.ColorDistanceSq(p, palette[i])
		if d < bestDist {
			bestDist = d
			best = palette[i]
		}
	}

	return best
}

func ApplyPalette(img [][]common.Pixel, palette []common.Pixel) {
	if len(palette) == 0 {
		return
	}

	cache := make(map[common.Pixel]common.Pixel)

	for y := range img {
		for x := range img[y] {
			p := img[y][x]

			if v, ok := cache[p]; ok {
				img[y][x] = v
				continue
			}

			nc := NearestColor(p, palette)
			cache[p] = nc
			img[y][x] = nc
		}
	}
}

func LoadAsRGBA(filePath string, w, h int) ([][]common.Pixel, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	fmt.Println("Формат:", format)

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(rgba, rgba.Bounds(), img, img.Bounds(), draw.Over, nil)

	width := rgba.Bounds().Dx()
	height := rgba.Bounds().Dy()
	res := make([][]common.Pixel, height)

	for y := 0; y < height; y++ {
		res[y] = make([]common.Pixel, width)
		for x := 0; x < width; x++ {
			idx := y*rgba.Stride + x*4
			res[y][x] = common.Pixel{
				rgba.Pix[idx],
				rgba.Pix[idx+1],
				rgba.Pix[idx+2],
				rgba.Pix[idx+3],
			}

			if common.PaletteStep > 1 {
				res[y][x] = QuantizePixel(res[y][x], common.PaletteStep)
			}
		}
	}

	return res, nil
}

func QuantizeChannel(v uint8, step int) uint8 {
	if step <= 1 {
		return v
	}

	x := int(v)
	q := ((x + step/2) / step) * step

	if q < 0 {
		q = 0
	}
	if q > 255 {
		q = 255
	}

	return uint8(q)
}

func QuantizePixel(p common.Pixel, step int) common.Pixel {
	if step <= 1 {
		return p
	}

	return common.Pixel{
		QuantizeChannel(p[0], step),
		QuantizeChannel(p[1], step),
		QuantizeChannel(p[2], step),
		p[3],
	}
}

func SaveDrawPartsAsPNG(parts [][]*drawtree.DrawNode, filepath string, w, h int) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	drawNode := func(n *drawtree.DrawNode) {
		if n == nil {
			return
		}

		for yy := n.Y; yy < n.Y+n.H; yy++ {
			if yy < 0 || yy >= h {
				continue
			}

			for xx := n.X; xx < n.X+n.W; xx++ {
				if xx < 0 || xx >= w {
					continue
				}

				idx := yy*img.Stride + xx*4

				img.Pix[idx+0] = n.P[0]
				img.Pix[idx+1] = n.P[1]
				img.Pix[idx+2] = n.P[2]
				img.Pix[idx+3] = n.P[3]
			}
		}
	}

	for _, part := range parts {
		for _, n := range part {
			drawNode(n)
		}
	}

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}
