package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
)

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

type Node struct {
	X, Y     int
	W, H     int
	P        Pixel
	IsLeaf   bool
	Parent   *Node
	Children [4]*Node
	Rank     int
	Size     int
}

type DrawNode struct {
	X, Y int
	W, H int
	P    Pixel
	Rank int
}

type ColorCount struct {
	P Pixel
	C int
}

type ProgramChunk struct {
	Index int
	Lines []string
}

var MinArea = 1
var MaxNodes = 450
var MaxDrawOp = 5
var PaletteSize = 0
var PaletteStep = 1

var ProgramOverheadLines = 6

var DisplayTilePixels = 32
var DisplayBlockName = "tile-logic-display"
var ProcessorBlockName = "micro-processor"

func ColorDistanceSq(a, b Pixel) int {
	dr := int(a[0]) - int(b[0])
	dg := int(a[1]) - int(b[1])
	db := int(a[2]) - int(b[2])

	return dr*dr + dg*dg + db*db
}

func BuildTopPalette(img [][]Pixel, maxColors int) []Pixel {
	if maxColors <= 0 {
		return nil
	}

	counts := make(map[Pixel]int)

	for y := range img {
		for x := range img[y] {
			counts[img[y][x]]++
		}
	}

	items := make([]ColorCount, 0, len(counts))
	for p, c := range counts {
		items = append(items, ColorCount{
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

	palette := make([]Pixel, 0, maxColors)
	for i := 0; i < maxColors; i++ {
		palette = append(palette, items[i].P)
	}

	return palette
}

func NearestColor(p Pixel, palette []Pixel) Pixel {
	if len(palette) == 0 {
		return p
	}

	best := palette[0]
	bestDist := ColorDistanceSq(p, best)

	for i := 1; i < len(palette); i++ {
		d := ColorDistanceSq(p, palette[i])
		if d < bestDist {
			bestDist = d
			best = palette[i]
		}
	}

	return best
}

func ApplyPalette(img [][]Pixel, palette []Pixel) {
	if len(palette) == 0 {
		return
	}

	cache := make(map[Pixel]Pixel)

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

func (n *Node) DominantColor(img [][]Pixel) {
	d := make(map[Pixel]int)

	for j := range n.H {
		for i := range n.W {
			p := img[n.Y+j][n.X+i]
			d[p]++
		}
	}

	mp := Pixel{}
	mx := 0

	for k, v := range d {
		if v > mx {
			mx = v
			mp = k
		}
	}

	n.P = mp
}

func (n *Node) BuildTree(img [][]Pixel) {
	n.Size = 1

	if n.Parent == nil {
		n.Children = [4]*Node{}
		n.H = len(img)
		n.W = len(img[0])
	}

	n.DominantColor(img)

	if n.W <= 1 || n.H <= 1 || n.W*n.H < MinArea {
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

	if root.Size < MaxNodes || root.IsLeaf {
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
		if child.Size > MaxNodes {
			if len(sub) > 0 {
				res = append(res, append([]*Node{}, sub...))
				sub = sub[:0]
				c = 0
			}

			res = append(res, SpiltTree(child)...)
			continue
		}

		if c+child.Size <= MaxNodes {
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

func MakeDrawNodes(n *Node, r map[int][]*DrawNode, force ...bool) {
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

func NodesToDrawNodes(part []*Node) map[int][]*DrawNode {
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
	byColor := make(map[Pixel][]*DrawNode)

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

func BuildDrawParts(root *Node) [][]*DrawNode {
	nodeParts := SpiltTree(root)

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

func DrawPartBodyLines(part []*DrawNode, h int) []string {
	lines := make([]string, 0)

	var current Pixel
	hasColor := false
	drawOpsSinceFlush := 0

	for _, n := range part {
		if n == nil {
			continue
		}

		if !hasColor || !current.IsEqual(n.P) {
			lines = append(lines, fmt.Sprintf(
				"draw color %d %d %d %d 0 0",
				n.P[0], n.P[1], n.P[2], n.P[3],
			))

			current = n.P
			hasColor = true
			drawOpsSinceFlush++
		}

		lines = append(lines, fmt.Sprintf(
			"draw rect %d %d %d %d 0 0",
			n.X, h-n.Y-n.H, n.W, n.H,
		))

		drawOpsSinceFlush++

		if drawOpsSinceFlush >= MaxDrawOp {
			lines = append(lines, "drawflush display1")
			drawOpsSinceFlush = 0
			hasColor = false
		}
	}

	return lines
}

func DrawPartsToChunks(parts [][]*DrawNode, h int) []ProgramChunk {
	chunks := make([]ProgramChunk, 0, len(parts))

	for i, part := range parts {
		if len(part) == 0 {
			continue
		}

		lines := DrawPartBodyLines(part, h)
		if len(lines) == 0 {
			continue
		}

		chunks = append(chunks, ProgramChunk{
			Index: i,
			Lines: lines,
		})
	}

	return chunks
}

func PackChunksByLines(chunks []ProgramChunk, maxLines int) [][]ProgramChunk {
	sorted := make([]ProgramChunk, len(chunks))
	copy(sorted, chunks)

	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].Lines) > len(sorted[j].Lines)
	})

	type bucket struct {
		Chunks    []ProgramChunk
		BodyLines int
	}

	buckets := make([]bucket, 0)

	for _, chunk := range sorted {
		chunkLen := len(chunk.Lines)
		placed := false

		for i := range buckets {
			if buckets[i].BodyLines+chunkLen <= maxLines {
				buckets[i].Chunks = append(buckets[i].Chunks, chunk)
				buckets[i].BodyLines += chunkLen
				placed = true
				break
			}
		}

		if !placed {
			buckets = append(buckets, bucket{
				Chunks:    []ProgramChunk{chunk},
				BodyLines: chunkLen,
			})
		}
	}

	res := make([][]ProgramChunk, 0, len(buckets))
	for _, b := range buckets {
		res = append(res, b.Chunks)
	}

	return res
}

func BuildProgramLines(programIndex int, chunks []ProgramChunk) []string {
	bodyLen := 0
	for _, chunk := range chunks {
		bodyLen += len(chunk.Lines)
	}

	lines := make([]string, 0, bodyLen+6)

	lines = append(lines, "sensor flag switch1 @enabled")
	lines = append(lines, "jump 0 strictEqual display1 null")

	for _, chunk := range chunks {
		lines = append(lines, chunk.Lines...)
	}

	lines = append(lines, "drawflush display1")
	lines = append(lines, "sensor result switch1 @enabled")

	// 0            sensor flag
	// 1            jump display1 null
	// 2..bodyLen+1 body
	// bodyLen+2    drawflush
	// bodyLen+3    sensor result
	// bodyLen+4    jump wait
	// bodyLen+5    end
	waitLine := bodyLen + 3

	lines = append(lines, fmt.Sprintf("jump %d strictEqual result flag", waitLine))
	lines = append(lines, "end")

	return lines
}

func BuildPackedPrograms(parts [][]*DrawNode, h int, maxLines int) [][]string {
	chunks := DrawPartsToChunks(parts, h)
	packed := PackChunksByLines(chunks, maxLines)

	programs := make([][]string, 0, len(packed))

	for i, group := range packed {
		program := BuildProgramLines(i, group)
		programs = append(programs, program)
	}

	return programs
}

func SavePrograms(programs [][]string, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	for i, program := range programs {
		fmt.Fprintf(file, "# Program %d\n", i)
		fmt.Fprintln(file, "# Copy only instructions below")
		fmt.Fprintln(file)

		for _, line := range program {
			fmt.Fprintln(file, line)
		}

		fmt.Fprintln(file)
		fmt.Fprintln(file)
	}

	return nil
}

func ValidatePrograms(programs [][]string) {
	for pi, program := range programs {
		for li, line := range program {
			line = strings.TrimSpace(line)

			if line == "" {
				fmt.Printf("WARNING: program %d has empty line at %d\n", pi, li)
			}

			if strings.HasPrefix(line, "#") {
				fmt.Printf("WARNING: program %d has comment line at %d: %q\n", pi, li, line)
			}
		}
	}
}

func ProgramToString(program []string) string {
	var buf bytes.Buffer

	for _, line := range program {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	return buf.String()
}

func InteractiveCopyPrograms(programs [][]string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Interactive copy mode enabled.\n")
	fmt.Printf("Programs: %d\n", len(programs))
	fmt.Printf("Press Enter to copy next program, type number to copy exact program, or q to quit.\n\n")

	current := 0

	for {
		if current >= len(programs) {
			fmt.Println("All programs copied.")
			return nil
		}

		fmt.Printf("Next program: %d/%d. Press Enter, number, or q: ", current, len(programs)-1)

		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		line = strings.TrimSpace(line)

		if line == "q" || line == "quit" || line == "exit" {
			fmt.Println("Stopped.")
			return nil
		}

		programIndex := current

		if line != "" {
			v, err := strconv.Atoi(line)
			if err != nil {
				fmt.Println("Invalid input. Use Enter, number, or q.")
				continue
			}

			if v < 0 || v >= len(programs) {
				fmt.Printf("Program index out of range: 0..%d\n", len(programs)-1)
				continue
			}

			programIndex = v
		}

		text := ProgramToString(programs[programIndex])

		if err := CopyToClipboard(text); err != nil {
			return err
		}

		fmt.Printf(
			"Copied Program %d: %d chars, %d lines\n",
			programIndex,
			len(text),
			len(programs[programIndex]),
		)

		if line == "" {
			current++
		} else {
			current = programIndex + 1
		}
	}
}

func CopyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")

	case "darwin":
		cmd = exec.Command("pbcopy")

	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("clipboard tool not found: install wl-clipboard, xclip, or xsel")
		}

	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	_, err = in.Write([]byte(text))

	if closeErr := in.Close(); closeErr != nil && err == nil {
		err = closeErr
	}

	if waitErr := cmd.Wait(); waitErr != nil && err == nil {
		err = waitErr
	}

	return err
}

func LoadAsRGBA(filePath string, w, h int) ([][]Pixel, error) {
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
	res := make([][]Pixel, height)

	for y := 0; y < height; y++ {
		res[y] = make([]Pixel, width)
		for x := 0; x < width; x++ {
			idx := y*rgba.Stride + x*4
			res[y][x] = Pixel{
				rgba.Pix[idx],
				rgba.Pix[idx+1],
				rgba.Pix[idx+2],
				rgba.Pix[idx+3],
			}

			if PaletteStep > 1 {
				res[y][x] = QuantizePixel(res[y][x], PaletteStep)
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

func QuantizePixel(p Pixel, step int) Pixel {
	if step <= 1 {
		return p
	}

	return Pixel{
		QuantizeChannel(p[0], step),
		QuantizeChannel(p[1], step),
		QuantizeChannel(p[2], step),
		p[3],
	}
}

func SaveDrawPartsAsPNG(parts [][]*DrawNode, filepath string, w, h int) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	drawNode := func(n *DrawNode) {
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

// ===============================
// Mindustry schematic writer
// ===============================

type BinWriter struct {
	bytes.Buffer
}

func (w *BinWriter) U8(v byte) {
	w.WriteByte(v)
}

func (w *BinWriter) U16(v uint16) {
	_ = binary.Write(&w.Buffer, binary.BigEndian, v)
}

func (w *BinWriter) I16(v int16) {
	_ = binary.Write(&w.Buffer, binary.BigEndian, v)
}

func (w *BinWriter) I32(v int32) {
	_ = binary.Write(&w.Buffer, binary.BigEndian, v)
}

func (w *BinWriter) Str(s string) {
	w.U16(uint16(len(s)))
	w.WriteString(s)
}

type SchematicLink struct {
	Name string
	DX   int16
	DY   int16
}

func ZlibCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func BuildProcessorConfig(program string, links []SchematicLink) ([]byte, error) {
	var raw BinWriter

	raw.U8(1)

	programBytes := []byte(program)
	raw.I32(int32(len(programBytes)))
	raw.Write(programBytes)

	raw.I32(int32(len(links)))

	for _, link := range links {
		raw.UTF(link.Name)
		raw.I16(link.DX)
		raw.I16(link.DY)
	}

	return ZlibCompress(raw.Bytes())
}

func WriteObjectNil(w *BinWriter) {
	w.U8(0)
}

func WriteObjectString(w *BinWriter, value string) {
	w.U8(4)
	w.U8(1)
	w.Str(value)
}

func WriteObjectBool(w *BinWriter, value bool) {
	w.U8(10)

	if value {
		w.U8(1)
	} else {
		w.U8(0)
	}
}

func WriteObjectProcessor(w *BinWriter, compressed []byte) {
	w.U8(14)
	w.I32(int32(len(compressed)))
	w.Write(compressed)
}

func WriteTile(w *BinWriter, blockIndex byte, x, y int, writeConfig func()) {
	w.U8(blockIndex)

	pos := int32((x&0xffff)<<16 | (y & 0xffff))
	w.I32(pos)

	writeConfig()

	w.U8(0)
}

func CeilDiv(a, b int) int {
	if b <= 0 {
		return 0
	}
	return (a + b - 1) / b
}

func (w *BinWriter) UTF(s string) {
	b := []byte(s)
	if len(b) > 65535 {
		panic(fmt.Errorf("UTF string too long: %d bytes", len(b)))
	}
	w.U16(uint16(len(b)))
	w.Write(b)
}

func OutputBase(path string) string {
	i := strings.LastIndex(path, ".")
	if i <= 0 {
		return path
	}
	return path[:i]
}

func BuildSchematicMSCH(programs [][]string, name string, imageW, imageH int) ([]byte, error) {
	const (
		blockSwitch    = byte(0)
		blockDisplay   = byte(1)
		blockProcessor = byte(2)
		blockMessage   = byte(3)
	)

	displayTilesX := CeilDiv(imageW, DisplayTilePixels)
	displayTilesY := CeilDiv(imageH, DisplayTilePixels)

	if displayTilesX < 1 {
		displayTilesX = 1
	}
	if displayTilesY < 1 {
		displayTilesY = 1
	}

	// Процессоры строго под дисплеем:
	// ширина блока процессоров равна ширине дисплея.
	procCols := displayTilesX
	if procCols < 1 {
		procCols = 1
	}

	procRows := 0
	if len(programs) > 0 {
		procRows = CeilDiv(len(programs), procCols)
	}

	// Минимум 2 клетки по ширине, чтобы было куда поставить
	// switch и message рядом, даже если дисплей 1xN.
	layoutWidth := displayTilesX
	if layoutWidth < 2 {
		layoutWidth = 2
	}

	// Дисплей по центру layout-а.
	displayX := (layoutWidth - displayTilesX) / 2

	// Процессоры начинаются строго под дисплеем.
	procStartX := displayX

	// 0              -> switch/message
	// 1..procRows    -> процессоры
	controlY := 0
	procStartY := 1
	displayY := procStartY + procRows

	anchorX := displayX + (displayTilesX-1)/2
	//anchorY := displayY

	switchX := anchorX

	messageX := switchX + 1
	if messageX >= layoutWidth {
		messageX = switchX - 1
	}
	if messageX < 0 || messageX == switchX {
		messageX = switchX
	}

	width := layoutWidth
	height := displayY + displayTilesY

	var payload BinWriter

	payload.U16(uint16(width))
	payload.U16(uint16(height))

	payload.U8(4)

	payload.UTF("name")
	payload.UTF(name)

	payload.UTF("contentMap")
	payload.UTF("{}")

	payload.UTF("description")
	payload.UTF("Generated image renderer")

	payload.UTF("labels")
	payload.UTF("[]")

	payload.U8(4)
	payload.UTF("switch")
	payload.UTF(DisplayBlockName)
	payload.UTF(ProcessorBlockName)
	payload.UTF("message")

	displayTileCount := displayTilesX * displayTilesY
	tileCount := 1 + 1 + displayTileCount + len(programs)

	payload.I32(int32(tileCount))

	// 1. display grid FIRST
	for dx := 0; dx < displayTilesX; dx++ {
		for dy := 0; dy < displayTilesY; dy++ {
			x := displayX + dx
			y := displayY + dy

			WriteTile(&payload, blockDisplay, x, y, func() {
				WriteObjectNil(&payload)
			})
		}
	}

	// 2. switch
	WriteTile(&payload, blockSwitch, switchX, controlY, func() {
		WriteObjectBool(&payload, true)
	})

	// 3. message
	WriteTile(&payload, blockMessage, messageX, controlY, func() {
		WriteObjectString(&payload, "Press button to rebuild image. Built using github.com/soules-one/im2msch")
	})
	WriteTile(&payload, blockMessage, messageX, controlY, func() {
		WriteObjectNil(&payload)
	})

	// 4. processors LAST
	for i, program := range programs {
		col := i % procCols
		row := i / procCols

		px := procStartX + col
		py := procStartY + row

		code := strings.Join(program, "\n")
		if !strings.HasSuffix(code, "\n") {
			code += "\n"
		}

		links := []SchematicLink{
			{
				Name: "switch1",
				DX:   int16(switchX - px),
				DY:   int16(controlY - py),
			},
			{
				Name: "display1",
				DX:   int16((displayX + col) - px),
				DY:   int16(displayY - py),
			},
		}

		config, err := BuildProcessorConfig(code, links)
		if err != nil {
			return nil, err
		}

		WriteTile(&payload, blockProcessor, px, py, func() {
			WriteObjectProcessor(&payload, config)
		})
	}

	compressedPayload, err := ZlibCompress(payload.Bytes())
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString("msch")
	out.WriteByte(1)
	out.Write(compressedPayload)

	return out.Bytes(), nil
}

func SaveSchematicMSCH(programs [][]string, filepath string, name string, imageW, imageH int) error {
	data, err := BuildSchematicMSCH(programs, name, imageW, imageH)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath, data, 0644)
}

func SaveSchematicBase64(programs [][]string, filepath string, name string, imageW, imageH int) error {
	data, err := BuildSchematicMSCH(programs, name, imageW, imageH)
	if err != nil {
		return err
	}

	text := base64.StdEncoding.EncodeToString(data)
	return os.WriteFile(filepath, []byte(text), 0644)
}

func CleanProgramLines(lines []string) []string {
	res := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "#") {
			continue
		}

		res = append(res, line)
	}

	return res
}

func main() {
	if len(os.Args) < 5 {
		fmt.Fprintf(
			os.Stderr,
			"usage: %s input output width height [minArea] [-i|--interactive] [--palette-step=N] [--palette-size=N]\n",
			os.Args[0],
		)
		os.Exit(1)
	}

	inp := os.Args[1]
	out := os.Args[2]

	w, err := strconv.Atoi(os.Args[3])
	if err != nil {
		panic(err)
	}

	h, err := strconv.Atoi(os.Args[4])
	if err != nil {
		panic(err)
	}

	interactive := false

	for _, arg := range os.Args[5:] {
		switch {
		case arg == "-i" || arg == "--interactive" || arg == "--console":
			interactive = true

		case strings.HasPrefix(arg, "--palette-step="):
			v, err := strconv.Atoi(strings.TrimPrefix(arg, "--palette-step="))
			if err != nil {
				panic(fmt.Errorf("invalid palette step: %s", arg))
			}
			if v < 1 {
				v = 1
			}
			PaletteStep = v

		case strings.HasPrefix(arg, "--palette-size="):
			v, err := strconv.Atoi(strings.TrimPrefix(arg, "--palette-size="))
			if err != nil {
				panic(fmt.Errorf("invalid palette size: %s", arg))
			}
			if v < 0 {
				v = 0
			}
			PaletteSize = v

		default:
			v, err := strconv.Atoi(arg)
			if err != nil {
				panic(fmt.Errorf("unknown argument: %s", arg))
			}
			if v < 1 {
				v = 1
			}
			MinArea = v
		}
	}

	pp, err := LoadAsRGBA(inp, w, h)
	if err != nil {
		panic(err)
	}

	if PaletteSize > 0 {
		palette := BuildTopPalette(pp, PaletteSize)
		ApplyPalette(pp, palette)
		fmt.Printf("Palette size: %d\n", len(palette))
	}

	root := &Node{}
	root.BuildTree(pp)

	parts := BuildDrawParts(root)
	programs := BuildPackedPrograms(parts, h, MaxNodes*2)
	ValidatePrograms(programs)

	if err := SavePrograms(programs, out); err != nil {
		panic(err)
	}

	if err := SaveDrawPartsAsPNG(parts, "result.png", w, h); err != nil {
		panic(err)
	}

	base := OutputBase(out)

	mschOut := base + ".msch"
	schname := fmt.Sprintf("%v-image-renderer", base)
	if err := SaveSchematicMSCH(programs, mschOut, schname, w, h); err != nil {
		panic(err)
	}

	schemTextOut := base + ".schem.txt"
	if err := SaveSchematicBase64(programs, schemTextOut, schname, w, h); err != nil {
		panic(err)
	}

	fmt.Printf("Saved %d programs to %s\n", len(programs), out)
	fmt.Printf("Saved preview to result.png\n")
	fmt.Printf("Saved schematic to %s\n", mschOut)
	fmt.Printf("Saved schematic base64 to %s\n", schemTextOut)
	fmt.Printf("Saved %d draw parts\n", len(parts))
	fmt.Printf(
		"MinArea=%d, MaxNodes=%d, MaxDrawOp=%d, DisplayTilePixels=%d\n",
		MinArea,
		MaxNodes,
		MaxDrawOp,
		DisplayTilePixels,
	)

	if interactive {
		if err := InteractiveCopyPrograms(programs); err != nil {
			panic(err)
		}
	}
}
