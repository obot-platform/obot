package guardrails

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// signAWSRequest signs req with AWS Signature Version 4 for service in region.
func signAWSRequest(req *http.Request, creds aws.Credentials, service, region string, signingTime time.Time) error {
	if req.Body == nil {
		req.Body = http.NoBody
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("failed to copy request body for AWS signing: %w", err)
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))

	sum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(sum[:])
	req.Header.Del("Authorization")
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	return v4signer.NewSigner().SignHTTP(req.Context(), creds, req, payloadHash, service, region, signingTime)
}
