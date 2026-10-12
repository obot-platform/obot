# Backend API messages

Locales are appropriately named JSON files such as `en.json`, `ja.json`, `ko.json`, and `zh-CN.json`. The Go server embeds every locale file under this directory and merges them into one catalog per locale. Keys must stay unique across files.

Shared messages used across the backend stay in the root locale files. Domain messages live in subdirectories:

- `auth-providers/` — auth provider configuration, local accounts, and setup
- `model-providers/` — model provider configuration
- `scim/` — SCIM setup and enforcement

API responses select a supported locale from `Accept-Language` and fall back to English when a translation is unavailable. Product names, stored provider names, group names, and raw downstream failure details are inserted as values rather than translated.

New locales are discovered automatically from BCP 47 filenames, such as `fr.json` or `pt-BR.json`, anywhere under this directory. Locale spelling is normalized while loading. Adding a catalog requires rebuilding the Go server because catalogs are embedded in the binary.

Translations can be partial: missing or empty messages fall back to English. English must supply every message key; supplied translations must use the same placeholders as their English source. No locale file is required for an untranslated area.

Provider YAML `locales` maps and configuration-parameter `locales` maps also accept additional locales without backend code changes. Provider responses select one language from backend catalogs and the translations available on the providers being returned. Fields missing that language retain their English text. YAML-only locales do not require a backend catalog and become available through the existing provider registry reload flow. The frontend language picker is configured separately.
