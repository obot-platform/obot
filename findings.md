# Review: initial local owner setup link and forced password change

Review of the uncommitted working tree against the request: provision an initial owner from
environment variables, drop the bootstrap token from the provisioning email, and force a password
change on first login.

The overall shape is good. Binding a hashed, expiring, single purpose setup secret to one local
account, keeping the secret in the URL fragment, and restricting the resulting session to the
password change flow is the right design for this problem. It is better than the obvious approach
of passing a plaintext owner password in an environment variable. The ADR is clear and honest
about the tradeoffs. Most of what follows is about the wiring around that design rather than the
design itself.

## What I verified

These all pass:

- `go build ./...`
- `go test` for `pkg/localauth`, `pkg/gateway/client`, `pkg/api/server`, `pkg/api/authz`,
  `pkg/bootstrap`, `pkg/services`
- `golangci-lint` on every changed Go package. The single revive warning is pre-existing in
  `pkg/gateway/types/serviceaccount.go` and is not part of this change.
- `pnpm run check` reports 0 errors, `pnpm run build` succeeds, and prettier is clean on the
  changed files.
- `adapter-static` emits `activate.html` and `change-password.html`, so the server side 303 to
  `/change-password` resolves to a real page.
- Bootstrap really is disabled when the initial owner is configured. The struct field
  `authEnabled` gates `AuthenticateRequest`, `Login`, `Enabled`, and `SetupEnabled`, so the token
  cannot be used.
- The proxy does not cache auth state, so clearing `RequirePasswordChange` takes effect on the
  very next request instead of after the session expires.
- I simulated an upgrade of a deployment that already has local auth users. I dropped the three
  new columns, inserted a pre-upgrade row, re-ran `AutoMigrate`, and read the row back. On SQLite
  it reads fine and NULL becomes `false` and `""`. GORM handles NULL into non-pointer fields, so
  Postgres should behave the same, but it is worth one manual check.
- `Profile.loaded` is set by `getProfile`, so `change-password/+page.ts` cannot loop against the
  layout redirect.
- `redirectTarget()` already sanitizes `rd` on the server, and the change password page validates
  it again in the browser. No open redirect was introduced.
- The credential context used by `EnsureInitialOwner` matches what the admin configure handler
  writes (`Context: authProvider.Name`), so the provisioned credential is not shadowed by a later
  admin edit.

## Blocking

### 1. `pkg.env` contains what looks like a live GitHub token

`pkg.env` is a new untracked file in the repo root holding
`export PKG_API_KEY=ghp_...`. It has nothing to do with this feature. It must not be committed.
Rotate that token, delete the file or move it outside the repo, and add it to `.gitignore` so it
cannot be picked up by a future `git add -A`.

### 2. `/activate` is missing from `UNAUTHORIZED_PATHS`, which breaks the main flow

`ui/user/src/lib/constants.ts:4` lists the paths where an anonymous visitor is expected. `/activate`
is not in it.

Here is what happens when the owner clicks the emailed link:

1. The browser loads `/activate#token=...`.
2. The root layout load runs and calls `getProfile()`.
3. `GET /api/me` is in `types.GroupAuthenticated`, so an anonymous caller gets a 401
   (`pkg/api/server/server.go:216-221`).
4. `handle401Redirect()` runs (`ui/user/src/lib/services/http.ts:38-53`). `profile.current.loaded`
   is false, and `/activate` is not in `UNAUTHORIZED_PATHS`, so it sets
   `window.location.href = '/?rd=%2Factivate'`.
5. That navigation drops the URL fragment, which is the only copy of the setup token. It races
   against the page's own `onMount` handler, which is trying to exchange the token at the same
   moment.

So the feature will fail, or work intermittently depending on which finishes first, and when it
fails the token is gone from the browser and the user has to reopen the email. The comment already
in `constants.ts:9-11` describes this exact hazard for `/login/local`, so the fix is the same: add
`/activate` to the set. Please confirm the whole flow in a real browser afterward, because I
reviewed this by reading the code rather than by running the server.

### 3. The activate page can never show its own error message

`activateInitialLocalAuthOwner` calls `doPost` without `dontLogErrors`
(`ui/user/src/lib/services/user/operations.ts`), and the handler returns 401 for an invalid,
expired, or already completed link (`pkg/api/handlers/localauth.go`). A 401 from `doPost` runs the
same `handle401Redirect()` above and also pushes the error into the global toast store. The result
is that the user is bounced to `/` and sees a generic error notification, and the carefully written
"Ask the person who provisioned this environment to reissue the owner setup link" branch in
`activate/+page.svelte` is unreachable.

Two ways to fix it. Either return 400 instead of 401 from `Activate`, or pass
`dontLogErrors: true` and handle the status locally. Changing the status code leaks nothing here,
because all three failure modes already return one identical response.

## High

### 4. Startup hard fails once the customer configures their own identity provider

`pkg/services/config.go` returns an error from `services.New` when `GetConfiguredAuthProvider`
returns any provider other than local while the initial owner variables are still set:

```
cannot provision initial local auth owner while auth provider %q is configured
```

In the provisioning model you described, those variables are baked into the deployment and stay
there. The trial customer is expected to eventually connect their own identity provider. The next
time that pod restarts, Obot refuses to boot. That turns a normal restart into an outage, across
every environment that has moved to a real provider.

`EnsureInitialOwner` already treats a completed owner as "nothing to do" and returns nil. This case
should behave the same way. Log at info that provisioning is being skipped because another provider
is configured, and continue starting.

### 5. The forced password change screen shows error notifications

The new gate in `Server.Wrap` allows only static assets, `/change-password`, `GET /api/me`,
`POST /api/local-auth/change-password`, and `/oauth2/sign_out`. But the root layout load fetches
`/api/version`, `/api/license`, and `/api/app-preferences` before it fetches `/api/me`, and none of
those pass `dontLogErrors`. Each one gets a 403 from the gate, and `doGetForResponse` pushes each
failure into the global `errors` store. The first screen a new trial customer sees will carry
several error toasts.

The better fix is to add those three read-only endpoints to `passwordChangeRequestAllowed`, so the
app shell renders normally instead of falling back to defaults. Suppressing the logging would hide
the toasts but leave the page unbranded.

## Medium

### 6. Local users still cannot change their own password, but the docs no longer say so

`ChangePassword` rejects any caller that does not have `password_change_required` set to true. That
is a reasonable restriction for the forced flow, but it means there is still no way for a local
user to change their password voluntarily. The owner who completes setup cannot later rotate their
own password except by using the admin reset endpoint on themselves, which by default forces
another change.

The change to `docs/docs/configuration/auth-providers.md` removed the note that said local users
cannot change their own password and replaced it with text about forced changes. The limitation is
still real, so the docs now read as though the gap were closed. Either allow a voluntary change,
verifying the current password first, or put the limitation back in writing.

### 7. Provisioning silently does nothing if the email already has a local account

In `EnsureInitialOwner`, an existing user with an empty `SetupTokenHash` is treated as "setup
already completed" and the function returns nil. That is correct for a completed owner, but it also
covers the case where an administrator had previously created a normal local user with the same
email. The operator gets a clean startup and a setup link that will never work, with nothing in the
logs. Add a warning log on that branch so the situation is diagnosable.

### 8. Auto-configuring the Local provider to only the owner's domain is a surprising side effect

`EnsureInitialOwner` writes `OBOT_AUTH_PROVIDER_EMAIL_DOMAINS` set to the owner's own email domain.
It is documented, but the consequences are easy to miss. A trial provisioned for
`owner@orionconnected.com` can only ever create local users at that one domain, and the owner has
to find the auth provider settings to change it. If the owner email is a consumer address, the
value becomes `gmail.com`, which is probably not what anyone wanted either. For a trial flow,
consider defaulting to `*`, or expose the domain list as its own environment variable so the
provisioning system decides.

### 9. An administrator password reset does not disarm a pending setup link

`SetLocalAuthUserPassword` only clears `setup_token_hash` when `requirePasswordChange` is false.
The admin reset path defaults that flag to true, so resetting the password of an initial owner who
has not activated yet leaves the emailed setup link armed. Exploitability is low today, because
bootstrap login is disabled and there is usually no other administrator yet, but the safe behavior
is to clear the setup token on any administrator initiated `SetPassword`.

### 10. Blocked requests are not audit logged and drop refreshed cookies

The gate was placed early in `Server.Wrap`, before the audit `responseWriter` is installed and
before the `set-cookies` extra is replayed onto the response. So a restricted session that probes
other API routes produces no audit entries, and a session the provider just refreshed loses its
new cookie on any blocked request. Moving the gate below both of those blocks fixes it without
weakening the restriction.

## Low and nits

11. `bootstrap.Disabled()` expresses "bootstrap is off" by storing `authEnabled: false` on the
    struct. It works today because every gate reads that one field, but the field now says
    something untrue about whether authentication is enabled. A separate `disabled` field would be
    clearer and would not mislead the next person who reads `b.authEnabled`.

12. Setting `OBOT_BOOTSTRAP_TOKEN` together with the initial owner variables silently ignores the
    bootstrap token. Log a warning so the operator knows their value had no effect.

13. `isStaticAssetPath` matches only `/_app/` and `/user/images/`, so `/favicon.ico` and similar
    top level assets get a 303 to `/change-password` during a restricted session. Harmless, but it
    makes the network log confusing while debugging.

14. In `change-password/+page.svelte`, `resolve(redirectTarget() as \`/${string}\`)` casts a runtime
    string to a route id, which defeats the typing that `resolve` exists to provide.
    `goto(redirectTarget())` is enough, since `redirectTarget()` already validates the value.

15. `POST /api/local-auth/activate` is unauthenticated and the setup token lookup skips the login
    throttle that `login()` uses. The token has enough entropy that brute force is not realistic,
    so this is defense in depth rather than a hole, but a throttle on that route is cheap.

16. The ADR and docs ask for a secret with "at least 32 characters", and the code enforces exactly
    that with `len(setupToken) < 32`. Thirty-two repeated characters passes. Since the whole design
    rests on entropy rather than length, consider saying so and pointing at
    `openssl rand -hex 32`, which the docs already use in the example.

17. Test coverage is good where it exists. `pkg/gateway/client/localauth_test.go` covers resume,
    rotation, and revocation well, and `password_change_test.go` pins the allowlist. What is
    missing is `EnsureInitialOwner` itself, which holds most of the branching logic: rotation, the
    already completed no-op, domain rejection, and the concurrent create reconcile path. The
    `Activate` and `ChangePassword` handlers are untested, and there is nothing for the two new UI
    routes even though `ui/user/src/tests` exists. Findings 2 and 3 are both the kind of bug a
    single browser test of the activation flow would have caught.
