package http

import (
	"bytes"
	"errors"
	"testing"
)

func TestReadImagePartAcceptsMagicBytesAndHashesStream(t *testing.T) {
	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, bytes.Repeat([]byte{0}, 64)...)
	file, err := ReadImagePart(bytes.NewReader(png), MaxUploadBytes)
	if err != nil {
		t.Fatalf("ReadImagePart() error = %v", err)
	}
	defer file.Close()
	if file.MIMEType != "image/png" || file.SizeBytes != int64(len(png)) || len(file.ChecksumSHA256) != 64 {
		t.Fatalf("unexpected file %+v", file)
	}
}

func TestReadImagePartRejectsUnknownMagicBytes(t *testing.T) {
	_, err := ReadImagePart(bytes.NewReader([]byte("not an image")), MaxUploadBytes)
	if !errors.Is(err, ErrUnsupportedMediaType) {
		t.Fatalf("error = %v, want unsupported media", err)
	}
}

func TestReadImagePartRejectsOversizeStream(t *testing.T) {
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{0}, 1024)...)
	_, err := ReadImagePart(bytes.NewReader(jpeg), 512)
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("error = %v, want too large", err)
	}
}
