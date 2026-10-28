# Backend API messages

Locales are appropriately named JSON files such as `en.json`, `ja.json`, `ko.json`, and `zh-CN.json`. The Go server embeds every locale file under this directory and merges them into one catalog per locale. Keys must stay unique across files.

Shared messages used across the backend stay in the root locale files. Domain messages live in subdirectories:

- `auth-providers/` — auth provider configuration, local accounts, and setup
- `model-providers/` — model provider configuration
- `scim/` — SCIM setup and enforcement

API responses select a supported locale from `Accept-Language` and fall back to English when a translation is unavailable. Group names and raw downstream failure details are inserted as values rather than translated.

New locales are discovered automatically from BCP 47 filenames, such as `fr.json` or `pt-BR.json`, anywhere under this directory. Locale spelling is normalized while loading. Adding a catalog requires rebuilding the Go server because catalogs are embedded in the binary.

Translations can be partial: missing or empty messages fall back to English. English must supply every message key; supplied translations must use the same placeholders as their English source. No locale file is required for an untranslated area.

Run `make lint-i18n` to check constant keys passed to the Go `i18n.Text`,
`i18n.Message`, and `i18n.Locales` helpers, plus `${key}` references in constant
`i18n.Resolve` arguments. Referenced keys must have non-empty translations in
every discovered locale, even when the call requests English. The check uses
Go's `go/analysis` framework and runs through `make lint` and
`make validate-go-code` in CI. The Makefile builds the analyzer and runs it
with Go's vet driver:

```sh
(cd tools/i18n-lint && go build -o ../../bin/i18n-lint .)
go vet -vettool="$(pwd)/bin/i18n-lint" ./...
```

It respects Go package selection and build tags, recognizes import aliases,
dot imports, named constants, and constant string expressions, and reports the
source location and missing locale. To check another build configuration, pass
`-tags` or set `GOOS`/`GOARCH` as with other Go tools. Runtime keys, function
aliases, wrapper calls, and references stored in provider data are outside its
scope. Tests in `pkg/i18n` are excluded because they deliberately exercise
unknown keys and partial catalogs; other package tests are checked.

The command also supports running its analysis driver directly:

```sh
bin/i18n-lint ./...
```

Catalogs are read from the enclosing module's `pkg/i18n/messages` directory.
Use `-root=/absolute/path/to/obot` to override the repository root. Missing
translations are reported without automatic fixes.

The analyzer has its own Go module under `tools/i18n-lint`. Run its tests with
`(cd tools/i18n-lint && go test ./...)`; `make test` also includes them.

Provider display fields use `${key}` references to messages in `auth-providers/`
or `model-providers/`. The API resolves these references once using the selected
backend locale, falling back to English for missing or empty translations.
Unknown references remain unchanged; literal text is preserved. Inline provider
and configuration-parameter `locales` maps are no longer supported. Resource IDs
and configuration environment variable names remain literal.

Provider registry image changes must ship with the corresponding embedded
catalogs in Obot. Adding or changing translations requires rebuilding the server.
