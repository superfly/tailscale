// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package qnap

import (
	"bytes"
	"fmt"
	"image"
	"image/png"

	"golang.org/x/image/draw"
)

// scalePNG decodes a PNG image from src and returns a new PNG scaled
// to size x size pixels using high-quality Catmull-Rom interpolation.
func scalePNG(src []byte, size int) ([]byte, error) {
	srcImg, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decoding source PNG: %w", err)
	}

	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), srcImg, srcImg.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("encoding scaled PNG: %w", err)
	}
	return buf.Bytes(), nil
}
