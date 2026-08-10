# UI Testing Guidelines for AI Agents

When adding or changing UI features under `ui/user/src`, create or update Vitest browser tests so behavior stays covered. Prefer adding new specs over rewriting existing ones.

## When to add tests

Add a colocated `*.svelte.spec.ts` when you introduce or meaningfully change:

- A SvelteKit route (`+page.svelte`, especially admin or catalog flows)
- A reusable component with user-visible behavior, validation, or API side effects
- Role/entitlement-gated UI, dialogs, forms, or menu actions

Skip trivial presentational tweaks with no behavior change.

## File naming and placement

| Target                 | Spec file             | Location                        |
| ---------------------- | --------------------- | ------------------------------- |
| Route `+page.svelte`   | `page.svelte.spec.ts` | Same directory as the page      |
| Component `Foo.svelte` | `Foo.svelte.spec.ts`  | Same directory as the component |

Vitest picks up `src/**/*.svelte.{test,spec}.{js,ts}` in the browser (client) project. Do not invent a parallel `__tests__` tree for these.

Non-Svelte unit tests use `*.{test,spec}.{js,ts}` (server/node project). Prefer browser Svelte specs for UI work. SSR specs (`*.ssr.{test,spec}.{js,ts}`) are excluded from both projects today — do not add them unless the Vite config is updated.

## Unbreakable rules (vitest-browser-svelte)

1. **Always use locators** — `page.getByRole` / `getByLabelText` / `getByText` / `getByCSS`. Never `container.querySelector`, `document.querySelector`, or `document.getElementById` for find/assert/click.
2. **Always `await expect.element(...)`** for locator assertions (auto-retry). Plain `expect(...)` is fine for mocks, spies, and non-DOM values.
3. **Handle strict mode** — if a locator matches multiple nodes, use `.first()`, `.nth(n)`, or `.last()` (or narrow with `.filter({ hasText })`).
4. **Use `untrack()`** when reading Svelte 5 `$derived` values from a component instance in tests.
5. **Prefer accessible queries** — role/label/text first; `page.getByCSS` only when there is no accessible name (see `CatalogServerForm.svelte.spec.ts`).
6. **SvelteKit progressive-enhancement forms** — do not click native `<form method>` submit buttons (triggers navigation / hangs). Custom forms that call an `onSubmit` callback (no full navigation) may click submit; assert validation and callback outcomes.
7. **Focus on user-visible behavior** — avoid brittle implementation details (exact SVG paths, incidental markup).
8. **Layout `children`** — use `createRawSnippet` when a component requires `children`; do not pass raw HTML strings as children props.
9. **Overlays / off-viewport clicks** — prefer locator `.click()`. If driver.js or similar intercepts Playwright actionability, resolve with a locator then call native `HTMLElement.click()` (see `Layout.svelte.spec.ts`). Document why in a short comment.

Optional: for large new surfaces, sketch coverage with `it.skip` / `describe` blocks first (foundation-first), then implement incrementally.

## Reference existing specs first

Before writing a new test, open the closest matching spec and mirror its structure:

| Pattern                                             | Reference                                                                                                   |
| --------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| Admin route + `preparePageData` + form/dialog flows | `routes/admin/license/page.svelte.spec.ts`, `routes/admin/auth-providers/page.svelte.spec.ts`               |
| Route that loads MCP data via MSW + store setup     | `routes/admin/mcp-deployments/page.svelte.spec.ts`, `routes/mcp-catalog/s/[id]/details/page.svelte.spec.ts` |
| Shared layout / role-based navigation               | `lib/components/Layout.svelte.spec.ts`                                                                      |
| Form component + validation + `worker.use`          | `lib/components/admin/CatalogServerForm.svelte.spec.ts`                                                     |

Copy local helpers (`renderXPage`, `mockXApis`, fixture builders) from those files rather than inventing new harness styles.

## Shared test infrastructure (`src/tests`)

Use these instead of ad-hoc setup:

| Path                        | Use for                                                                                                     |
| --------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `tests/vitest-setup.ts`     | Global MSW lifecycle, `localStorage` clear, `page.getByCSS` — already wired via Vite; do not duplicate      |
| `tests/helpers/pageData.ts` | `preparePageData`, `createMockProfile`, `createPageData` for route renders that need layout `data` / stores |
| `tests/helpers/mcp.ts`      | MCP entry/server fixture builders                                                                           |
| `tests/mocks/data.ts`       | Shared API response fixtures; extend here when many specs need the same payload                             |
| `tests/mocks/handlers.ts`   | Default MSW handlers                                                                                        |
| `tests/mocks/worker.ts`     | `worker` for per-test `worker.use(...)` overrides                                                           |

Typical route test shape:

```ts
import { preparePageData } from '../../../tests/helpers/pageData';
// adjust relative depth
import { worker } from '../../../tests/mocks/worker';
import type { PageData } from './$types';
import MyPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

async function renderMyPage(overrides = {}) {
	const data = await preparePageData<PageData>(overrides);
	return render(MyPage, { data });
}
```

For API interactions under test, override with `worker.use(http.get/post/delete(...))` and assert with `vi.fn` / `vi.waitFor`. Handlers reset automatically after each test. Prefer MSW + real `fetch` over mocking browser APIs.

## Writing style

- Assert both positive and negative UI states when branching (license, role, bootstrap user, etc.)
- Extract small local helpers for repeated click/fill/open-menu steps
- Browser tests time out quickly (`testTimeout: 2000`); avoid unnecessary waits and flaky polling
- Animated / overlay controls may need `{ force: true }` on click/fill when the default action is blocked

## Commands

From `ui/user/`:

```bash
pnpm test                 # vitest run --browser.headless
pnpm exec vitest run path/to/file.svelte.spec.ts
pnpm exec playwright install chromium   # if browser binaries are missing
```

Run the relevant spec (or full suite) after adding or changing tests.

## Modifying existing tests — regression check first

Do **not** edit an existing `*.svelte.spec.ts` (or shared helpers/mocks used by multiple specs) until you have verified current behavior:

1. Run the existing spec(s) that cover the area you will change and confirm they pass (or note pre-existing failures).
2. Prefer adding new `it(...)` / `describe(...)` cases for new behavior instead of rewriting assertions that still describe intended behavior.
3. Only change an existing assertion when product behavior intentionally changed; update the assertion to the new contract and keep neighboring cases green.
4. If you must change shared fixtures in `tests/mocks/data.ts` or helpers in `tests/helpers/`, re-run every spec that imports them before finishing.
5. Never weaken or delete coverage to make a failing change pass without an explicit product reason.

If a change would force broad rewrites of unrelated specs, stop and narrow the implementation or fixtures so existing tests remain valid.
