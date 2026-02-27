"""
Connect and verify only the WordPress MCP server (catalog entry default-wordpress-f9378c33).
Uses POST .../configure (user then catalog fallback). Does not run the full test case.

Configuration: WORDPRESS_SITE, WORDPRESS_URL (same value), WORDPRESS_USERNAME, WORDPRESS_PASSWORD
(application password from WordPress Users > Profile > Application Passwords).
For write operations (create post, etc.) the WordPress user must be Editor or Administrator;
Application Passwords inherit the user's role. "Read-only" errors mean the user role is
too limited (e.g. Subscriber) or the app password is for a read-only account.
Admin alternative: PUT /api/mcp-servers/{id} with manifest.env (requires admin token).

Direct MCP (after configure): Connect URL = {OBOT_URL}/mcp-connect/ms14gqpv;
  initialize -> notifications/initialized -> tools/list; then tools/call (e.g. validate_credential, list_posts).

Usage (set env then run):
  OBOT_EVAL_BASE_URL=http://localhost:8080
  OBOT_EVAL_AUTH_HEADER="Cookie: obot_access_token=YOUR_TOKEN"
  OBOT_EVAL_WP_URL=https://yoursite.com
  OBOT_EVAL_WP_USERNAME=your_username
  OBOT_EVAL_WP_APP_PASSWORD=your_app_password
  python connect_wordpress_mcp.py
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from eval.clients.client import Client

WORDPRESS_CATALOG_ENTRY_ID = "default-wordpress-f9378c33"


def _normalize_wordpress_site_url(url: str) -> str:
    """Site root for REST API; strip /wp-admin and trailing slashes."""
    if not url or not isinstance(url, str):
        return url or ""
    u = url.strip().rstrip("/")
    if u.endswith("/wp-admin"):
        u = u[: -len("/wp-admin")].rstrip("/")
    return u


def main() -> None:
    base_url = (os.environ.get("OBOT_EVAL_BASE_URL") or "http://localhost:8080").rstrip("/")
    auth_header = os.environ.get("OBOT_EVAL_AUTH_HEADER") or ""
    wp_url_raw = os.environ.get("OBOT_EVAL_WP_URL") or ""
    wp_username = os.environ.get("OBOT_EVAL_WP_USERNAME") or ""
    wp_password = os.environ.get("OBOT_EVAL_WP_APP_PASSWORD") or ""

    if not auth_header:
        print("ERROR: Set OBOT_EVAL_AUTH_HEADER (e.g. Cookie: obot_access_token=...)")
        sys.exit(1)
    if not wp_url_raw or not wp_username or not wp_password:
        print("ERROR: Set OBOT_EVAL_WP_URL, OBOT_EVAL_WP_USERNAME, OBOT_EVAL_WP_APP_PASSWORD")
        sys.exit(1)

    wp_url = _normalize_wordpress_site_url(wp_url_raw)
    if wp_url != wp_url_raw.strip().rstrip("/"):
        print("WordPress URL normalized to site root:", wp_url)

    c = Client(base_url, auth_header)

    # 1) Find existing user server for this catalog entry
    body, status = c._do("GET", "/api/mcp-servers")
    print("GET /api/mcp-servers ->", status)
    if status != 200:
        print("Failed to list MCP servers:", body[:500] if body else "")
        sys.exit(1)

    server_id = None
    try:
        data = json.loads(body)
        for s in data.get("items") or []:
            if not isinstance(s, dict):
                continue
            cid = (s.get("catalogEntryID") or s.get("mcpServerCatalogEntryId") or "").strip()
            if cid == WORDPRESS_CATALOG_ENTRY_ID:
                server_id = s.get("id") or (s.get("metadata") or {}).get("name")
                if server_id:
                    print("Found existing WordPress server:", server_id)
                    break
    except json.JSONDecodeError:
        pass

    # 2) Create server from catalog if not found
    if not server_id:
        payload = {"catalogEntryID": WORDPRESS_CATALOG_ENTRY_ID}
        body, status = c._do("POST", "/api/mcp-catalogs/default/servers", payload)
        print("POST /api/mcp-catalogs/default/servers (catalogEntryID=%s) -> %s" % (WORDPRESS_CATALOG_ENTRY_ID, status))
        if status not in (200, 201):
            print("Response:", body[:500] if body else "")
            sys.exit(1)
        try:
            created = json.loads(body)
            server_id = created.get("id") or (created.get("metadata") or {}).get("name")
        except json.JSONDecodeError:
            print("Invalid JSON response")
            sys.exit(1)
        if not server_id:
            print("Response missing server id")
            sys.exit(1)
        print("Created WordPress server:", server_id)

    # 3) Configure WordPress credentials (send both SITE and URL for MCP compatibility; user must be Editor/Admin for write)
    config_payload = {
        "WORDPRESS_SITE": wp_url,
        "WORDPRESS_URL": wp_url,
        "WORDPRESS_USERNAME": wp_username,
        "WORDPRESS_PASSWORD": wp_password,
        "WordPress App Password": wp_password,
    }
    body, status = c._do("POST", "/api/mcp-servers/%s/configure" % server_id, config_payload)
    print("POST /api/mcp-servers/%s/configure -> %s" % (server_id, status))
    if status not in (200, 201, 204):
        body_cat, status_cat = c._do(
            "POST", "/api/mcp-catalogs/default/servers/%s/configure" % server_id, config_payload
        )
        print("POST /api/mcp-catalogs/default/servers/%s/configure -> %s" % (server_id, status_cat))
        if status_cat not in (200, 201, 204):
            print("Configure failed. Response:", (body_cat or body)[:500] if (body_cat or body) else "")
            sys.exit(1)
        status = status_cat
        body = body_cat

    # 4) Verify: get server details (user-level or catalog)
    body, status = c._do("GET", "/api/mcp-servers/%s" % server_id)
    if status != 200:
        body, status = c._do("GET", "/api/mcp-catalogs/default/servers/%s" % server_id)
    print("GET server (verify) -> %s" % status)
    if status != 200:
        print("Verify get failed:", body[:300] if body else "")
    else:
        try:
            s = json.loads(body)
            configured = s.get("configured", False)
            missing = s.get("missingRequiredEnvVars")
            connect_url = s.get("connectURL") or ""
            print("WordPress MCP server connected successfully.")
            print("  server id: %s" % server_id)
            print("  configured: %s" % configured)
            print("  missingRequiredEnvVars: %s" % (missing if missing is not None else "null"))
            print("  connectURL: %s" % (connect_url or "(empty)"))
        except json.JSONDecodeError:
            print("Response (raw):", body[:300] if body else "")

    print("Done.")


if __name__ == "__main__":
    main()
