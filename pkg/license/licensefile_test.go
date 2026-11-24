package license

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	keygen "github.com/keygen-sh/keygen-go/v3"
)

const (
	// licenseFileAlgorithm is the algorithm of the unencrypted, Ed25519-signed license files Obot issues.
	licenseFileAlgorithm = "base64+ed25519"

	testLicenseFileID        = "6f1c3b0e-2d4a-4c55-9a7e-000000000001"
	testLicenseFileProductID = keygenProduct
	testLicenseFilePolicyID  = "6f1c3b0e-2d4a-4c55-9a7e-00000000001c"
	testLicenseFileKey       = "TEST-EMBEDDED-LICENSE-KEY-0123456789-MUST-NOT-LEAK"
	testLicenseFileLicensee  = "Example Customer, Inc."
	testLicenseFileIssued    = "2026-11-13T00:00:00.000Z"
	testLicenseFileExpiry    = "2027-11-13T00:00:00.000Z"
)

// testLicenseFileIssuer stands in for a Keygen account, signing license files with a key derived from a seed.
// verifyKey is its public key, publicKey the same key in hex as Keygen shows it, and publicKeyPEM in PEM.
type testLicenseFileIssuer struct {
	key          ed25519.PrivateKey
	verifyKey    ed25519.PublicKey
	publicKey    string
	publicKeyPEM []byte
}

func newTestLicenseFileIssuer(seed string) testLicenseFileIssuer {
	digest := sha256.Sum256([]byte(seed))
	key := ed25519.NewKeyFromSeed(digest[:])
	verifyKey := key.Public().(ed25519.PublicKey)
	return testLicenseFileIssuer{
		key:          key,
		verifyKey:    verifyKey,
		publicKey:    hex.EncodeToString(verifyKey),
		publicKeyPEM: mustMarshalPublicKeyPEM(verifyKey),
	}
}

// mustMarshalPublicKeyPEM encodes key as a PEM "PUBLIC KEY" block.
func mustMarshalPublicKeyPEM(key any) []byte {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		panic(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

// envelope signs dataset the way Keygen does and returns the envelope's members, for tests to change before armoring.
func (i testLicenseFileIssuer) envelope(dataset []byte, alg string) map[string]string {
	enc := base64.StdEncoding.EncodeToString(dataset)
	sig := ed25519.Sign(i.key, []byte(keygen.LicenseFileSigningPrefix+enc))
	return map[string]string{
		"enc": enc,
		"sig": base64.StdEncoding.EncodeToString(sig),
		"alg": alg,
	}
}

// sign returns a complete license file for dataset.
func (i testLicenseFileIssuer) sign(dataset []byte) string {
	return armorTestLicenseFile(mustMarshalJSON(i.envelope(dataset, licenseFileAlgorithm)))
}

func (i testLicenseFileIssuer) signDataset(dataset map[string]any) string {
	return i.sign(mustMarshalJSON(dataset))
}

func armorTestLicenseFile(envelope []byte) string {
	return armorTestLicenseFileBody(base64.StdEncoding.EncodeToString(envelope))
}

// armorTestLicenseFileBody wraps a base64 body at 80 columns between the armor lines.
func armorTestLicenseFileBody(body string) string {
	return licenseFileBeginArmor + "\n" + strings.Join(splitEvery(body, 80), "\n") + "\n" + licenseFileEndArmor + "\n"
}

func mustMarshalJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func testEntitlement(code string) map[string]any {
	return map[string]any{
		"id":   "entitlement-" + strings.ToLower(code),
		"type": "entitlements",
		"attributes": map[string]any{
			"name": code,
			"code": code,
		},
	}
}

func testRelationship(resourceType, id string) map[string]any {
	return map[string]any{
		"links": map[string]any{"related": "/v1/accounts/test/" + resourceType + "/" + id},
		"data":  map[string]any{"type": resourceType, "id": id},
	}
}

// testLicenseFileDataset returns a valid dataset shaped like the ones Keygen issues.
// Besides what the verifier reads, it has:
// - Members the verifier ignores, including the license key it must never leak
// - A duplicate OBOT_ENTERPRISE entitlement
// - An included resource that isn't an entitlement but has a code
func testLicenseFileDataset() map[string]any {
	return map[string]any{
		"data": map[string]any{
			"id":   testLicenseFileID,
			"type": "licenses",
			"attributes": map[string]any{
				"name":     testLicenseFileLicensee,
				"key":      testLicenseFileKey,
				"expiry":   testLicenseFileExpiry,
				"status":   "ACTIVE",
				"metadata": map[string]any{},
			},
			"relationships": map[string]any{
				"account": testRelationship("accounts", "6f1c3b0e-2d4a-4c55-9a7e-00000000acc7"),
				"product": testRelationship("products", testLicenseFileProductID),
				"policy":  testRelationship("policies", testLicenseFilePolicyID),
			},
		},
		"included": []any{
			testEntitlement("OBOT_ENTERPRISE_AUTH_PROVIDERS"),
			testEntitlement(EnterpriseEntitlement),
			testEntitlement("OBOT_ENTERPRISE_250_USERS"),
			testEntitlement("OBOT_FUTURE_FEATURE"),
			testEntitlement(EnterpriseEntitlement),
			map[string]any{
				"id":         testLicenseFileProductID,
				"type":       "products",
				"attributes": map[string]any{"name": "Obot", "code": "NOT_AN_ENTITLEMENT"},
			},
		},
		"meta": map[string]any{
			"issued": testLicenseFileIssued,
			"expiry": nil,
			"ttl":    nil,
		},
	}
}

// setAttribute returns a mutation that sets a license attribute.
func setAttribute(name string, value any) func(map[string]any) {
	return func(d map[string]any) { datasetObject(d, "data", "attributes")[name] = value }
}

// setMeta returns a mutation that sets a member of the dataset's meta.
func setMeta(name string, value any) func(map[string]any) {
	return func(d map[string]any) { datasetObject(d, "meta")[name] = value }
}

// datasetObject returns the object at path in dataset, for tests to change in place.
func datasetObject(dataset map[string]any, path ...string) map[string]any {
	for _, name := range path {
		dataset = dataset[name].(map[string]any)
	}
	return dataset
}

func mustParseTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}

func splitEvery(s string, n int) []string {
	var parts []string
	for len(s) > n {
		parts, s = append(parts, s[:n]), s[n:]
	}
	return append(parts, s)
}

func TestVerifyLicenseFile(t *testing.T) {
	issuer := newTestLicenseFileIssuer("valid")

	got, err := verifyLicenseFile(issuer.signDataset(testLicenseFileDataset()), issuer.verifyKey)
	if err != nil {
		t.Fatalf("verifyLicenseFile() error = %v", err)
	}
	if got.ID != testLicenseFileID {
		t.Errorf("ID = %q, want %q", got.ID, testLicenseFileID)
	}
	if want := mustParseTime(testLicenseFileExpiry); got.Expiry == nil || !got.Expiry.Equal(want) {
		t.Errorf("Expiry = %v, want %v", got.Expiry, want)
	}
	// Duplicates collapse, and an included resource that isn't an entitlement is skipped.
	want := []keygen.EntitlementCode{"OBOT_ENTERPRISE", "OBOT_ENTERPRISE_250_USERS", "OBOT_ENTERPRISE_AUTH_PROVIDERS", "OBOT_FUTURE_FEATURE"}
	if codes := slices.Sorted(maps.Keys(got.Entitlements)); !slices.Equal(codes, want) {
		t.Errorf("Entitlements = %v, want %v", codes, want)
	}
}

// A file checked out without its entitlements is still a valid license, like a license key with none: it grants no
// features.
func TestVerifyLicenseFileWithoutEntitlements(t *testing.T) {
	issuer := newTestLicenseFileIssuer("no entitlements")
	dataset := testLicenseFileDataset()
	dataset["included"] = []any{}

	got, err := verifyLicenseFile(issuer.signDataset(dataset), issuer.verifyKey)
	if err != nil {
		t.Fatalf("verifyLicenseFile() error = %v", err)
	}
	if got.Entitlements == nil || len(got.Entitlements) != 0 {
		t.Fatalf("Entitlements = %v, want an empty set", got.Entitlements)
	}
}

func TestVerifyLicenseFileAcceptedForms(t *testing.T) {
	issuer := newTestLicenseFileIssuer("forms")
	dataset := mustMarshalJSON(testLicenseFileDataset())
	file := issuer.sign(dataset)
	body := base64.StdEncoding.EncodeToString(mustMarshalJSON(issuer.envelope(dataset, licenseFileAlgorithm)))

	tests := []struct {
		name     string
		contents string
	}{
		{
			name:     "CRLF line endings",
			contents: strings.ReplaceAll(file, "\n", "\r\n"),
		},
		{
			name:     "leading byte order mark",
			contents: "\xef\xbb\xbf" + file,
		},
		{
			name:     "surrounding whitespace",
			contents: "\n\n  " + file + "\n\n",
		},
		{
			name:     "body on one line",
			contents: licenseFileBeginArmor + "\n" + body + "\n" + licenseFileEndArmor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := verifyLicenseFile(tt.contents, issuer.verifyKey); err != nil {
				t.Fatalf("verifyLicenseFile() error = %v", err)
			}
		})
	}
}

func TestVerifyLicenseFileRefuses(t *testing.T) {
	issuer := newTestLicenseFileIssuer("refuses")
	dataset := mustMarshalJSON(testLicenseFileDataset())
	other := issuer.envelope([]byte(`{"data":{}}`), licenseFileAlgorithm)

	withEnvelope := func(change func(envelope map[string]string)) string {
		envelope := issuer.envelope(dataset, licenseFileAlgorithm)
		change(envelope)
		return armorTestLicenseFile(mustMarshalJSON(envelope))
	}
	withDataset := func(mutate func(dataset map[string]any)) string {
		changed := testLicenseFileDataset()
		mutate(changed)
		return issuer.signDataset(changed)
	}

	tests := []struct {
		name        string
		contents    string
		noPublicKey bool
		wantErr     error
	}{
		{
			name:     "empty",
			contents: "",
			wantErr:  errNotALicenseFile,
		},
		{
			name:     "a license key",
			contents: "0A1B2C-3D4E5F-6A7B8C-9D0E1F",
			wantErr:  errNotALicenseFile,
		},
		{
			name:     "body isn't base64",
			contents: licenseFileBeginArmor + "\n!!!\n" + licenseFileEndArmor,
			wantErr:  errLicenseFileInvalid,
		},
		{
			name:     "signed by another account",
			contents: newTestLicenseFileIssuer("another account").sign(dataset),
			wantErr:  errLicenseFileInvalid,
		},
		{
			name:     "dataset replaced",
			contents: withEnvelope(func(e map[string]string) { e["enc"] = other["enc"] }),
			wantErr:  errLicenseFileInvalid,
		},
		{
			name:     "signature replaced",
			contents: withEnvelope(func(e map[string]string) { e["sig"] = other["sig"] }),
			wantErr:  errLicenseFileInvalid,
		},
		{
			name:        "no public key",
			contents:    issuer.sign(dataset),
			noPublicKey: true,
			wantErr:     errLicenseFileInvalid,
		},
		{
			name:     "encrypted",
			contents: withEnvelope(func(e map[string]string) { e["alg"] = "aes-256-gcm+ed25519" }),
			wantErr:  errLicenseFileInvalid,
		},
		{
			name:     "another signing algorithm",
			contents: withEnvelope(func(e map[string]string) { e["alg"] = "base64+ecdsa-p256" }),
			wantErr:  errLicenseFileInvalid,
		},
		{
			// keygen-go panics on these three.
			name:     "algorithm without a plus",
			contents: withEnvelope(func(e map[string]string) { e["alg"] = "ed25519" }),
			wantErr:  errLicenseFileInvalid,
		},
		{
			name:     "no algorithm",
			contents: withEnvelope(func(e map[string]string) { delete(e, "alg") }),
			wantErr:  errLicenseFileInvalid,
		},
		{
			name:     "null envelope",
			contents: armorTestLicenseFile([]byte("null")),
			wantErr:  errLicenseFileInvalid,
		},
		{
			name:     "dataset isn't JSON",
			contents: issuer.sign([]byte("not json")),
			wantErr:  errLicenseFileInvalid,
		},
		{
			name: "another product",
			contents: withDataset(func(d map[string]any) {
				datasetObject(d, "data", "relationships")["product"] = testRelationship("products", "another-product")
			}),
			wantErr: errLicenseFileProduct,
		},
		{
			name:     "no product",
			contents: withDataset(func(d map[string]any) { delete(datasetObject(d, "data", "relationships"), "product") }),
			wantErr:  errLicenseFileProduct,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publicKey := issuer.verifyKey
			if tt.noPublicKey {
				publicKey = nil
			}
			if _, err := verifyLicenseFile(tt.contents, publicKey); !errors.Is(err, tt.wantErr) {
				t.Fatalf("verifyLicenseFile() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestEmbeddedLicenseFilePublicKey(t *testing.T) {
	if productionLicenseFilePublicKey() == nil {
		t.Fatal("keygen.pem must hold a PEM-encoded Ed25519 public key")
	}
}

func TestParseLicenseFilePublicKey(t *testing.T) {
	issuer := newTestLicenseFileIssuer("public key parsing")
	file := issuer.signDataset(testLicenseFileDataset())

	for _, publicKey := range []string{
		string(issuer.publicKeyPEM),
		"\n" + string(issuer.publicKeyPEM) + "\n",
		strings.ReplaceAll(string(issuer.publicKeyPEM), "\n", "\r\n"),
	} {
		if _, err := verifyLicenseFile(file, parseLicenseFilePublicKey([]byte(publicKey))); err != nil {
			t.Errorf("verifyLicenseFile() with public key %q error = %v", publicKey, err)
		}
	}
	for _, publicKey := range []string{
		"",
		issuer.publicKey,
		strings.ReplaceAll(string(issuer.publicKeyPEM), "PUBLIC KEY", "PRIVATE KEY"),
	} {
		if key := parseLicenseFilePublicKey([]byte(publicKey)); key != nil {
			t.Errorf("parseLicenseFilePublicKey(%q) = %x, want nothing", publicKey, key)
		}
	}
}

func TestVerifyLicenseFileExpiry(t *testing.T) {
	issuer := newTestLicenseFileIssuer("expiry")
	licenseExpiry := mustParseTime(testLicenseFileExpiry)

	tests := []struct {
		name          string
		licenseExpiry any
		fileExpiry    any
		ttl           any
		issued        string
		want          time.Time
	}{
		{
			name:          "license expiry only",
			licenseExpiry: testLicenseFileExpiry,
			want:          licenseExpiry,
		},
		{
			name:          "file expires first",
			licenseExpiry: testLicenseFileExpiry,
			fileExpiry:    "2026-12-13T00:00:00.000Z",
			ttl:           2592000,
			want:          mustParseTime("2026-12-13T00:00:00.000Z"),
		},
		{
			name:          "license expires first",
			licenseExpiry: testLicenseFileExpiry,
			fileExpiry:    "2028-11-13T00:00:00.000Z",
			ttl:           63072000,
			want:          licenseExpiry,
		},
		{
			name:       "license never expires, file does",
			fileExpiry: "2026-12-13T05:29:06.000-05:00",
			ttl:        2592000,
			want:       mustParseTime("2026-12-13T10:29:06.000Z"),
		},
		{
			// keygen-go reports these two against the host's clock; the provider's clock decides instead.
			name:          "file already expired",
			licenseExpiry: testLicenseFileExpiry,
			fileExpiry:    "2020-01-02T00:00:00.000Z",
			ttl:           86400,
			issued:        "2020-01-01T00:00:00.000Z",
			want:          mustParseTime("2020-01-02T00:00:00.000Z"),
		},
		{
			name:          "issued in the future",
			licenseExpiry: "2999-11-13T00:00:00.000Z",
			issued:        "2999-01-01T00:00:00.000Z",
			want:          mustParseTime("2999-11-13T00:00:00.000Z"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataset := testLicenseFileDataset()
			setAttribute("expiry", tt.licenseExpiry)(dataset)
			setMeta("expiry", tt.fileExpiry)(dataset)
			setMeta("ttl", tt.ttl)(dataset)
			if tt.issued != "" {
				setMeta("issued", tt.issued)(dataset)
			}

			got, err := verifyLicenseFile(issuer.signDataset(dataset), issuer.verifyKey)
			if err != nil {
				t.Fatalf("verifyLicenseFile() error = %v", err)
			}
			if got.Expiry == nil || !got.Expiry.Equal(tt.want) {
				t.Fatalf("Expiry = %v, want %v", got.Expiry, tt.want)
			}
			if got.expired(tt.want.Add(-time.Nanosecond)) || !got.expired(tt.want) {
				t.Fatalf("expired() doesn't flip at %v", tt.want)
			}
		})
	}
}

// A license with no expiry, checked out without a TTL, gives a file that never expires.
func TestVerifyLicenseFileWithoutExpiry(t *testing.T) {
	issuer := newTestLicenseFileIssuer("no expiry")
	dataset := testLicenseFileDataset()
	setAttribute("expiry", nil)(dataset)

	got, err := verifyLicenseFile(issuer.signDataset(dataset), issuer.verifyKey)
	if err != nil {
		t.Fatalf("verifyLicenseFile() error = %v", err)
	}
	if got.Expiry != nil || got.expired(time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Expiry = %v, want a file that never expires", got.Expiry)
	}
}

// FuzzVerifyLicenseFile checks that no input makes verification panic, since keygen-go panics on some, and that a
// refusal is always a *FileError, whose message is safe to show.
func FuzzVerifyLicenseFile(f *testing.F) {
	issuer := newTestLicenseFileIssuer("fuzz")
	dataset := mustMarshalJSON(testLicenseFileDataset())
	f.Add(issuer.sign(dataset))
	f.Add(armorTestLicenseFile([]byte("null")))
	f.Add(armorTestLicenseFile([]byte(`{"enc":"","sig":"","alg":""}`)))
	f.Add(licenseFileBeginArmor)

	f.Fuzz(func(t *testing.T, contents string) {
		if _, err := verifyLicenseFile(contents, issuer.verifyKey); err != nil {
			if _, ok := errors.AsType[*FileError](err); !ok {
				t.Fatalf("error = %v, want a *FileError", err)
			}
		}
	})
}
