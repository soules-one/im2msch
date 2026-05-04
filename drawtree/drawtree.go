package drawtree

import (
	"im2mlog/common"
	"im2mlog/quadtree"
	"sort"
)

type DrawNode struct {
	X, Y int
	W, H int
	P    common.Pixel
	Rank int
}

func MakeDrawNodes(n *quadtree.Node, r map[int][]*DrawNode, force ...bool) {
	if n == nil {
		return
	}

	forced := len(force) > 0 && force[0]

	if forced || n.Parent == nil || !n.Parent.P.IsEqual(n.P) {
		d := &DrawNode{
			X:    n.X,
			Y:    n.Y,
			W:    n.W,
			H:    n.H,
			P:    n.P.Copy(),
			Rank: n.Rank,
		}

		r[d.Rank] = append(r[d.Rank], d)
	}

	if !n.IsLeaf {
		for i := 0; i < 4; i++ {
			MakeDrawNodes(n.Children[i], r)
		}
	}
}

func NodesToDrawNodes(part []*quadtree.Node) map[int][]*DrawNode {
	r := make(map[int][]*DrawNode)

	for _, v := range part {
		MakeDrawNodes(v, r, true)
	}

	return r
}

func OptimizeRankMap(r map[int][]*DrawNode) {
	for rank, nodes := range r {
		r[rank] = OptimizeRank(nodes)
	}
}

func OptimizeRank(nodes []*DrawNode) []*DrawNode {
	byColor := make(map[common.Pixel][]*DrawNode)

	for _, n := range nodes {
		if n == nil {
			continue
		}
		byColor[n.P] = append(byColor[n.P], n)
	}

	res := make([]*DrawNode, 0)

	for _, group := range byColor {
		rects := RectangulateMask(group)
		res = append(res, rects...)
	}

	return res
}

func RectangulateMask(nodes []*DrawNode) []*DrawNode {
	if len(nodes) == 0 {
		return nil
	}

	first := nodes[0]

	minX := first.X
	minY := first.Y
	maxX := first.X + first.W
	maxY := first.Y + first.H

	color := first.P
	rank := first.Rank

	for _, n := range nodes {
		if n == nil {
			continue
		}

		if n.X < minX {
			minX = n.X
		}
		if n.Y < minY {
			minY = n.Y
		}
		if n.X+n.W > maxX {
			maxX = n.X + n.W
		}
		if n.Y+n.H > maxY {
			maxY = n.Y + n.H
		}
	}

	w := maxX - minX
	h := maxY - minY

	if w <= 0 || h <= 0 {
		return nil
	}

	mask := make([][]bool, h)
	for y := 0; y < h; y++ {
		mask[y] = make([]bool, w)
	}

	for _, n := range nodes {
		if n == nil {
			continue
		}

		for y := n.Y; y < n.Y+n.H; y++ {
			for x := n.X; x < n.X+n.W; x++ {
				mask[y-minY][x-minX] = true
			}
		}
	}

	res := make([]*DrawNode, 0)

	for {
		startX, startY := FindFirstTrue(mask, w, h)
		if startX == -1 {
			break
		}

		rectW := FindMaxWidth(mask, startX, startY, w)
		rectH := FindMaxHeight(mask, startX, startY, rectW, h)

		ClearMaskRect(mask, startX, startY, rectW, rectH)

		res = append(res, &DrawNode{
			X:    minX + startX,
			Y:    minY + startY,
			W:    rectW,
			H:    rectH,
			P:    color.Copy(),
			Rank: rank,
		})
	}

	return res
}

func FindFirstTrue(mask [][]bool, w, h int) (int, int) {
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mask[y][x] {
				return x, y
			}
		}
	}

	return -1, -1
}

func FindMaxWidth(mask [][]bool, startX, startY, w int) int {
	rectW := 0

	for x := startX; x < w; x++ {
		if !mask[startY][x] {
			break
		}
		rectW++
	}

	return rectW
}

func FindMaxHeight(mask [][]bool, startX, startY, rectW, h int) int {
	rectH := 1

	for y := startY + 1; y < h; y++ {
		ok := true

		for x := startX; x < startX+rectW; x++ {
			if !mask[y][x] {
				ok = false
				break
			}
		}

		if !ok {
			break
		}

		rectH++
	}

	return rectH
}

func ClearMaskRect(mask [][]bool, x, y, w, h int) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			mask[yy][xx] = false
		}
	}
}

func FlattenRanks(r map[int][]*DrawNode) []*DrawNode {
	ranks := make([]int, 0, len(r))

	for rank := range r {
		ranks = append(ranks, rank)
	}

	sort.Ints(ranks)

	res := make([]*DrawNode, 0)

	for _, rank := range ranks {
		nodes := r[rank]

		sort.SliceStable(nodes, func(i, j int) bool {
			a := nodes[i]
			b := nodes[j]

			if !a.P.IsEqual(b.P) {
				return a.P.Order(b.P)
			}

			if a.Y != b.Y {
				return a.Y < b.Y
			}

			if a.X != b.X {
				return a.X < b.X
			}

			if a.H != b.H {
				return a.H < b.H
			}

			return a.W < b.W
		})

		res = append(res, nodes...)
	}

	return res
}

func BuildDrawParts(root *quadtree.Node) [][]*DrawNode {
	nodeParts := quadtree.SpiltTree(root)

	res := make([][]*DrawNode, 0, len(nodeParts))

	for _, part := range nodeParts {
		ranks := NodesToDrawNodes(part)
		OptimizeRankMap(ranks)
		drawNodes := FlattenRanks(ranks)

		if len(drawNodes) > 0 {
			res = append(res, drawNodes)
		}
	}

	return res
}
