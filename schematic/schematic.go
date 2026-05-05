package schematic

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"im2mlog/common"
	"os"
	"strings"
)

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

func BuildSchematicMSCH(programs [][]string, name string, imageW, imageH int) ([]byte, error) {
	const (
		blockSwitch    = byte(0)
		blockDisplay   = byte(1)
		blockProcessor = byte(2)
		blockMessage   = byte(3)
	)

	displayTilesX := CeilDiv(imageW, common.DisplayTilePixels)
	displayTilesY := CeilDiv(imageH, common.DisplayTilePixels)

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
	payload.UTF(common.DisplayBlockName)
	payload.UTF(common.ProcessorBlockName)
	payload.UTF("message")

	displayTileCount := displayTilesX * displayTilesY
	tileCount := 1 + 1 + displayTileCount + len(programs)

	// 1. display grid FIRST
	if (imageH == 80 && imageW == 80) || (imageH == 176 && imageW == 176) {
		tileCount := 1 + 1 + 1 + len(programs)
		payload.I32(int32(tileCount))
		if imageH == 80 {
			WriteTile(&payload, blockDisplay, displayX+1, displayY+1, func() {
				WriteObjectNil(&payload)
			})
		} else {
			WriteTile(&payload, blockDisplay, displayX+2, displayY+2, func() {
				WriteObjectNil(&payload)
			})
		}
	} else {
		payload.I32(int32(tileCount))
		for dx := 0; dx < displayTilesX; dx++ {
			for dy := 0; dy < displayTilesY; dy++ {
				x := displayX + dx
				y := displayY + dy

				WriteTile(&payload, blockDisplay, x, y, func() {
					WriteObjectNil(&payload)
				})
			}
		}
	}

	// 2. switch
	WriteTile(&payload, blockSwitch, switchX, controlY, func() {
		WriteObjectBool(&payload, true)
	})

	// 3. message
	WriteTile(&payload, blockMessage, messageX, controlY, func() {
		WriteObjectString(&payload, "Press button to rebuild image.\nBuilt using github.com/soules-one/im2msch")
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
