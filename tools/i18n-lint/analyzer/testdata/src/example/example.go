package example

import tr "github.com/obot-platform/obot/pkg/i18n"

const key = "known"
const unknown = "unknown"

type other struct{}

func (other) Locales(string) {}
func check(dynamic string) {
	tr.Text("en", key)
	tr.Text("en", tr.Key)
	tr.Text("en", ("joined_" + "key"))
	tr.Message("en", "known", nil)
	tr.Locales("known")
	tr.Resolve("en", "prefix ${known} plain {ignored}")
	tr.Resolve("en", "plain text")
	tr.Text("en", dynamic)
	tr.Locale("ignored")
	alias := tr.Locales
	alias("ignored")
	tr.Text("en", unknown)                      // want `message "unknown" is missing or empty in en` `message "unknown" is missing or empty in ja`
	tr.Message("en", "empty", nil)              // want `message "empty" is missing or empty in ja`
	tr.Locales("missing")                       // want `message "missing" is missing or empty in ja`
	tr.Resolve("en", "${empty} and ${missing}") // want `message "empty" is missing or empty in ja` `message "missing" is missing or empty in ja`
	{
		tr := other{}
		tr.Locales("ignored")
	}
}
