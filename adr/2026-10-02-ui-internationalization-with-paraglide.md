# 2026-10-02: Internationalize the UI with Paraglide JS

- **Status:** Accepted
- **Date:** 2026-10-02
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None.

## Related ODPs

None.

## Context

The SvelteKit UI in `ui/user` hardcoded every user-facing string in English. We
wanted to support Japanese, Korean, and Simplified Chinese. The UI is built as a
static SPA (`adapter-static` with a fallback page) and already rewrites several
URL prefixes in `src/hooks.ts`, so locale-prefixed routes (`/ja/...`) would conflict
with existing routing and deep links.

## Decision

Use [Paraglide JS](https://inlang.com/m/gerre34r/library-inlang-paraglideJs), the
i18n library supported by SvelteKit's `sv add` tooling.

- Messages live in `ui/user/messages/{locale}.json` using the inlang message
  format. `en` is the base locale; `ja`, `ko`, and `zh-CN` are supported.
- The Vite plugin compiles messages into typed, tree-shakable functions under
  `src/lib/paraglide` (generated and git-ignored). `pnpm run check` compiles them
  first through the `i18n:compile` script so type checking works without a dev
  server.
- The locale is resolved per browser with the strategy
  `localStorage` → `preferredLanguage` → `baseLocale`. URLs are not localized.
- Components import `m` from `$lib/i18n`. The language picker in the profile
  menu calls `setLocale`, which saves the choice and reloads the page.
- The inlang message-format plugin is loaded from `node_modules`, not from a CDN,
  so builds do not need network access.

## Rationale

Paraglide compiles messages at build time. That gives type-checked keys and
parameters and ships only the messages a page uses. `svelte-i18n` was the main
alternative. It is store-based, untyped, and predates Svelte 5 runes. Keeping the
locale out of the URL avoids changing routing, `reroute` rules, OAuth redirect
URLs, and existing links.

## Consequences

- New user-facing strings should be added to `messages/en.json` and every other
  locale file, and rendered with `m.<key>()`. A string added only to `en.json`
  falls back to English in the other locales.
- Changing the locale reloads the page, which matches how most users switch
  languages.
- The locale is a per-browser preference and is not stored on the user's profile.
- Each feature area has its own catalog in `messages/<area>/` with keys
  prefixed by the area name, so changes in different areas don't touch the same
  files. A test checks that every locale has the same keys and placeholders, and
  that no key is defined in two catalogs.
- Values that tables filter and sort on (for example MCP server status, type,
  and registry) stay in English in the row data, because filters are saved in
  URLs. `Table`'s `displayValue` hook translates them only for display.
- Dates and times are formatted with the app locale, not the browser's.
- Legal pages (terms of service, privacy policy), server-provided text, user
  data, code samples, and text sent to models stay in English.
- The initial `ja`, `ko`, and `zh-CN` translations were machine-written and need
  review by native speakers.

## References

- `ui/user/project.inlang/settings.json`
- `ui/user/src/lib/i18n.ts`
- `ui/user/src/lib/components/LanguageSelect.svelte`
