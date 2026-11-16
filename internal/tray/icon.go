package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

var (
	cGreen = color.NRGBA{110, 200, 140, 255}
	cAmber = color.NRGBA{235, 190, 90, 255}
	cBlue  = color.NRGBA{120, 170, 235, 255}
	cRed   = color.NRGBA{228, 110, 100, 255}
	cOff   = color.NRGBA{130, 138, 150, 255}
	cAcc   = color.NRGBA{217, 119, 87, 255}
	cTrack = color.NRGBA{255, 255, 255, 90}
)

func statusColor(s string) color.NRGBA {
	switch s {
	case "working":
		return cGreen
	case "waiting":
		return cAmber
	case "idle":
		return cBlue
	case "stale":
		return cRed
	}
	return cOff
}

// renderIcon dibuja el icono (32x32 PNG): punto = estado, anillo = % de la
// ventana de 5h (pct5 < 0 => sin datos), numero = sesiones si hay mas de una.
func renderIcon(status string, pct5, active int) []byte {
	const S = 4 // supersampling
	const N = 32 * S
	big := image.NewNRGBA(image.Rect(0, 0, N, N))
	cx, cy := float64(N)/2, float64(N)/2
	rOut, rIn := 15.0*S, 11.0*S
	rDot := 7.0 * S

	arcColor := cAcc
	if pct5 >= 90 {
		arcColor = cRed
	} else if pct5 >= 70 {
		arcColor = cAmber
	}
	sweep := 0.0
	if pct5 >= 0 {
		sweep = math.Max(0.03, math.Min(1, float64(pct5)/100)) * 2 * math.Pi
	}
	dot := statusColor(status)
	rMid := (rOut + rIn) / 2
	capR := (rOut - rIn) / 2
	endA := -math.Pi/2 + sweep
	sx, sy := cx+rMid*math.Cos(-math.Pi/2), cy+rMid*math.Sin(-math.Pi/2)
	ex, ey := cx+rMid*math.Cos(endA), cy+rMid*math.Sin(endA)

	for y := 0; y < N; y++ {
		for x := 0; x < N; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			dx, dy := fx-cx, fy-cy
			d := math.Hypot(dx, dy)
			switch {
			case d <= rDot:
				big.SetNRGBA(x, y, dot)
			case d >= rIn && d <= rOut:
				a := math.Atan2(dy, dx) + math.Pi/2 // 0 arriba, horario
				if a < 0 {
					a += 2 * math.Pi
				}
				if sweep > 0 && a <= sweep {
					big.SetNRGBA(x, y, arcColor)
				} else {
					big.SetNRGBA(x, y, cTrack)
				}
			}
			// Extremos redondeados del arco.
			if sweep > 0 && sweep < 2*math.Pi {
				if math.Hypot(fx-sx, fy-sy) <= capR || math.Hypot(fx-ex, fy-ey) <= capR {
					big.SetNRGBA(x, y, arcColor)
				}
			}
		}
	}

	// Reducir 4x promediando (antialiasing).
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			var r, g, b, a float64
			for j := 0; j < S; j++ {
				for i := 0; i < S; i++ {
					c := big.NRGBAAt(x*S+i, y*S+j)
					fa := float64(c.A)
					r += float64(c.R) * fa
					g += float64(c.G) * fa
					b += float64(c.B) * fa
					a += fa
				}
			}
			if a > 0 {
				img.SetNRGBA(x, y, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(a / (S * S))})
			}
		}
	}

	if active > 1 {
		txt := strconv.Itoa(active)
		if active > 9 {
			txt = "+"
		}
		face := basicfont.Face7x13
		w := font.MeasureString(face, txt).Ceil()
		d := &font.Drawer{Dst: img, Src: image.NewUniform(color.NRGBA{20, 20, 24, 255}), Face: face,
			Dot: fixed.P(16-w/2, 16+4)}
		d.DrawString(txt)
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// pngToICO envuelve un PNG en un contenedor .ico (Windows Vista+ acepta PNG dentro).
func pngToICO(p []byte) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, struct{ Reserved, Type, Count uint16 }{0, 1, 1})
	_ = binary.Write(&b, binary.LittleEndian, struct {
		W, H, Colors, Reserved uint8
		Planes, BPP            uint16
		Size, Offset           uint32
	}{32, 32, 0, 0, 1, 32, uint32(len(p)), 22})
	b.Write(p)
	return b.Bytes()
}
