# Backend API messages

Locales are appropriately named JSON files such as `en.json`, `ja.json`, `ko.json`, and `zh-CN.json`. The Go server embeds every locale file under this directory and merges them into one catalog per locale. Keys must stay unique across files.

Shared messages used across the backend stay in the root locale files. Domain messages live in subdirectories:

- `auth-providers/` — auth provider configuration, local accounts, and setup
- `model-providers/` — model provider configuration
- `scim/` — SCIM setup and enforcement

API responses select a supported locale from `Accept-Language` and fall back to English when a translation is unavailable. Product names, stored provider names, group names, and raw downstream failure details are inserted as values rather than translated.
