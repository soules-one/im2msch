# im2msch
Program that creates Mindustry schematics for you images
> Vast parts of this projects were vibe-coded (even thought design was made by me) and are in wait for bugfix, refactor and proper documentation.

## Usage
Either clone and compile main.go on your PC or download version for your OS in release section.
As of now, png and jpeg are supported.

There is now validation checks for sizes of schematics so be aware it might not be correct

`im2msch filename schematic_name width height [area] --palette-step [value] --palette-size [value]
`

##### flags and area are optional and used for schematic optimisation

area is value that defines smallest area of a drawn element on screen (by pixels)

palette-step is a value that defines how much is deference between colors RGB value. If set to i, each color would be rounded to closesth i'th color/

palette-size is value that defines how many different colors will be drawn. It selects top palette-size colors and others are rounded to closest from palette.

## How it works

Image are being compressed into quad-tree (which consist of hierarchy of rectangles).

Since each subtree represent independent part of image, so we split tree into list of subtrees of set size (so number of draw commands for at least 1 part of image would meet in-game processor limit). 

After that, since each subtree is independent part of image, we could flatten tree and sort it by each tree node height (where each rank actually represents drawn image layer). For each height value we can merge some rectangles to make programs more compact and fast.

After all optimisation we build draw programs for each flattened subtree and then pack them into processors (if there is enough space, processor can actually draw more than one subtree).

Then we save schematic of tiled display with programmed processors.
Schematic also have switch that can start image regeneration.

## Known bugs/issues
- display border calculation is not implemented

