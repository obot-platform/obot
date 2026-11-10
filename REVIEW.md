# Review notes

- Clean up tables an auth provider owns (`<postgresTablePrefix>sessions`, `session_locks`, `directory_users`,
  `directory_memberships`) on every path that ends a provider's configuration, discarding a staged switch included,
  not only on deconfigure: a staged provider runs the verification sign-in and can write to them, and rows left
  behind reappear, as groups on the Groups page, when the provider is configured later. Drop them before deleting the
  credential that marks the staging, so that a retry after a failed drop still finds it. Never drop them while the
  provider being unstaged holds an active credential of its own: a switch interrupted after promoting the staged
  credential leaves it with both, and its tables then hold every live session and recorded group. Decide that from the
  provider's own credential, failing closed on an error, not from which provider the dispatcher reports as configured,
  which may still be the outgoing one.
