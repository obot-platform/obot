package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilterV1ContractFixtures(t *testing.T) {
	paths, err := filepath.Glob("testdata/filter/v1/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no Filter contract fixtures found")
	}

	for _, path := range paths {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(name, "response-") {
				var response FilterResponse
				if err := json.Unmarshal(data, &response); err != nil {
					t.Fatal(err)
				}
				if err := response.Validate(FilterCapabilities{CanReject: true, CanMutate: true}); err != nil {
					t.Fatal(err)
				}
				assertEquivalentJSON(t, data, response)
				return
			}

			var request FilterRequest
			if err := json.Unmarshal(data, &request); err != nil {
				t.Fatal(err)
			}
			if err := request.Validate(); err != nil {
				t.Fatal(err)
			}
			assertEquivalentJSON(t, data, request)
		})
	}
}

func assertEquivalentJSON(t *testing.T, expected []byte, actual any) {
	t.Helper()
	actualData, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	var expectedValue, actualValue any
	if err := json.Unmarshal(expected, &expectedValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(actualData, &actualValue); err != nil {
		t.Fatal(err)
	}
	expectedData, _ := json.Marshal(expectedValue)
	actualData, _ = json.Marshal(actualValue)
	if string(expectedData) != string(actualData) {
		t.Fatalf("JSON mismatch\nexpected: %s\nactual:   %s", expectedData, actualData)
	}
}

func TestFilterToolRequestUsesSingleRequestArgument(t *testing.T) {
	wrapper := FilterToolRequest{Request: FilterRequest{
		APIVersion: FilterAPIVersionV1,
		Source:     FilterSourceMCP,
		Event:      FilterEvent{Type: FilterEventTypeMCPMessage, Phase: FilterPhaseRequest},
		Payload:    json.RawMessage(`{}`),
	}}
	data, err := json.Marshal(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 || len(fields["request"]) == 0 {
		t.Fatalf("tool arguments = %s, want only request", data)
	}
}

func TestDeviceResourceRemainsInvalidForAccessControl(t *testing.T) {
	if err := (Resource{Type: ResourceTypeDevice, ID: "*"}).Validate(); err == nil {
		t.Fatal("device resource must remain invalid outside Filter-manifest validation")
	}
}
