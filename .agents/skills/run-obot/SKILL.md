---
name: run-obot
description: Build, launch, and drive the Obot server and SvelteKit UI locally to see a change working at runtime, on isolated ports with a throwaway SQLite database, optionally driven with Playwright. Use when asked to run, start, or launch Obot, reproduce a bug in the running app, screenshot the UI, check a change in the browser, or when startup hangs. Covers the model provider registry and the OPENAI_API_KEY / ANTHROPIC_API_KEY startup hang.
---

# Running Obot locally

Humans usually run `make dev` (server on 8080, UI on 5174, `obot.db` in the repo root). For verifying a change,
prefer the isolated launch below: it doesn't collide with a running `make dev` or a container on 8080, and it
starts from a fresh database every time.

Server configuration is all environment variables, documented in
`docs/docs/configuration/server-configuration.md`. Check there before inventing flags.

## Model providers and the API key startup hang

Model and auth providers come from [obot-platform/providers](https://github.com/obot-platform/providers) and are
loaded from the local directories listed in `OBOT_SERVER_PROVIDER_REGISTRIES`. The Docker image sets this; `make dev`
and a bare `obot server` do not, so by default no providers exist.

If `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` is set and no registry is configured, **startup hangs**: `PostStart` in
`pkg/controller/controller.go` waits forever for the matching model provider (`ensureModelProviderCredAndDefaults`
in `pkg/controller/handlers/provider/provider.go`), so the default MCP catalog and everything after it is never set
up. `curl 'localhost:18080/debug/pprof/goroutine?debug=2'` shows where startup is stuck.

Pick one:

- **No model providers** (most UI and API changes): unset both keys, as in the launch below.
- **With model providers** (anything that needs an LLM, such as chat or the LLM gateway): clone the providers repo,
  run `make build` in it to build the provider binaries, and add
  `OBOT_SERVER_PROVIDER_REGISTRIES=<path-to-providers-clone>` to the server's environment. Keeping the keys set then
  configures the OpenAI/Anthropic providers with them. See "Developing Obot Providers" in `DEVELOPMENT.md`.

## Launch (isolated)

Shell variables don't survive between separate tool calls, so every block below recomputes `REPO` and `RUN` from
fixed locations instead of relying on an earlier one. Processes started with `&` keep running after the call that
started them returns.

```bash
REPO=$(git rev-parse --show-toplevel)
RUN=/tmp/run-obot
rm -rf "$RUN" && mkdir -p "$RUN"
go build -o "$RUN/obot" "$REPO"
(cd "$REPO/ui/user" && pnpm install)   # needs the Node version in ui/user/package.json "engines"

# Run from $RUN: the default DSN is obot.db in the working directory (a fresh install every time), and the
# server also writes apiserver.local.config/ there. --dev-mode puts storage on HTTP port + 1 (18081); the
# checked-in tools/devmode-kubeconfig points at `make dev`'s 8443, not this instance, so don't use it here.
# `exec` makes the saved PID the server's own.
(cd "$RUN" && exec env -u ANTHROPIC_API_KEY -u OPENAI_API_KEY \
  OBOT_SERVER_PRODUCT_ANALYTICS_MODE=off \
  OBOT_SERVER_DISABLE_UPDATE_CHECK=true \
  ./obot server --dev-mode --http-listen-port 18080 --dev-uiport 15174 > server.log 2>&1) &
echo $! > "$RUN/server.pid"

# SvelteKit must run from ui/user, or it fails and writes .svelte-kit/ into the repo root.
(cd "$REPO/ui/user" && exec env VITE_API_TARGET=http://localhost:18080 \
  ./node_modules/.bin/vite dev --port 15174 --strictPort > "$RUN/ui.log" 2>&1) &
echo $! > "$RUN/ui.pid"
```

Browse `http://localhost:18080/`; the server proxies the UI from the Vite port. With authentication disabled (the
default) you are the owner.

Startup is done when the default catalog exists. Wait for it before seeding or driving the UI:

```bash
until curl -sf localhost:18080/api/mcp-catalogs/default | grep -q '"id"'; do sleep 2; done
```

If that never succeeds, check `/tmp/run-obot/server.log` and the API key section above.

Not everything is isolated: some data (e.g. published artifacts) still goes under `~/.local/share/obot`.

## Fresh-install state

A fresh database shows a "Welcome to Obot!" dialog (EULA, plus analytics consent) that intercepts clicks. There
is no environment variable for the EULA, but with `OBOT_SERVER_PRODUCT_ANALYTICS_MODE=off` (set above) accepting it
through the API is enough to keep the dialog away:

```bash
curl -X PUT localhost:18080/api/eula -d '{"accepted":true}'
```

When you need the dialog itself gone but the EULA unaccepted, set `localStorage.seenSplashDialog` to an ISO timestamp
before navigating (e.g. Playwright `context.addInitScript`).

## Seeding data

Create catalog entries through the API rather than the UI:

```bash
curl -X POST localhost:18080/api/mcp-catalogs/default/entries \
  -d '{"name":"example","runtime":"remote","remoteConfig":{"fixedURL":"https://example.com/mcp"}}'
# or "runtime":"npx","npxConfig":{"package":"<npm package>"}
```

An entry's page is `/mcp-servers/c/<id>` (tabs via `?view=...`, e.g. `?view=troubleshooting`).

## Driving the UI with Playwright

`ui/user` already depends on Playwright. Use it from a script outside the repo (e.g. `/tmp/run-obot/drive.mjs`) so
nothing is left in the tree, and pass the repo path in:

```js
import { createRequire } from 'node:module';
const { chromium } = createRequire(`${process.env.REPO}/ui/user/package.json`)('playwright');
// Use Playwright's own browser, or point at an installed one: chromium.launch({ executablePath: '/usr/bin/chromium' })
const browser = await chromium.launch();
```

```bash
REPO=$(git rev-parse --show-toplevel) node /tmp/run-obot/drive.mjs
```

If launch fails because no browser is installed, run `(cd "$(git rev-parse --show-toplevel)/ui/user" && pnpm exec playwright install chromium)`
or pass `executablePath` for a system Chromium.

- Collect `pageerror` and console `error` events, and treat any as a failure to investigate.
- Screenshot the pages you changed and look at them.
- For translation work: the language picker is under the profile button (`#btn-navbar-profile`); the choice is saved
  in `localStorage.PARAGLIDE_LOCALE`. A browser context with e.g. `locale: 'ko-KR'` exercises automatic detection.
  Scan `document.body.innerText` for raw message keys to catch missing translations.

## Cleanup

Stop both processes when done, or the next run fails on `--strictPort`:

```bash
RUN=/tmp/run-obot
kill "$(cat "$RUN/server.pid")" "$(cat "$RUN/ui.pid")"
rm -rf "$RUN"
```
