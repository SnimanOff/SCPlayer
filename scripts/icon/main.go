package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/akavel/rsrc/rsrc"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

func main() {
	syso := flag.Bool("syso", false, "also write rsrc_windows_amd64.syso for the Windows build")
	flag.Parse()
	svg := filepath.Join("packaging", "icon", "sc-player.svg")
	ico := filepath.Join("packaging", "windows", "sc-player.ico")

	big, err := render(svg, 256)
	if err != nil {
		log.Fatal(err)
	}
	write(filepath.Join("internal", "icon", "sc-player.png"), encodePNG(big))

	sizes := []int{16, 20, 24, 32, 40, 48, 64, 128, 256}
	var images [][]byte
	for _, size := range sizes {
		img, err := render(svg, size)
		if err != nil {
			log.Fatal(err)
		}
		if size == 256 {
			images = append(images, encodePNG(img))
		} else {
			images = append(images, encodeDIB(img))
		}
	}
	write(ico, icoFile(images, sizes))
	if *syso {
		if err := rsrc.Embed("rsrc_windows_amd64.syso", "amd64", "", ico); err != nil {
			log.Fatal(err)
		}
	}
}

func render(path string, size int) (*image.RGBA, error) {
	icon, err := oksvg.ReadIcon(path, oksvg.StrictErrorMode)
	if err != nil {
		return nil, err
	}
	icon.SetTarget(0, 0, float64(size), float64(size))
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	icon.Draw(rasterx.NewDasher(size, size, rasterx.NewScannerGV(size, size, img, img.Bounds())), 1)
	return img, nil
}

func encodePNG(img image.Image) []byte {
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		log.Fatal(err)
	}
	return out.Bytes()
}

func encodeDIB(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	maskRow := (w + 31) / 32 * 4
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, struct {
		Size                     uint32
		Width, Height            int32
		Planes, Bits             uint16
		Compression, ImageSize   uint32
		XPerMeter, YPerMeter     int32
		ColorsUsed, ColorsImport uint32
	}{40, int32(w), int32(2 * h), 1, 32, 0, uint32(w * h * 4), 0, 0, 0, 0})
	mask := make([]byte, maskRow*h)
	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			out.Write([]byte{c.B, c.G, c.R, c.A})
			if c.A == 0 {
				row := h - 1 - y
				mask[row*maskRow+x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	out.Write(mask)
	return out.Bytes()
}

func icoFile(frames [][]byte, sizes []int) []byte {
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(frames))})
	offset := 6 + 16*len(frames)
	for i, data := range frames {
		side := byte(sizes[i])
		_ = binary.Write(&out, binary.LittleEndian, struct {
			Width, Height, Colors, Reserved byte
			Planes, Bits                    uint16
			Size, Offset                    uint32
		}{side, side, 0, 0, 1, 32, uint32(len(data)), uint32(offset)})
		offset += len(data)
	}
	for _, data := range frames {
		out.Write(data)
	}
	return out.Bytes()
}

func write(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatal(err)
	}
}
