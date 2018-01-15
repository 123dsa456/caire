package caire

import (
	"image"
	"math"
	"os"
	"image/png"
//	"image/color"
)

// SeamCarver is an interface that Carver uses to implement the Resize function.
// It takes an image and the output as parameters and returns the resized image
// and the error, if exists.
type SeamCarver interface {
	Resize(*image.NRGBA, *image.NRGBA, string)(*os.File, error)
}

// Seam struct containing the pixel coordinate values.
type Seam struct {
	X int
	Y int
}

// NewCarver returns an initialized Carver structure.
func NewCarver(width, height, threshold, blur int, rw, rh, perc int) *Carver {
	return &Carver{
		width,
		height,
		make([]float64, width*height),
		threshold,
		blur,
		rw, rh,
		perc,
	}
}

// Get energy pixel value.
func (c *Carver) get(x, y int) float64 {
	px := x + y * c.Width
	return c.Points[px]
}

// Set energy pixel value.
func (c *Carver) set(x, y int, px float64) {
	idx := x + y * c.Width
	c.Points[idx] = px
}

// Compute the minimum energy level based on the following logic:
// 	- traverse the image from the second row to the last row
// 	  and compute the cumulative minimum energy M for all possible
//	  connected seams for each entry (i, j).
//
//	- the minimum energy level is calculated by summing up the current pixel value
// 	  with the minimum pixel value of the neighboring pixels from the previous row.
func (c *Carver) computeSeams(img *image.NRGBA) []float64 {
	var src *image.NRGBA
	bounds := img.Bounds()
	iw, ih := bounds.Dx(), bounds.Dy()
	sobel := SobelFilter(Grayscale(img), float64(c.SobelThreshold))

	if c.BlurRadius > 0 {
		src = Stackblur(sobel, uint32(iw), uint32(ih), uint32(c.BlurRadius))
	} else {
		src = sobel
	}
	for x := 0; x < c.Width; x++ {
		for y := 0; y < c.Height; y++ {
			r, _, _, a := src.At(x, y).RGBA()
			c.set(x, y, float64(r) / float64(a))
		}
	}

	// Compute the minimum energy level and set the resulting value into carver table.
	for x := 0; x < c.Width; x++ {
		for y := 1; y < c.Height; y++ {
			var left, middle, right float64
			left, right = math.MaxFloat64, math.MaxFloat64

			// Do not compute edge cases: pixels are far left.
			if x > 0 {
				left = c.get(x-1, y-1)
			}
			middle = c.get(x, y-1)
			// Do not compute edge cases: pixels are far right.
			if x < c.Width-1 {
				right = c.get(x+1, y-1)
			}
			// Obtain the minimum pixel value
			min := math.Min(math.Min(left, middle), right)
			c.set(x, y, c.get(x, y) + min)
		}
	}
	return c.Points
}

// Find the lowest vertical energy seam.
func (c *Carver) findLowestEnergySeams() []Seam {
	// Find the lowest cost seam from the energy matrix starting from the last row.
	var min float64 = math.MaxFloat64
	var px int
	seams := make([]Seam, 0)

	// Find the pixel on the last row with the minimum cumulative energy and use this as the starting pixel
	for x := 0; x < c.Width; x++ {
		seam := c.get(x, c.Height-1)
		if seam < min && seam > 0 {
			min = seam
			px = x
		}
	}
	seams = append(seams, Seam{X: px, Y: c.Height-1})
	var left, middle, right float64

	// Walk up in the matrix table,
	// check the immediate three top pixel seam level and
	// add add the one which has the lowest cumulative energy.
	for y := c.Height-2; y >= 0; y-- {
		left, right = math.MaxFloat64, math.MaxFloat64
		middle = c.get(px, y)
		// Leftmost seam, no child to the left
		if px == 0 {
			right = c.get(px+1, y)
			middle = c.get(px, y)
			if right < middle {
				px += 1
			}
		// Rightmost seam, no child to the right
		} else if px == c.Width-1 {
			left = c.get(px-1, y)
			middle = c.get(px, y)
			if left < middle {
				px -= 1
			}
		} else {
			left = c.get(px-1, y)
			middle = c.get(px, y)
			right = c.get(px+1, y)
			min := math.Min(math.Min(left, middle), right)

			if min == left {
				px -= 1
			} else if min == right {
				px += 1
			}
		}
		seams = append(seams, Seam{X: px, Y: y})
	}
	return seams
}

// Remove image pixels based on energy seams level.
func (c *Carver) removeSeam(img *image.NRGBA, seams []Seam) *image.NRGBA {
	bounds := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx()-1, bounds.Dy()))

	for _, seam := range seams {
		y := seam.Y
		for x := 0; x < bounds.Max.X; x++ {
			if seam.X == x {
				continue
				//dst.Set(x-1, y, color.RGBA{255, 0, 0, 255})
			} else if seam.X < x {
				dst.Set(x-1, y, img.At(x, y))
			} else {
				dst.Set(x, y, img.At(x, y))
			}
		}
	}
	return dst
}

// Resize is the main function taking the source image and encoding the rescaled image into the output file.
func (c *Carver) Resize(src *image.NRGBA, sobel *image.NRGBA, output string) (*os.File, error) {
	width, height := src.Bounds().Max.X, src.Bounds().Max.Y
	carver := NewCarver(width, height,c.SobelThreshold, c.BlurRadius, c.NewWidth, c.NewWidth, c.Percentage)
	resize := func() {
		carver.computeSeams(src)
		seams := carver.findLowestEnergySeams()
		src = carver.removeSeam(src, seams)
	}
	if carver.Percentage > 0 && (carver.NewWidth != 0 && carver.NewHeight != 0) {
		// Calculate new sizes based on provided percentage.
		nw := carver.Width - (carver.Width * (carver.Percentage/carver.Width))
		nh := carver.Height - (carver.Height * (carver.Percentage/carver.Height))
		for x := 0; x < nw; x++ {
			resize()
		}
		for y := 0; y < nh; y++ {
			resize()
		}
	} else {
		if carver.NewWidth > 0 && carver.NewWidth < carver.Width {
			for x := 0; x < carver.NewWidth; x++ {
				resize()
			}
		} else if carver.NewHeight > 0 && carver.NewHeight < carver.Height {
			for y := 0; y < carver.NewHeight; y++ {
				resize()
			}
		}
	}

	fq, err := os.Create(output)
	if err != nil {
		return nil, err
	}
	defer fq.Close()

	if err = png.Encode(fq, src); err != nil {
		return nil, err
	}
	return fq, nil
}