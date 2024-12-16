package textio

import "github.com/pterm/pterm"

func Info(text string) {
	pterm.Info.Println(text)
}

func Print(text string) {
	pterm.DefaultParagraph.WithMaxWidth(80).Println(text)
}

func Ask(text, def string) (string, error) {
	return pterm.DefaultInteractiveTextInput.
		WithDefaultValue(def).Show(text)
}

func Select(string, map[string]string, string) (string, error) {
	return "", nil
}
