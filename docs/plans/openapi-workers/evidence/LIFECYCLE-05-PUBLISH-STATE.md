# FastAPI connector publish-state evidence

Date: 2026-11-10 15:55 Asia/Shanghai.

The same real catalog entry was toggled in the Obot admin UI while its API state was read through the browser session:

```json
{
  "id": "default-fastapi-items-api-3c82c6c554zvd",
  "marketPublished": {
    "off": "false",
    "on": "true"
  }
}
```

Boundary: local authentication is disabled and the current viewer is Owner/Admin. The Admin marketplace can render unpublished entries, so this proves the admin publish-state transition but not visibility as seen by a separate non-admin user.
