package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

// TestProviderManifestFieldsReadFromYAML checks that every field of a provider manifest is read
// from a registry file under its JSON name. Registry files are YAML, which is decoded with yaml
// struct tags and ignores json tags, so a field without a matching yaml tag is silently dropped.
func TestProviderManifestFieldsReadFromYAML(t *testing.T) {
	t.Run("auth provider", func(t *testing.T) {
		testManifestFieldsReadFromYAML[types.AuthProviderManifest](t)
	})
	t.Run("model provider", func(t *testing.T) {
		testManifestFieldsReadFromYAML[types.ModelProviderManifest](t)
	})
}

func testManifestFieldsReadFromYAML[T types.ModelProviderManifest | types.AuthProviderManifest](t *testing.T) {
	t.Helper()

	var want T
	fillFields(t, reflect.ValueOf(&want).Elem(), "")

	// JSON is valid YAML, and its keys are the names the manifest fields are documented and written under.
	content, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "provider.yaml"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	providers, err := readProviderDirectory[T](dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}

	if got := providers[0].Manifest; !reflect.DeepEqual(got, want) {
		t.Fatalf("manifest fields were lost reading YAML; check that each field's yaml tag matches its json tag\nwant: %+v\ngot:  %+v", want, got)
	}
}

// fillFields sets every field reachable from v to a non-zero value, so that a field the decoder
// drops shows up as a difference.
func fillFields(t *testing.T, v reflect.Value, path string) {
	t.Helper()

	switch v.Kind() {
	case reflect.Struct:
		for i := range v.NumField() {
			fillFields(t, v.Field(i), path+"."+v.Type().Field(i).Name)
		}
	case reflect.String:
		v.SetString("value" + path)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fillFields(t, s.Index(0), fmt.Sprintf("%s[0]", path))
		v.Set(s)
	default:
		t.Fatalf("fillFields: unsupported kind %s at %s; extend fillFields to cover it", v.Kind(), path)
	}
}
