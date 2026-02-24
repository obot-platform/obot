package handlers

import "testing"

func TestVersionResponseIncludesDisableLegacyChat(t *testing.T) {
	t.Setenv("OBOT_SERVER_VERSIONS", "")

	v := &VersionHandler{
		disableLegacyChat:  true,
		nanobotIntegration: true,
	}

	values := v.getVersionResponse()

	disableLegacyChat, ok := values["disableLegacyChat"].(bool)
	if !ok {
		t.Fatalf("expected disableLegacyChat to be a bool, got %T", values["disableLegacyChat"])
	}
	if !disableLegacyChat {
		t.Fatalf("expected disableLegacyChat to be true")
	}
}

func TestVersionResponseIncludesNanobotIntegration(t *testing.T) {
	t.Setenv("OBOT_SERVER_VERSIONS", "")

	v := &VersionHandler{
		disableLegacyChat:  false,
		nanobotIntegration: false,
	}

	values := v.getVersionResponse()

	nanobotIntegration, ok := values["nanobotIntegration"].(bool)
	if !ok {
		t.Fatalf("expected nanobotIntegration to be a bool, got %T", values["nanobotIntegration"])
	}
	if nanobotIntegration {
		t.Fatalf("expected nanobotIntegration to be false")
	}
}
