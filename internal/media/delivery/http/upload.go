// Package http is the Media transport adapter.
package http

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	mediaApp "github.com/VladHrytsaiuk/ecommerce-core/internal/media/application"
)

const MaxUploadBytes int64 = 15 << 20
const MaxMultipartRequestBytes int64 = MaxUploadBytes + 1024

var ErrFileTooLarge = errors.New("media file exceeds 15 MiB")
var ErrUnsupportedMediaType = errors.New("only JPEG, PNG, and WebP images are supported")
var ErrInvalidMultipart = errors.New("upload must contain exactly one file field named file")

// UploadFile is a bounded temporary spool. It is removed by Close and never
// logs a user-controlled original filename.
type UploadFile struct {
	path, MIMEType, ChecksumSHA256 string
	SizeBytes                      int64
}

func (f UploadFile) Open() (*os.File, error) { return os.Open(f.path) }
func (f UploadFile) Close() error {
	if f.path == "" {
		return nil
	}
	return os.Remove(f.path)
}

func ReadImagePart(part io.Reader, maxBytes int64) (UploadFile, error) {
	if part == nil || maxBytes <= 0 {
		return UploadFile{}, ErrInvalidMultipart
	}
	temporary, err := os.CreateTemp("", "ecommerce-media-upload-*")
	if err != nil {
		return UploadFile{}, fmt.Errorf("create upload spool: %w", err)
	}
	name := temporary.Name()
	cleanup := func(err error) (UploadFile, error) {
		_ = temporary.Close()
		_ = os.Remove(name)
		return UploadFile{}, err
	}
	buffer := make([]byte, 512)
	n, readErr := io.ReadFull(part, buffer)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		return cleanup(fmt.Errorf("read image header: %w", readErr))
	}
	if n == 0 {
		return cleanup(ErrUnsupportedMediaType)
	}
	mimeType := http.DetectContentType(buffer[:n])
	if !isAllowedMIME(mimeType) {
		return cleanup(ErrUnsupportedMediaType)
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(io.MultiReader(bytes.NewReader(buffer[:n]), part), maxBytes+1))
	if err != nil {
		return cleanup(fmt.Errorf("write image spool: %w", err))
	}
	if written > maxBytes {
		return cleanup(ErrFileTooLarge)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(name)
		return UploadFile{}, fmt.Errorf("close image spool: %w", err)
	}
	return UploadFile{path: name, MIMEType: mimeType, ChecksumSHA256: hex.EncodeToString(hash.Sum(nil)), SizeBytes: written}, nil
}

func isAllowedMIME(value string) bool {
	return value == "image/jpeg" || value == "image/png" || value == "image/webp"
}

func RegisterV1Routes(group *gin.RouterGroup, authorizer adminDomain.Authorizer, service *mediaApp.UploadService, renderer *apiresponse.ErrorRenderer) {
	if group == nil || service == nil || renderer == nil {
		return
	}
	group.POST("/upload", shared.RequirePermissionV1(authorizer, mediaApp.PermissionMediaWrite, renderer), upload(service, renderer))
}

// upload godoc
// @Summary Upload a product media asset
// @Description Streams one JPEG, PNG or WebP image (maximum 15 MiB) to quarantine.
// @Tags Admin v1
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Image file"
// @Param Idempotency-Key header string true "Stable upload ID"
// @Success 201 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,413,422,503 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/media/upload [post]
func upload(service *mediaApp.UploadService, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			renderer.Abort(c, apiresponse.Unauthenticated(errors.New("authenticated subject missing")))
			return
		}
		uploadID, err := idempotencyKey(c)
		if err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		// Keep the transport-level bound independent of route wiring. This also
		// caps multipart framing and unexpected non-file parts.
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxMultipartRequestBytes)
		reader, err := c.Request.MultipartReader()
		if err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(ErrInvalidMultipart))
			return
		}
		file, err := readSingleFile(reader)
		if err != nil {
			abortUploadError(c, renderer, err)
			return
		}
		defer file.Close()
		body, err := file.Open()
		if err != nil {
			renderer.Abort(c, err)
			return
		}
		defer body.Close()
		asset, err := service.Upload(c.Request.Context(), mediaApp.UploadCommand{ActorID: actor, UploadID: uploadID, Body: body, SizeBytes: file.SizeBytes, MIMEType: file.MIMEType, ChecksumSHA256: file.ChecksumSHA256})
		if err != nil {
			renderer.Abort(c, err)
			return
		}
		apiresponse.Success(c, http.StatusCreated, gin.H{"asset_id": asset.ID, "status": asset.Status, "mime_type": asset.MIMEType, "size_bytes": asset.SizeBytes})
	}
}

func readSingleFile(reader *multipart.Reader) (UploadFile, error) {
	var file UploadFile
	found := false
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var limitError *http.MaxBytesError
			if errors.As(err, &limitError) {
				return UploadFile{}, err
			}
			return UploadFile{}, ErrInvalidMultipart
		}
		if part.FormName() != "file" || part.FileName() == "" || found {
			_ = part.Close()
			return UploadFile{}, ErrInvalidMultipart
		}
		file, err = ReadImagePart(part, MaxUploadBytes)
		_ = part.Close()
		if err != nil {
			return UploadFile{}, err
		}
		found = true
	}
	if !found {
		return UploadFile{}, ErrInvalidMultipart
	}
	return file, nil
}

func idempotencyKey(c *gin.Context) (uuid.UUID, error) {
	raw := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	value, err := uuid.Parse(raw)
	if err != nil || value == uuid.Nil {
		return uuid.Nil, errors.New("Idempotency-Key must be a UUID")
	}
	return value, nil
}
func abortUploadError(c *gin.Context, renderer *apiresponse.ErrorRenderer, err error) {
	var limitError *http.MaxBytesError
	if errors.Is(err, ErrFileTooLarge) || errors.As(err, &limitError) {
		renderer.Abort(c, apiresponse.PayloadTooLarge(err))
		return
	}
	renderer.Abort(c, apiresponse.InvalidPayload(err))
}
