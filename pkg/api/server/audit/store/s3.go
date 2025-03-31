package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3StoreOptions struct {
	AuditLogsStoreS3Bucket   string `usage:"Audit log store S3 bucket"`
	AuditLogsStoreS3Endpoint string `usage:"Audit log store S3 endpoint"`
}

type s3Store struct {
	host, bucket string
	compress     bool
	client       *s3.Client
}

func NewS3Store(host string, compress bool, options S3StoreOptions) (Store, error) {
	if options.AuditLogsStoreS3Bucket == "" {
		return nil, errors.New("Audit log store S3 bucket is required")
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if options.AuditLogsStoreS3Endpoint != "" {
			o.BaseEndpoint = aws.String(options.AuditLogsStoreS3Endpoint)
		}
		o.RetryMaxAttempts = 1
	})

	return &s3Store{
		host:     host,
		bucket:   options.AuditLogsStoreS3Bucket,
		compress: compress,
		client:   client,
	}, nil
}

func (s *s3Store) Persist(b []byte) error {
	var reader io.Reader
	if s.compress {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)

		if _, err := gw.Write(b); err != nil {
			return fmt.Errorf("failed to compress audit log: %v", err)
		}

		if err := gw.Close(); err != nil {
			return fmt.Errorf("failed to close gzip writer: %v", err)
		}

		reader = bytes.NewReader(buf.Bytes())
	} else {
		reader = bytes.NewReader(b)
	}

	_, err := s.client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(filename(s.host)),
		Body:   reader,
	})
	return err
}
