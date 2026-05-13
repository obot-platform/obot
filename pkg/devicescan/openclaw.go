package devicescan

// openclaw is presence-only: OpenClaw has no public config or plugin
// format we scan today. Presence is signalled by the `openclaw` binary
// in $PATH or the macOS app bundle.
var openclaw = client{
	name:      "openclaw",
	binaries:  []string{"openclaw"},
	appBundle: "OpenClaw.app",
}
