package quadtree

import (
	"im2mlog/common"
	"sort"
)

type Node struct {
	X, Y     int
	W, H     int
	P        common.Pixel
	IsLeaf   bool
	Parent   *Node
	Children [4]*Node
	Rank     int
	Size     int
}

func (n *Node) DominantColor(img [][]common.Pixel) {
	d := make(map[common.Pixel]int)

	for j := range n.H {
		for i := range n.W {
			p := img[n.Y+j][n.X+i]
			d[p]++
		}
	}

	mp := common.Pixel{}
	mx := 0

	for k, v := range d {
		if v > mx {
			mx = v
			mp = k
		}
	}

	n.P = mp
}

func (n *Node) BuildTree(img [][]common.Pixel) {
	n.Size = 1

	if n.Parent == nil {
		n.Children = [4]*Node{}
		n.H = len(img)
		n.W = len(img[0])
	}

	n.DominantColor(img)

	if n.W <= 1 || n.H <= 1 || n.W*n.H < common.MinArea {
		n.IsLeaf = true
		return
	}

	n.IsLeaf = false

	x1, x2 := n.X, n.X+n.W/2
	y1, y2 := n.Y, n.Y+n.H/2
	nw, nh := n.W/2, n.H/2

	n.Children[0] = &Node{x1, y1, nw, nh, n.P.Copy(), true, n, [4]*Node{}, n.Rank + 1, 0}
	n.Children[1] = &Node{x1, y2, nw, n.H - nh, n.P.Copy(), true, n, [4]*Node{}, n.Rank + 1, 0}
	n.Children[2] = &Node{x2, y1, n.W - nw, nh, n.P.Copy(), true, n, [4]*Node{}, n.Rank + 1, 0}
	n.Children[3] = &Node{x2, y2, n.W - nw, n.H - nh, n.P.Copy(), true, n, [4]*Node{}, n.Rank + 1, 0}

	same := true

	for i := 0; i < 4; i++ {
		n.Children[i].BuildTree(img)
		same = same && n.Children[i].IsLeaf && n.Children[i].P.IsEqual(n.P)
		n.Size += n.Children[i].Size
	}

	if same {
		n.Children = [4]*Node{}
		n.IsLeaf = true
		n.Size = 1
	}
}

func SpiltTree(root *Node) [][]*Node {
	res := make([][]*Node, 0)

	if root.Size < common.MaxNodes || root.IsLeaf {
		res = append(res, []*Node{root})
		return res
	}

	children := make([]*Node, 0, 4)
	for i := 0; i < 4; i++ {
		if root.Children[i] != nil {
			children = append(children, root.Children[i])
		}
	}

	sort.Slice(children, func(i, j int) bool {
		return children[i].Size < children[j].Size
	})

	c := 0
	sub := make([]*Node, 0)

	for _, child := range children {
		if child.Size > common.MaxNodes {
			if len(sub) > 0 {
				res = append(res, append([]*Node{}, sub...))
				sub = sub[:0]
				c = 0
			}

			res = append(res, SpiltTree(child)...)
			continue
		}

		if c+child.Size <= common.MaxNodes {
			sub = append(sub, child)
			c += child.Size
		} else {
			if len(sub) > 0 {
				res = append(res, append([]*Node{}, sub...))
			}

			sub = sub[:0]
			sub = append(sub, child)
			c = child.Size
		}
	}

	if len(sub) > 0 {
		res = append(res, append([]*Node{}, sub...))
	}

	return res
}
