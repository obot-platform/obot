package types

import "fmt"

// Harness is the runtime a hosted agent is built on (for example "Claude
// Code", "Codex", or a custom Python/Node image). The harness supplies the
// docker image; the agent supplies configuration.
type Harness struct {
	Metadata        `json:",inline"`
	HarnessManifest `json:",inline"`
}

type HarnessManifest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	IconDark    string `json:"iconDark,omitempty"`
	Image       string `json:"image,omitempty"`

	// Interactive runs the harness with a TTY and an open stdin, the equivalent
	// of `docker run -it`. Images whose entrypoint is a shell exit immediately
	// without one, so a harness built around an interactive shell rather than a
	// long-running server needs this to stay up.
	Interactive bool `json:"interactive,omitempty"`
}

func (m HarnessManifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Image == "" {
		return fmt.Errorf("image is required")
	}
	return nil
}

type HarnessList List[Harness]
