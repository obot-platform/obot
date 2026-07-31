---
title: MCP Tunnels
description: Connect Obot to remote MCP servers on private networks through an outbound WebSocket tunnel.
---

## Overview

MCP tunnels let the Obot gateway reach remote HTTP or HTTPS MCP servers that are not directly accessible from the Obot network. An `obot tunnel` process runs on a machine that can reach the private MCP server and opens an outbound, authenticated WebSocket connection to Obot.

Traffic follows this path:

1. The Obot gateway receives a request for a remote MCP server.
2. The remote MCP catalog entry selects an MCP tunnel by its generated ID.
3. Obot sends the HTTP request over that tunnel.
4. The `obot tunnel` process forwards the request to the real MCP server URL and returns its response.

The remote MCP server continues to use its ordinary URL, such as `http://mcp.internal.example:8080/mcp`. MCP tunnels proxy HTTP requests and streaming responses; they are not general-purpose TCP tunnels or VPNs.

## Prerequisites

- An Admin or Owner must create and configure the tunnel.
- A Docker container or the `obot` CLI must run on a long-lived machine that can reach both Obot and the remote MCP server.
- The tunnel machine must be allowed to make outbound WebSocket connections to Obot. It does not need an inbound port.
- A reverse proxy in front of Obot must allow WebSocket upgrades and long-lived connections on `/tunnel/connect`.
- The remote catalog entry must use an exact URL or hostname. URL templates are not supported with tunnels.

## Create an MCP tunnel

1. Open **MCP Management > MCP Tunnels**.
2. Select **Create MCP Tunnel**.
3. Enter a required **Display Name** and an optional **Description**.
4. Add one or more **Allowed URLs** that describe the destinations this tunnel may reach.
5. Select **Create Tunnel**.
6. Copy the tunnel secret and either the generated Docker or CLI command before closing the dialog.

Obot generates the tunnel ID and a bearer secret containing 64 random bytes. The complete secret is shown only when the tunnel is created or rotated. All other responses return only a shortened preview.

:::warning

Store the complete secret in a secure secret manager when it is created. The preview cannot be used to connect, and the existing secret cannot be retrieved later.

:::

## Configure allowed URLs

Allowed URLs are the tunnel's network boundary. An empty list permits no destinations. Each rule can be an exact value or contain one `*` at the beginning or end.

| Pattern | Allows |
|---|---|
| `https://mcp.internal.example/mcp` | Only that complete URL |
| `https://services.internal.example/*` | URLs beginning with that URL prefix |
| `mcp.internal.example` | Any HTTP or HTTPS URL with that exact hostname |
| `*.internal.example` | Hostnames ending in `.internal.example`, excluding the bare `internal.example` hostname |
| `build-*` | Hostnames beginning with `build-` |
| `*` | Every HTTP or HTTPS destination |

Hostname rules are case-insensitive and do not restrict the scheme, port, path, or query. Use a complete URL rule when those details must be constrained. Complete URL rules normalize the scheme, hostname, and default ports, while paths and queries remain significant.

A prefix rule is usually preferable to one exact URL when an MCP transport uses multiple paths or adds session query parameters.

:::warning

Use the narrowest rules practical. In particular, `*` allows the tunnel process to make requests to any HTTP or HTTPS destination it can reach.

:::

Obot checks the allowlist when a tunneled remote catalog entry or server is saved and again for every forwarded request. Updating the allowlist therefore affects new requests immediately, including requests from existing catalog entries.

## Connect the tunnel

The Docker tab is selected by default after creating or rotating a tunnel. Run the displayed command on a machine with network access to the remote MCP server:

```bash
docker run --rm ghcr.io/obot-platform/obot:v0.25.0 \
  tunnel \
  --obot-base-url https://obot.example.com/api \
  --token YOUR_TUNNEL_SECRET
```

The UI reads the running Obot version from `/api/version` and uses it as the image tag when it is a valid release or prerelease semantic version. Semantic-version build metadata is omitted because it is not valid in a Docker tag. Development, dirty, missing, or invalid versions use the `main` image tag.

Select the CLI tab to run an installed `obot` binary instead:

```bash
obot tunnel \
  --obot-base-url https://obot.example.com/api \
  --token YOUR_TUNNEL_SECRET
```

The secret identifies the MCP tunnel, so there is no tunnel name argument. Both forms invoke the same tunnel command. It converts the Obot URL to the WebSocket endpoint automatically; for example, `https://obot.example.com/api` connects to `wss://obot.example.com/tunnel/connect` on the normal Obot server port.

You can also set the Obot base URL with `OBOT_BASE_URL`:

```bash
export OBOT_BASE_URL=https://obot.example.com/api
obot tunnel --token YOUR_TUNNEL_SECRET
```

The tunnel command retries failed connection attempts and lost connections indefinitely, using exponential backoff from 1 second up to 30 seconds. Run it under a service manager or another supervisor for production use.

Multiple tunnel clients can connect with the same MCP tunnel secret, including
clients connected to different Obot replicas. Each connection multiplexes
concurrent requests. Obot routes a request through an available connection;
connections for the same tunnel are failover options rather than a guaranteed
load-balancing pool.

### Multiple Obot replicas

Helm deployments with `replicaCount` greater than `1` automatically form an internal tunnel peer mesh. The tunnel CLI still opens one WebSocket to one Obot replica, but every replica learns which tunnel is connected and can route gateway requests through the owning replica. The CLI does not need one connection per replica.

Every replica runs a non-leader-elected controller for the existing gateway
Service's Kubernetes EndpointSlices. The controller watches only the Service's
namespace and label, then connects directly to ready endpoint addresses on the
`http` port. EndpointSlice names are generated and a Service can have more than
one, so each event reconciles the complete set. The chart uses a dedicated
shared peer secret and grants read access only to EndpointSlices.

More than one client for the same tunnel can connect through the peer mesh at
once. The connected-tunnels API still reports the tunnel ID once, regardless of
how many clients are connected with its secret.

For normal Helm installs, the generated peer token is preserved across upgrades. For GitOps systems that render charts without access to the live cluster, set `tunnelPeer.token` to a stable secret value or provide a Secret containing `OBOT_SERVER_TUNNEL_PEER_TOKEN` with `tunnelPeer.existingSecret`.

Changing the peer token triggers a normal rolling update. During that rollout,
replicas using different tokens cannot peer, so tunneled requests can fail until
every replica uses the new value. Rotate the peer token during a maintenance
window and verify that the rollout completes. The same restriction applies to
externally managed peer secrets.

`/tunnel/peer` is authenticated separately from user-facing APIs. Because it is served by the gateway API mux, an ingress that forwards all paths to Obot also makes this endpoint reachable; the shared peer secret still prevents non-peer access. No separate public ingress route is required.

If a replica or peer connection is lost, new requests can use the tunnel again after the peer mesh or CLI reconnects. An in-flight request is failed rather than replayed because MCP requests may not be safe to repeat.

### CLI logs

The tunnel command logs connection changes and each forwarded request. Request and response entries share a `request_id`, making them easy to correlate. Request logs include the tunnel ID, HTTP method, target URL without its query or fragment, whether a query was present, and content length. Response logs add the status, response content length, and duration.

Headers, query values, and URL fragments are not logged. URL paths are logged, so do not place secrets in paths.

## Configure a remote MCP catalog entry

1. Open **MCP Management > MCP Catalog**.
2. Create or edit a remote MCP catalog entry.
3. Configure the real MCP endpoint using **Exact URL** or **Hostname**.
4. Open **Advanced Configuration**.
5. Select the MCP tunnel from the **Tunnel** dropdown and save the entry.

The dropdown displays the tunnel's display name and falls back to its generated ID. Obot stores the generated ID in `remoteConfig.tunnelName`.

The configured endpoint must match one of the selected tunnel's allowed URL rules. URL templates and interpolated URLs cannot be combined with a tunnel. System MCP servers also do not support tunnels.

:::note

Tunnel selection is available only for catalog entries managed directly in Obot. Entries synchronized from a catalog source cannot set `remoteConfig.tunnelName`.

:::

## Operate MCP tunnels

### Check live connections

`GET /api/tunnels` returns tunnels connected anywhere in the Obot installation:

```json
{
  "items": [
    {
      "name": "mt1example"
    }
  ]
}
```

`name` is the generated ID of each currently connected tunnel. A tunnel appears
only once even when it has multiple connected clients. The endpoint does not
expose connection timing or request statistics.

### Rotate a secret

Open the tunnel from **MCP Management > MCP Tunnels** and select **Rotate Secret**. Rotation immediately invalidates the old secret and disconnects clients using it. Save the newly displayed secret, then restart the tunnel clients with it.

A tunnel client that still uses the old secret continues retrying but cannot reconnect.

### Update or delete a tunnel

Changing the display name or description does not change the tunnel ID or secret. Narrowing **Allowed URLs** can immediately block requests from catalog entries that no longer match.

Obot does not allow a tunnel to be deleted while any MCP catalog entry still uses it, including
tunnel references nested in composite catalog entries. The API returns `400 Bad Request` and lists
the dependent catalog entries. Update those entries to remove or replace the tunnel first. Deleting
the tunnel then disconnects its clients.

## Security behavior

- Use HTTPS for Obot in production so the WebSocket uses WSS. The tunnel secret is a bearer credential, and forwarded MCP headers may contain credentials.
- Obot stores a SHA-256 digest and a shortened preview of the secret, not the complete secret.
- A tunnel secret is authorized only to open `GET /tunnel/connect`; it does not grant access to the Obot API or UI.
- Tunneled requests are checked against the current allowed URL rules before forwarding.
- Same-origin redirects remain inside the tunnel. Cross-origin redirect locations are not followed through the tunnel.
- Run the CLI only on a trusted machine because it makes the final request to the private destination.

For tunneled targets, the tunnel allowlist is the primary network boundary. The usual gateway restrictions on localhost, private, and link-local destinations do not apply because the tunnel machine performs DNS resolution and connects to the target.

## Troubleshooting

| Symptom | What to check |
|---|---|
| The CLI repeatedly receives `401 Unauthorized` | The secret is incorrect or was rotated. Restart with the complete current secret. |
| The CLI repeatedly disconnects | Verify the Obot base URL, TLS trust, outbound firewall rules, and WebSocket support or idle timeouts in any reverse proxy. |
| Obot reports that the tunnel is not connected | Confirm the CLI process is running and that the tunnel ID appears in `GET /api/tunnels`. |
| Obot reports that the target is not allowed | Compare the final target URL with the tunnel's current allowed URL rules. |
| A forwarded request fails with a gateway error | From the tunnel machine, verify DNS, TLS, firewall access, and direct connectivity to the real MCP URL. |
| A catalog entry cannot be saved | Use an exact URL or hostname that matches the allowlist. Remove any URL template before selecting a tunnel. |

## API reference

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/mcp-tunnels` | List configured MCP tunnels |
| `POST` | `/api/mcp-tunnels` | Create a tunnel and return its complete secret |
| `GET` | `/api/mcp-tunnels/{id}` | Get a tunnel with a secret preview |
| `PUT` | `/api/mcp-tunnels/{id}` | Update its manifest |
| `DELETE` | `/api/mcp-tunnels/{id}` | Delete it and disconnect its client |
| `POST` | `/api/mcp-tunnels/{id}/rotate-secret` | Rotate and return the new complete secret |
| `GET` | `/api/tunnels` | List live tunnel connections |
| `GET` | `/tunnel/connect` | CLI-only authenticated WebSocket upgrade |
| `GET` | `/tunnel/peer` | Internal authenticated WebSocket upgrade between Obot replicas |

The `POST` and `PUT` bodies contain the manifest fields directly:

```json
{
  "displayName": "Office network",
  "description": "Access to MCP services inside the office network",
  "allowedURLs": [
    "https://mcp.internal.example/mcp",
    "https://services.internal.example/*",
    "*.corp.example"
  ]
}
```
