package handlers

import (
	"testing"
)

func TestVersionFeatureValuesIncludesHostedAgentsState(t *testing.T) {
	disabled := (&VersionHandler{}).featureValues()
	if enabled := disabled["hostedAgentsEnabled"]; enabled {
		t.Fatal("expected Hosted Agents to be disabled")
	}

	handler := &VersionHandler{}
	handler.HostedAgentsEnabled = true
	enabled := handler.featureValues()
	if hostedAgentsEnabled := enabled["hostedAgentsEnabled"]; !hostedAgentsEnabled {
		t.Fatal("expected Hosted Agents to be enabled")
	}
}
