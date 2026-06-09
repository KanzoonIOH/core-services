package lib

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type ObjectStorage struct {
	client         *s3.Client
	bucket         string
	publicEndpoint string
}

func NewObjectStorage(ctx context.Context, endpoint string, publicEndpoint string, region string, accessKey string, secretKey string, bucket string) (*ObjectStorage, error) {
	if region == "" {
		region = "us-east-1"
	}
	endpoint = normalizeEndpoint(endpoint)
	publicEndpoint = normalizeEndpoint(publicEndpoint)

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
	)
	if err != nil {
		return nil, fmt.Errorf("load object storage config: %w", err)
	}

	if publicEndpoint == "" {
		publicEndpoint = endpoint
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	return &ObjectStorage{
		client:         client,
		bucket:         bucket,
		publicEndpoint: strings.TrimRight(publicEndpoint, "/"),
	}, nil
}

func (s *ObjectStorage) Upload(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	if contentType == "" {
		contentType = mime.TypeByExtension(path.Ext(key))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("put object: %w", err)
	}

	return s.ObjectURL(key), nil
}

func (s *ObjectStorage) ObjectURL(key string) string {
	return s.publicEndpoint + "/" + path.Join(s.bucket, escapeObjectKey(key))
}

func normalizeEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	return "https://" + endpoint
}

func escapeObjectKey(key string) string {
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
