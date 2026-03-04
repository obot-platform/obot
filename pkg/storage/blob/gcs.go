package blob

import (
	"context"
	"fmt"
	"io"

	gcsstorage "cloud.google.com/go/storage"
	"github.com/obot-platform/obot/apiclient/types"
	"google.golang.org/api/option"
)

// GCSStore implements BlobStore for Google Cloud Storage.
type GCSStore struct {
	config types.GCSConfig
}

// NewGCSStore creates a new GCS blob store.
func NewGCSStore(config types.GCSConfig) (*GCSStore, error) {
	return &GCSStore{config: config}, nil
}

func (g *GCSStore) Upload(ctx context.Context, bucket, key string, data io.Reader) error {
	client, err := g.createClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	writer := client.Bucket(bucket).Object(key).NewWriter(ctx)
	if _, err = io.Copy(writer, data); err != nil {
		writer.Close()
		return err
	}
	return writer.Close()
}

func (g *GCSStore) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	client, err := g.createClient(ctx)
	if err != nil {
		return nil, err
	}

	reader, err := client.Bucket(bucket).Object(key).NewReader(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to download from GCS: %w", err)
	}
	// The client must stay open while the reader is in use.
	return &gcsReadCloser{reader: reader, client: client}, nil
}

func (g *GCSStore) Delete(ctx context.Context, bucket, key string) error {
	client, err := g.createClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Bucket(bucket).Object(key).Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete from GCS: %w", err)
	}
	return nil
}

func (g *GCSStore) Test(ctx context.Context) error {
	client, err := g.createClient(ctx)
	if err != nil {
		return fmt.Errorf("invalid GCS credentials: %w", err)
	}
	defer client.Close()
	return nil
}

func (g *GCSStore) createClient(ctx context.Context) (*gcsstorage.Client, error) {
	if g.config.ServiceAccountJSON != "" {
		return gcsstorage.NewClient(ctx, option.WithCredentialsJSON([]byte(g.config.ServiceAccountJSON)))
	}
	return gcsstorage.NewClient(ctx)
}

// gcsReadCloser wraps a GCS reader and client so both are closed together.
type gcsReadCloser struct {
	reader *gcsstorage.Reader
	client *gcsstorage.Client
}

func (r *gcsReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *gcsReadCloser) Close() error {
	readerErr := r.reader.Close()
	clientErr := r.client.Close()
	if readerErr != nil {
		return readerErr
	}
	return clientErr
}
