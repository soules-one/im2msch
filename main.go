package main

import (
	"fmt"
	"im2mlog/common"
	"im2mlog/drawtree"
	imageloader "im2mlog/image_loader"
	programbuilder "im2mlog/program_builder"
	"im2mlog/quadtree"
	"im2mlog/schematic"
	_ "image/jpeg"
	"os"
	"strconv"
	"strings"
)

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
			common.PaletteStep = v

		case strings.HasPrefix(arg, "--palette-size="):
			v, err := strconv.Atoi(strings.TrimPrefix(arg, "--palette-size="))
			if err != nil {
				panic(fmt.Errorf("invalid palette size: %s", arg))
			}
			if v < 0 {
				v = 0
			}
			common.PaletteSize = v

		default:
			v, err := strconv.Atoi(arg)
			if err != nil {
				panic(fmt.Errorf("unknown argument: %s", arg))
			}
			if v < 1 {
				v = 1
			}
			common.MinArea = v
		}
	}

	if w == 80 && h == 80 {
		common.DisplayBlockName = "logic-display"
	}
	if w == 176 && h == 176 {
		common.DisplayBlockName = "large-logic-display"
	}
	pp, err := imageloader.LoadAsRGBA(inp, w, h)
	if err != nil {
		panic(err)
	}

	if common.PaletteSize > 0 {
		palette := imageloader.BuildTopPalette(pp, common.PaletteSize)
		imageloader.ApplyPalette(pp, palette)
		fmt.Printf("Palette size: %d\n", len(palette))
	}

	root := &quadtree.Node{}
	root.BuildTree(pp)

	parts := drawtree.BuildDrawParts(root)
	programs := programbuilder.BuildPackedPrograms(parts, h, common.MaxNodes*2)
	programbuilder.ValidatePrograms(programs)

	if err := programbuilder.SavePrograms(programs, out); err != nil {
		panic(err)
	}

	if err := imageloader.SaveDrawPartsAsPNG(parts, "result.png", w, h); err != nil {
		panic(err)
	}

	base := programbuilder.OutputBase(out)

	mschOut := base + ".msch"
	schname := fmt.Sprintf("%v-image-renderer", base)
	if err := schematic.SaveSchematicMSCH(programs, mschOut, schname, w, h); err != nil {
		panic(err)
	}

	schemTextOut := base + ".schem.txt"
	if err := schematic.SaveSchematicBase64(programs, schemTextOut, schname, w, h); err != nil {
		panic(err)
	}

	fmt.Printf("Saved %d programs to %s\n", len(programs), out)
	fmt.Printf("Saved preview to result.png\n")
	fmt.Printf("Saved schematic to %s\n", mschOut)
	fmt.Printf("Saved schematic base64 to %s\n", schemTextOut)
	fmt.Printf("Saved %d draw parts\n", len(parts))
	fmt.Printf(
		"MinArea=%d, MaxNodes=%d, MaxDrawOp=%d, DisplayTilePixels=%d\n",
		common.MinArea,
		common.MaxNodes,
		common.MaxDrawOp,
		common.DisplayTilePixels,
	)

	if interactive {
		if err := programbuilder.InteractiveCopyPrograms(programs); err != nil {
			panic(err)
		}
	}
}
