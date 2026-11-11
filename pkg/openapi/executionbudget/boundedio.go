package executionbudget

import (
	"encoding/base64"
	"fmt"
	"io"
)

type LimitedReader struct {
	reader io.Reader
	limit  int64
	read   int64
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
	return count, err
}

func (reader *LimitedReader) BytesRead() int64 { return reader.read }

type LimitedWriter struct {
	writer io.Writer
	limit  int64
	wrote  int64
}

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

type MCPContent struct {
	Type       string `json:"type"`
	Data       string `json:"data"`
	MimeType   string `json:"mimeType"`
	StatusCode int    `json:"statusCode"`
	ByteLength int64  `json:"byteLength"`
	Truncated  bool   `json:"truncated"`
}

func ToBoundedMCPContent(reader io.Reader, contentType string, statusCode int, rawLimit int64) (MCPContent, error) {
	if rawLimit < 0 || Base64EnvelopeLimit(rawLimit) > Base64EnvelopeLimit(32*1024*1024) {
		return MCPContent{}, ErrLimitExceeded
	}
	body, err := io.ReadAll(LimitReader(reader, rawLimit))
	if err != nil {
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
