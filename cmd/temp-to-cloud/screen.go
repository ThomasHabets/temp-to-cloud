package main

import (
	"flag"
	"image"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/inconsolata"
	"golang.org/x/image/math/fixed"

	log "github.com/sirupsen/logrus"

	"periph.io/x/periph/conn/i2c"
	"periph.io/x/periph/conn/i2c/i2creg"
	"periph.io/x/periph/conn/physic"
	"periph.io/x/periph/devices/ssd1306"
	"periph.io/x/periph/devices/ssd1306/image1bit"
	"periph.io/x/periph/host"
)

var (
	h      = flag.Int("screen_height", 32, "Display height")
	w      = flag.Int("screen_width", 128, "Display width")
	hz     physic.Frequency
	screen *ssd1306.Dev
)

func init() {
	flag.Var(&hz, "hz", "I²C bus/SPI port speed")
}

// TODO: really this should take an i2cbus and return an object. And also set up the drawer.
func initScreen() (func(), error) {
	if false {
		// Assume this is already done.
		if _, err := host.Init(); err != nil {
			return func() {}, err
		}
	}

	opts := ssd1306.Opts{W: *w, H: *h, Rotated: false, Sequential: true, SwapTopBottom: false}

	var err error

	i2cConn, err := i2creg.Open(*i2cID)
	if err != nil {
		return func(){},err
	}
	iclose := func(){i2cConn.Close()}

	if hz != 0 {
		if err := i2cConn.SetSpeed(hz); err != nil {
			return iclose, err
		}
	}
	if p, ok := i2cConn.(i2c.Pins); ok {
		log.Infof("Using pins SCL: %s  SDA: %s", p.SCL(), p.SDA())
	}
	screen, err = ssd1306.NewI2C(i2cConn, &opts)
	if err != nil {
		return iclose, err
	}

	return func() {
		screen.Halt()
		iclose()
	}, nil
}

func updateScreen(st string) error {
	src := image1bit.NewVerticalLSB(screen.Bounds())
	// src.SetBit(x,y,image1bit.On)

	// Prepare to draw.
	face := inconsolata.Regular8x16
	if true {
		face = basicfont.Face7x13
	}

	drawer := &font.Drawer{
		Src:  &image.Uniform{C: image1bit.On},
		Dst:  src,
		Face: inconsolata.Regular8x16,
	}

	// Set up lines.
	lines := []string{
		time.Now().Format("02 Jan, 15:04:05"),
		st,
	}

	// Draw lines.
	lastrow := 0
	for n, line := range lines {
		b, a := font.BoundString(face, line)
		y := lastrow - b.Min.Y.Round()
		x := 0
		if n > 0 {
			y++
		}
		drawer.Dot = fixed.Point26_6{fixed.Int26_6(x * 64), fixed.Int26_6(y * 64)}
		drawer.DrawString(line)

		log.Printf("Line %d on %d (%q): %+v, %+v", n, y, line, b, a)
		lastrow += (b.Max.Y - b.Min.Y).Round()
	}

	return screen.Draw(src.Bounds(), src, image.Point{})
}
