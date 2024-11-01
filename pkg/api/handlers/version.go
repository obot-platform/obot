package handlers

import (
	"os"
	"strings"

	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/version"
)

type versionResponse struct {
	Otto              string `json:"otto,omitempty"`
	Tools             string `json:"tools,omitempty"`
	WorkspaceProvider string `json:"workspaceProvider,omitempty"`
}

func GetVersion(req api.Context) error {
	return req.Write(getVersionResponse())
}

func getVersionResponse() *versionResponse {
	// Retrieve the multi-line environment variable
	envVar := os.Getenv("OTTO_SERVER_VERSIONS")
	// Initialize a map to store the parsed key-value pairs
	values := make(map[string]string)

	// Parse the environment variable into the map
	for _, line := range strings.Split(envVar, "\n") {
		if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
			values[parts[0]] = parts[1]
		}
	}

	// Populate the versionResponse struct using the map
	return &versionResponse{
		Otto:              version.Get().String(),
		Tools:             strings.TrimSpace(values["Tools"]),
		WorkspaceProvider: strings.TrimSpace(values["WorkspaceProvider"]),
	}
}
