// Package s3 adapts AWS S3, MinIO and Cloudflare R2 to Media's ObjectStore port.
package s3

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
)

type Config struct {
	Provider        string
	Bucket          string
	Region          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	PublicBaseURL   string
	UsePathStyle    bool
}

type Adapter struct {
	client *awss3.Client
	config Config
}

func New(ctx context.Context, cfg Config) (*Adapter, error) {
	if strings.TrimSpace(cfg.Bucket) == "" || strings.TrimSpace(cfg.Region) == "" {
		return nil, fmt.Errorf("S3 bucket and region are required")
	}
	if strings.TrimSpace(cfg.PublicBaseURL) == "" {
		return nil, fmt.Errorf("a media public base URL is required")
	}
	publicURL, err := url.ParseRequestURI(cfg.PublicBaseURL)
	if err != nil || publicURL.Host == "" || (publicURL.Scheme != "http" && publicURL.Scheme != "https") {
		if err == nil {
			err = fmt.Errorf("public base URL must be absolute HTTP(S)")
		}
		return nil, fmt.Errorf("invalid media public base URL: %w", err)
	}
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(cfg.Region)}
	if strings.TrimSpace(cfg.AccessKeyID) != "" || strings.TrimSpace(cfg.SecretAccessKey) != "" {
		if strings.TrimSpace(cfg.AccessKeyID) == "" || strings.TrimSpace(cfg.SecretAccessKey) == "" {
			return nil, fmt.Errorf("both S3 access key and secret are required")
		}
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load S3 configuration: %w", err)
	}
	client := awss3.NewFromConfig(awsCfg, func(o *awss3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
		if endpoint := strings.TrimSpace(cfg.Endpoint); endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})
	return &Adapter{client: client, config: cfg}, nil
}

func (a *Adapter) Put(ctx context.Context, request media.PutRequest) (media.StoredObject, error) {
	if a == nil || a.client == nil || request.Body == nil || strings.TrimSpace(request.Key) == "" || request.ContentLength <= 0 {
		return media.StoredObject{}, fmt.Errorf("invalid media object put request")
	}
	bucket := request.Bucket
	if bucket == "" {
		bucket = a.config.Bucket
	}
	output, err := a.client.PutObject(ctx, &awss3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(request.Key), Body: request.Body, ContentLength: aws.Int64(request.ContentLength), ContentType: aws.String(request.ContentType), Metadata: map[string]string{"sha256": request.ChecksumSHA256}})
	if err != nil {
		return media.StoredObject{}, fmt.Errorf("put media object: %w", err)
	}
	return media.StoredObject{ObjectRef: media.ObjectRef{Provider: a.config.Provider, Bucket: bucket, Key: request.Key}, ETag: strings.Trim(aws.ToString(output.ETag), "\""), SizeBytes: request.ContentLength}, nil
}

func (a *Adapter) Get(ctx context.Context, object media.ObjectRef) (io.ReadCloser, error) {
	if a == nil || a.client == nil || strings.TrimSpace(object.Key) == "" {
		return nil, fmt.Errorf("invalid media object read")
	}
	bucket := object.Bucket
	if bucket == "" {
		bucket = a.config.Bucket
	}
	output, err := a.client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(object.Key)})
	if err != nil {
		return nil, fmt.Errorf("get media object: %w", err)
	}
	return output.Body, nil
}

// List fully paginates ListObjectsV2 so callers cannot accidentally leave
// later S3 pages unreconciled. It returns metadata only, never object bytes.
func (a *Adapter) List(ctx context.Context, prefix string) ([]media.ListedObject, error) {
	if a == nil || a.client == nil {
		return nil, fmt.Errorf("invalid media object listing")
	}
	prefix = strings.TrimSpace(prefix)
	bucket := a.config.Bucket
	paginator := awss3.NewListObjectsV2Paginator(a.client, &awss3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)})
	objects := make([]media.ListedObject, 0)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list media objects: %w", err)
		}
		for _, item := range page.Contents {
			if item.Key == nil || item.LastModified == nil {
				continue
			}
			objects = append(objects, media.ListedObject{ObjectRef: media.ObjectRef{Provider: a.config.Provider, Bucket: bucket, Key: aws.ToString(item.Key)}, LastModified: *item.LastModified})
		}
	}
	return objects, nil
}

func (a *Adapter) Delete(ctx context.Context, object media.ObjectRef) error {
	if a == nil || a.client == nil || strings.TrimSpace(object.Key) == "" {
		return fmt.Errorf("invalid media object deletion")
	}
	bucket := object.Bucket
	if bucket == "" {
		bucket = a.config.Bucket
	}
	_, err := a.client.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(object.Key)})
	if err != nil {
		return fmt.Errorf("delete media object: %w", err)
	}
	return nil
}

func (a *Adapter) PublicURL(_ context.Context, object media.ObjectRef) (string, error) {
	if a == nil || strings.TrimSpace(object.Key) == "" {
		return "", fmt.Errorf("invalid media object URL")
	}
	base, err := url.Parse(a.config.PublicBaseURL)
	if err != nil {
		return "", fmt.Errorf("parse media public URL: %w", err)
	}
	base.Path = path.Join(base.Path, object.Key)
	return base.String(), nil
}

var _ media.ObjectStore = (*Adapter)(nil)
