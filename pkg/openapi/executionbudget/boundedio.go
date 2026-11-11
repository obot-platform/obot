package executionbudget

import (
	"compress/flate"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

type LimitedReader struct {
	reader io.Reader
	limit  int64
	read   int64
}

type BoundedUpstreamReader struct {
	source  *LimitedReader
	decoder io.Reader
	output  *LimitedReader
	closer  io.Closer
}

type LimitedWriter struct {
	writer io.Writer
	limit  int64
	wrote  int64
}

type MCPContent struct {
	Type       string `json:"type"`
	Data       string `json:"data"`
	MimeType   string `json:"mimeType"`
	StatusCode int    `json:"statusCode"`
	ByteLength int64  `json:"byteLength"`
	Truncated  bool   `json:"truncated"`
}

func LimitReader(reader io.Reader, limit int64) *LimitedReader {
	return &LimitedReader{reader: reader, limit: limit}
}

func LimitRequestBody(reader io.Reader, limit int64) *LimitedReader {
	return LimitReader(reader, limit)
}

func LimitUpstreamBody(reader io.Reader, limit int64) *LimitedReader {
	return LimitReader(reader, limit)
}

func LimitUpstreamBodyWithEncoding(reader io.Reader, contentEncoding string, compressedLimit, decompressedLimit int64) (*BoundedUpstreamReader, error) {
	if compressedLimit <= 0 || decompressedLimit <= 0 {
		return nil, ErrLimitExceeded
	}
	source := LimitReader(reader, compressedLimit)
	bounded := &BoundedUpstreamReader{source: source}
	switch strings.ToLower(strings.TrimSpace(contentEncoding)) {
	case "", "identity":
		bounded.decoder = source
	case "gzip":
		gzipReader, err := gzip.NewReader(source)
		if err != nil {
			return nil, err
		}
		bounded.decoder = gzipReader
		bounded.closer = gzipReader
	case "deflate":
		deflateReader := flate.NewReader(source)
		bounded.decoder = deflateReader
		bounded.closer = deflateReader
	default:
		return nil, ErrUnsupportedContentEncoding
	}
	bounded.output = LimitReader(bounded.decoder, decompressedLimit)
	return bounded, nil
}

func (reader *BoundedUpstreamReader) Read(output []byte) (int, error) {
	return reader.output.Read(output)
}

func (reader *BoundedUpstreamReader) Close() error {
	if reader.closer == nil {
		return nil
	}
	return reader.closer.Close()
}

func (reader *LimitedReader) Read(output []byte) (int, error) {
	if reader.read > reader.limit {
		return 0, fmt.Errorf("%w after %d bytes", ErrLimitExceeded, reader.limit)
	}
	if reader.read == reader.limit {
		probe := make([]byte, 1)
		count, err := reader.reader.Read(probe)
		reader.read += int64(count)
		if count > 0 {
			return count, fmt.Errorf("%w after %d bytes", ErrLimitExceeded, reader.limit)
		}
		return count, err
	}
	remaining := reader.limit + 1 - reader.read
	if int64(len(output)) > remaining {
		output = output[:remaining]
	}
	count, err := reader.reader.Read(output)
	reader.read += int64(count)
	if count > 0 && reader.read > reader.limit {
		return count, fmt.Errorf("%w after %d bytes", ErrLimitExceeded, reader.limit)
	}
	return count, err
}

func (reader *LimitedReader) BytesRead() int64 { return reader.read }

func LimitHTTPResponse(writer io.Writer, limit int64) *LimitedWriter {
	return &LimitedWriter{writer: writer, limit: limit}
}

func (writer *LimitedWriter) Write(output []byte) (int, error) {
	if writer.wrote+int64(len(output)) > writer.limit {
		return 0, fmt.Errorf("%w after %d bytes", ErrLimitExceeded, writer.limit)
	}
	count, err := writer.writer.Write(output)
	writer.wrote += int64(count)
	return count, err
}

func (writer *LimitedWriter) BytesWritten() int64 { return writer.wrote }

func Base64EnvelopeLimit(rawLimit int64) int64 {
	return 4*((rawLimit+2)/3) + 64*1024
}

func LimitWorkerEnvelope(reader io.Reader, rawLimit int64) *LimitedReader {
	return LimitReader(reader, Base64EnvelopeLimit(rawLimit))
}

func ToBoundedMCPContent(reader io.Reader, contentType string, statusCode int, rawLimit int64) (MCPContent, error) {
	if rawLimit < 0 || Base64EnvelopeLimit(rawLimit) > Base64EnvelopeLimit(32*1024*1024) {
		return MCPContent{}, ErrLimitExceeded
	}
	body, err := io.ReadAll(LimitReader(reader, rawLimit))
	if err != nil {
		if errors.Is(err, ErrLimitExceeded) {
			return MCPContent{}, ErrMCPContentTooLarge
		}
		return MCPContent{}, err
	}
	return MCPContent{
		Type:       "resource",
		Data:       base64.StdEncoding.EncodeToString(body),
		MimeType:   contentType,
		StatusCode: statusCode,
		ByteLength: int64(len(body)),
		Truncated:  false,
	}, nil
}
