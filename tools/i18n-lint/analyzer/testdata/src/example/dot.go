package example

import . "github.com/obot-platform/obot/pkg/i18n"

func dot() {
	Text("en", "known")
	Locales("empty") // want `message "empty" is missing or empty in ja`
	Locales := func(string) {}
	Locales("ignored")
}
