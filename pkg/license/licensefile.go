package license

import (
	"crypto/ed25519"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"strings"
	"time"

	keygen "github.com/keygen-sh/keygen-go/v3"
)

const (
	licenseFileBeginArmor = "-----BEGIN LICENSE FILE-----"
	licenseFileEndArmor   = "-----END LICENSE FILE-----"
)

var (
	// licenseFilePublicKeyPEM is the PEM-encoded Ed25519 public key of Obot's Keygen account, which signs license files.
	// It's compiled into the binary so configuration and the database can't change it.
	//go:embed keygen.pem
	licenseFilePublicKeyPEM []byte

	// These are shown to whoever supplied the license file.
	// They never include material from the file, since the file embeds the license key.
	errNotALicenseFile    = &FileError{Message: "not a license file; it must include the BEGIN and END lines"}
	errLicenseFileInvalid = &FileError{Message: "the license file is damaged, encrypted, or wasn't issued for this Obot release; copy the whole file again, or contact Obot"}
	errLicenseFileProduct = &FileError{Message: "the license file isn't for Obot; contact Obot"}
)

// FileError is returned when a license file is refused.
// Message is safe to show the user: it never includes material from the file.
type FileError struct {
	Message string
}

// licenseFile holds the claims of a verified license file.
// Expiry is the earlier of the license's expiry and the file's own expiry, or nil for a file that never expires.
type licenseFile struct {
	ID           string
	Expiry       *time.Time
	Entitlements map[keygen.EntitlementCode]struct{}
}

func (e *FileError) Error() string {
	return e.Message
}

// productionLicenseFilePublicKey returns the public key built into this release.
// A malformed keygen.pem returns nil, which refuses every license file; TestEmbeddedLicenseFilePublicKey catches that
// before release.
func productionLicenseFilePublicKey() ed25519.PublicKey {
	return parseLicenseFilePublicKey(licenseFilePublicKeyPEM)
}

// parseLicenseFilePublicKey decodes a PEM "PUBLIC KEY" block holding an Ed25519 key.
// Anything else returns nil, which refuses every license file.
func parseLicenseFilePublicKey(data []byte) ed25519.PublicKey {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil
	}
	publicKey, _ := key.(ed25519.PublicKey)
	return publicKey
}

// verifyLicenseFile verifies a Keygen license file against publicKey with keygen-go and returns its claims.
// It works offline and doesn't check the time; licenseFile.expired does that.
// On top of the signature, the file must be for Obot's Keygen product.
func verifyLicenseFile(contents string, publicKey ed25519.PublicKey) (file licenseFile, err error) {
	contents = normalizeLicenseFile(contents)
	if !strings.HasPrefix(contents, licenseFileBeginArmor) {
		return licenseFile{}, errNotALicenseFile
	}

	// keygen-go panics on a file whose envelope is null or names no algorithm.
	defer func() {
		if recover() != nil {
			file, err = licenseFile{}, errLicenseFileInvalid
		}
	}()

	// Verify checks the signature. Decode doesn't, so it only runs on a file that verified.
	keygenFile := &keygen.LicenseFile{Certificate: contents}
	if err := keygenFile.Verify(keygen.VerifyPublicKey(hex.EncodeToString(publicKey))); err != nil {
		return licenseFile{}, errLicenseFileInvalid
	}
	// Decode also compares the file's issue time and TTL with this host's clock, and still returns the dataset when
	// they're off. Those errors are ignored: the provider checks the expiry against its own clock on every read.
	dataset, _ := keygenFile.Decode()
	if dataset == nil {
		return licenseFile{}, errLicenseFileInvalid
	}
	if licenseFileProduct(contents) != keygenProduct {
		return licenseFile{}, errLicenseFileProduct
	}

	entitlements := make(map[keygen.EntitlementCode]struct{}, len(dataset.Entitlements))
	for _, entitlement := range dataset.Entitlements {
		if entitlement.Code != "" {
			entitlements[entitlement.Code] = struct{}{}
		}
	}

	// The file ends at the earlier of the license's expiry and the file's own, when it has either.
	expiry := dataset.License.Expiry
	if fileExpiry := dataset.Expiry; !fileExpiry.IsZero() && (expiry == nil || fileExpiry.Before(*expiry)) {
		expiry = &fileExpiry
	}

	return licenseFile{
		ID:           dataset.License.ID,
		Expiry:       expiry,
		Entitlements: entitlements,
	}, nil
}

// licenseFileProduct returns the Keygen product of the license in a verified license file, which keygen-go doesn't
// expose. It reads the envelope the same way keygen-go does, so it reads the dataset that was verified.
func licenseFileProduct(contents string) string {
	body := strings.TrimSuffix(strings.TrimPrefix(contents, licenseFileBeginArmor), licenseFileEndArmor)
	var envelope struct {
		Enc string `json:"enc"`
	}
	if data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(body)); err != nil || json.Unmarshal(data, &envelope) != nil {
		return ""
	}

	var dataset struct {
		Data struct {
			Relationships struct {
				Product struct {
					Data struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"product"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if data, err := base64.StdEncoding.DecodeString(envelope.Enc); err != nil || json.Unmarshal(data, &dataset) != nil {
		return ""
	}
	return dataset.Data.Relationships.Product.Data.ID
}

// normalizeLicenseFile trims what the verifier ignores around a license file: whitespace, and one leading UTF-8 BOM,
// which some editors add on save.
func normalizeLicenseFile(contents string) string {
	return strings.TrimSpace(strings.TrimPrefix(contents, "\ufeff"))
}

// expired reports whether f has reached its expiry at now. A file without one never expires.
func (f licenseFile) expired(now time.Time) bool {
	return f.Expiry != nil && !now.Before(*f.Expiry)
}
