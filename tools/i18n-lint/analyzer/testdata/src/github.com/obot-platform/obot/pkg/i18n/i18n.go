package i18n

func Text(locale, key string) string                            { return "" }
func Message(locale, key string, args map[string]string) string { return "" }
func Locales(key string) map[string]string                      { return nil }
func Resolve(locale, text string) string                        { return "" }
func Locale(locale string) string                               { return "" }

const Key = "known"

var _ = Locales("missing") // want `message "missing" is missing or empty in ja`
