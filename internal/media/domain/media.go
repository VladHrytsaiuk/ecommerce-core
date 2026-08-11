// Package domain defines provider-neutral Media contracts.
package domain

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
)

const TopicAssetUploaded = "media.asset.uploaded.v1"

type AssetStatus string

const (
	AssetQuarantine AssetStatus = "quarantine"
	AssetProcessing AssetStatus = "processing"
	AssetReady      AssetStatus = "ready"
	AssetFailed     AssetStatus = "failed"
)

type ObjectRef struct {
	Provider string
	Bucket   string
	Key      string
}

type SourceObject struct {
	AssetID        uuid.UUID
	Object         ObjectRef
	MIMEType       string
	SizeBytes      int64
	ChecksumSHA256 string
}

type StoredObject struct {
	ObjectRef
	ETag      string
	SizeBytes int64
	Width     int
	Height    int
}

// ListedObject is the minimal provider-neutral metadata needed by the
// reconciliation worker. Object bytes are never read during orphan cleanup.
type ListedObject struct {
	ObjectRef
	LastModified time.Time
}

type PutRequest struct {
	ObjectRef
	Body           io.Reader
	ContentType    string
	ContentLength  int64
	ChecksumSHA256 string
}

// ObjectStore is implemented by S3-compatible providers. It never exposes a
// provider SDK to application services.
type ObjectStore interface {
	Put(context.Context, PutRequest) (StoredObject, error)
	Get(context.Context, ObjectRef) (io.ReadCloser, error)
	List(context.Context, string) ([]ListedObject, error)
	Delete(context.Context, ObjectRef) error
	PublicURL(context.Context, ObjectRef) (string, error)
}

// ImageProcessor is reserved for the next phase (libvips/Cloudinary transforms).
type ImageProcessor interface {
	Process(context.Context, SourceObject) ([]StoredObject, error)
}

type Asset struct {
	ID             uuid.UUID
	UploadID       uuid.UUID
	Object         ObjectRef
	Status         AssetStatus
	ChecksumSHA256 string
	SizeBytes      int64
	MIMEType       string
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
}

type Variant struct {
	AssetID       uuid.UUID
	Key           string
	Object        ObjectRef
	Width, Height int
	SizeBytes     int64
	MIMEType      string
}
