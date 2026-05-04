package programbuilder

import (
	"bufio"
	"bytes"
	"fmt"
	"im2mlog/common"
	"im2mlog/drawtree"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type ProgramChunk struct {
	Index int
	Lines []string
}

func DrawPartBodyLines(part []*drawtree.DrawNode, h int) []string {
	lines := make([]string, 0)

	var current common.Pixel
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

		if drawOpsSinceFlush >= common.MaxDrawOp {
			lines = append(lines, "drawflush display1")
			drawOpsSinceFlush = 0
			hasColor = false
		}
	}

	return lines
}

func DrawPartsToChunks(parts [][]*drawtree.DrawNode, h int) []ProgramChunk {
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

func BuildPackedPrograms(parts [][]*drawtree.DrawNode, h int, maxLines int) [][]string {
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

func OutputBase(path string) string {
	i := strings.LastIndex(path, ".")
	if i <= 0 {
		return path
	}
	return path[:i]
}
