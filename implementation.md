# 2nd Level OAuth for Composite MCP Servers - Implementation Plan

## Overview

This document provides a detailed implementation plan for adding 2nd level OAuth support to composite MCP servers. When external clients or users connect to a composite MCP server, they need to authenticate with each child remote MCP server that requires OAuth.

## Requirements

From `plan.md`:
- Clients connecting to a composite MCP server must go through OAuth for every child OAuth-enabled MCP server
- Users should see a page/dialog that allows them to "Authenticate" or "Skip" each child server
- Child servers that aren't authenticated shouldn't be proxied to (their tools/resources/prompts won't be available)
- OAuth tokens for each child must be attached when proxying requests

## Design Decisions

### URL Structure
Simplified from the original design, we use:
```
/mcp/composite/{mcp_id}
```

Where:
- `{mcp_id}` - The composite parent MCP server ID

### State Management
- **No schema changes needed** - Use existing `MCPOAuthToken` table
- Multiple children share the same `OAuthAuthRequestID` (it's just an index, not unique)
- Each child gets its own `MCPOAuthToken` record with same `OAuthAuthRequestID`
- Existing `stateCache` and gateway DB handle all state persistence

### UI Component Strategy
Create a single `McpCompositeOauth.svelte` component that renders as a standalone page for composite MCP server authentication. The component is used directly in chat requirements dialogs within the Obot UI.

## Implementation Steps

### 1. Backend: Modify CheckForMCPAuth

**File**: `/Users/nick/projects/obot-platform/obot/pkg/api/handlers/mcpgateway/oauth/mcpoauthhandler.go`

**Current code** (lines 41-74):
```go
func (f *MCPOAuthHandlerFactory) CheckForMCPAuth(ctx context.Context, mcpServer v1.MCPServer, mcpServerConfig mcp.ServerConfig, userID, mcpID, oauthAppAuthRequestID string) (string, error) {
	if mcpServerConfig.Runtime != types.RuntimeRemote {
		// OAuth is only support for remote MCP servers.
		return "", nil
	}
	// ... existing single server OAuth check
}
```

**Modified code**:
```go
func (f *MCPOAuthHandlerFactory) CheckForMCPAuth(ctx context.Context, mcpServer v1.MCPServer, mcpServerConfig mcp.ServerConfig, userID, mcpID, oauthAppAuthRequestID string) (string, error) {
	// Handle composite servers
	if mcpServerConfig.Runtime == types.RuntimeComposite {
		return f.checkForCompositeMCPAuth(ctx, mcpServer, userID, mcpID, oauthAppAuthRequestID)
	}

	if mcpServerConfig.Runtime != types.RuntimeRemote {
		// OAuth is only support for remote MCP servers.
		return "", nil
	}

	// ... existing single server OAuth check (unchanged)
}

func (f *MCPOAuthHandlerFactory) checkForCompositeMCPAuth(ctx context.Context, mcpServer v1.MCPServer, userID, mcpID, oauthAppAuthRequestID string) (string, error) {
	// Query child servers
	var childServerList v1.MCPServerList
	if err := f.k8sClient.List(ctx, &childServerList, kclient.MatchingLabels{
		"composite-parent": mcpServer.Name,
	}); err != nil {
		return "", fmt.Errorf("failed to list child servers: %w", err)
	}

	// Check if any child needs OAuth
	needsOAuth := false
	for _, childServer := range childServerList.Items {
		childConfig, err := getServerConfig(ctx, f.k8sClient, childServer)
		if err != nil {
			return "", fmt.Errorf("failed to get config for child %s: %w", childServer.Name, err)
		}

		if childConfig.Runtime != types.RuntimeRemote {
			continue
		}

		// Check if this child already has a valid token
		hasToken, err := f.tokenStore.ForUserAndMCP(userID, childServer.Name).HasValidToken(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to check token for child %s: %w", childServer.Name, err)
		}

		if !hasToken {
			needsOAuth = true
			break
		}
	}

	if !needsOAuth {
		// All children are authenticated
		return "", nil
	}

	// Return URL to composite OAuth page
	return fmt.Sprintf("%s/mcp/composite/%s", f.baseURL, mcpID), nil
}
```

**Notes**:
- Reuses existing child server loading pattern from `handler.go:117-136`
- Checks if any child needs OAuth by looking for missing/invalid tokens
- Returns composite page URL if any child needs authentication
- All children will share the same `oauthAppAuthRequestID`

### 2. Backend: Update OAuth Callback Handler

**File**: `/Users/nick/projects/obot-platform/obot/pkg/api/handlers/mcpgateway/oauth/authorize.go`

**Current code** (lines 342-353):
```go
func (h *handler) oauthCallback(req api.Context) error {
	oauthAuthRequestID, err := h.oauthChecker.stateCache.createToken(req.Context(), req.URL.Query().Get("state"), req.URL.Query().Get("code"), req.URL.Query().Get("error"), req.URL.Query().Get("error_description"))
	if err != nil {
		return types.NewErrHTTP(http.StatusBadRequest, err.Error())
	}

	if oauthAuthRequestID == "" {
		// If there is no OAuth request object, then MCP OAuth wasn't started by OAuth; likely the UI kicked it off.
		// Redirect to the login complete page.
		http.Redirect(req.ResponseWriter, req.Request, "/login_complete", http.StatusFound)
		return nil
	}
```

**Modified code**:
```go
func (h *handler) oauthCallback(req api.Context) error {
	oauthAuthRequestID, err := h.oauthChecker.stateCache.createToken(req.Context(), req.URL.Query().Get("state"), req.URL.Query().Get("code"), req.URL.Query().Get("error"), req.URL.Query().Get("error_description"))
	if err != nil {
		return types.NewErrHTTP(http.StatusBadRequest, err.Error())
	}

	if oauthAuthRequestID == "" {
		// If there is no OAuth request object, then MCP OAuth wasn't started by OAuth; likely the UI kicked it off.
		// Check if this is part of a composite flow
		compositeMCPID, err := h.getCompositeMCPIDFromState(req.Context(), req.URL.Query().Get("state"))
		if err == nil && compositeMCPID != "" {
			// Redirect back to composite OAuth page
			http.Redirect(req.ResponseWriter, req.Request, fmt.Sprintf("/mcp/composite/%s", compositeMCPID), http.StatusFound)
			return nil
		}

		// Regular single server flow - redirect to login complete page
		http.Redirect(req.ResponseWriter, req.Request, "/login_complete", http.StatusFound)
		return nil
	}

	// ... rest of existing OAuth flow (unchanged)
}

func (h *handler) getCompositeMCPIDFromState(ctx context.Context, state string) (string, error) {
	// Retrieve the stored state to check if it's part of a composite flow
	stateObj, err := h.oauthChecker.stateCache.get(ctx, state)
	if err != nil {
		return "", err
	}

	// Check if the MCP server associated with this state has a composite-parent label
	// If it does, we need to find the parent
	var mcpServer v1.MCPServer
	if err := h.k8sClient.Get(ctx, kclient.ObjectKey{
		Namespace: h.namespace,
		Name:      stateObj.mcpID,
	}, &mcpServer); err != nil {
		return "", err
	}

	// Check if this server has a composite-parent label
	if parentName, ok := mcpServer.Labels["composite-parent"]; ok {
		// This is a child server - return the parent's ID
		var parentServer v1.MCPServer
		if err := h.k8sClient.Get(ctx, kclient.ObjectKey{
			Namespace: h.namespace,
			Name:      parentName,
		}, &parentServer); err != nil {
			return "", err
		}
		return parentServer.Name, nil
	}

	return "", nil
}
```

**Notes**:
- After OAuth callback completes, checks if the server is a child of a composite
- If yes, redirects back to composite OAuth page instead of `/login_complete`
- Uses `composite-parent` label to identify parent-child relationships
- Regular single-server flows remain unchanged

### 3. Frontend: Create McpCompositeOauth Component

**File**: `/Users/nick/projects/obot-platform/obot/ui/user/src/lib/components/mcp/McpCompositeOauth.svelte`

```svelte
<script lang="ts">
	import {
		AdminService,
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer
	} from '$lib/services';
	import { parseErrorContent } from '$lib/errors';
	import { Info, LoaderCircle, Server, X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';

	interface Props {
		mcpID: string;
		onComplete?: () => void;
	}

	let { mcpID, onComplete }: Props = $props();

	interface ChildServerAuth {
		id: string;
		name: string;
		icon?: string;
		oauthURL: string | null;
		authenticated: boolean;
		loading: boolean;
		error?: string;
	}

	let parentServer = $state<MCPCatalogEntry | null>(null);
	let childServers = $state<ChildServerAuth[]>([]);
	let loading = $state(true);
	let error = $state<string>('');
	let dialog = $state<HTMLDialogElement>();
	let initializedListener = $state(false);

	const allAuthenticated = $derived(
		childServers.length > 0 && childServers.every((c) => c.authenticated)
	);

	async function loadServers() {
		loading = true;
		error = '';

		try {
			// Load parent server info
			const servers = await ChatService.listSingleOrRemoteMcpServers();
			parentServer = servers.find((s) => s.id === mcpID) || null;

			if (!parentServer) {
				throw new Error('Composite MCP server not found');
			}

			// Load child servers (those with composite-parent label)
			const allServers = await ChatService.listSingleOrRemoteMcpServers();
			const children = allServers.filter((s) =>
				s.labels?.['composite-parent'] === parentServer?.name
			);

			// Initialize child server auth status
			childServers = await Promise.all(
				children.map(async (child) => {
					const authStatus = await checkChildAuthStatus(child.id);
					return {
						id: child.id,
						name: child.alias || child.manifest?.name || child.id,
						icon: child.manifest?.icon,
						oauthURL: authStatus.oauthURL,
						authenticated: authStatus.authenticated,
						loading: false
					};
				})
			);
		} catch (err) {
			const { message } = parseErrorContent(err);
			error = message;
		} finally {
			loading = false;
		}
	}

	async function checkChildAuthStatus(
		childID: string
	): Promise<{ authenticated: boolean; oauthURL: string | null }> {
		try {
			// Try to get OAuth URL - if none needed, server is authenticated
			const url = await ChatService.getMcpServerOauthURL(childID, {
				dontLogErrors: true
			});
			return { authenticated: false, oauthURL: url || null };
		} catch (err) {
			// If error or no URL, assume authenticated
			return { authenticated: true, oauthURL: null };
		}
	}

	async function refreshChildStatus(childID: string) {
		const child = childServers.find((c) => c.id === childID);
		if (!child) return;

		child.loading = true;
		child.error = undefined;

		try {
			const status = await checkChildAuthStatus(childID);
			child.authenticated = status.authenticated;
			child.oauthURL = status.oauthURL;

			if (allAuthenticated) {
				// All children authenticated - notify completion
				onComplete?.();
				if (asDialog && dialog?.open) {
					dialog.close();
				}
			}
		} catch (err) {
			const { message } = parseErrorContent(err);
			child.error = message;
		} finally {
			child.loading = false;
		}
	}

	function handleVisibilityChange() {
		if (document.visibilityState === 'visible') {
			// User returned to page - refresh all non-authenticated children
			childServers.forEach((child) => {
				if (!child.authenticated) {
					refreshChildStatus(child.id);
				}
			});
		}
	}

	onMount(() => {
		loadServers();

		if (asDialog && dialog) {
			dialog.showModal();
		}

		document.addEventListener('visibilitychange', handleVisibilityChange);
		initializedListener = true;

		return () => {
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});

	function handleClose() {
		if (asDialog && dialog?.open) {
			dialog.close();
		}
	}
</script>

{#if asDialog}
	<dialog
		bind:this={dialog}
		class="flex w-full flex-col gap-4 p-4 md:w-lg"
		use:dialogAnimation={{ type: 'fade' }}
	>
		<div class="absolute top-2 right-2">
			<button class="icon-button" onclick={handleClose}>
				<X class="size-4" />
			</button>
		</div>

		<div class="flex items-center gap-2">
			<div class="h-fit flex-shrink-0 self-start rounded-md bg-gray-50 p-1 dark:bg-gray-600">
				{#if parentServer?.manifest?.icon}
					<img src={parentServer.manifest.icon} alt={parentServer.name} class="size-6" />
				{:else}
					<Server class="size-6" />
				{/if}
			</div>
			<h3 class="text-lg leading-5.5 font-semibold">
				{parentServer?.name || 'MCP Server Authentication'}
			</h3>
		</div>

		<p class="text-sm">
			This composite MCP server requires authentication with multiple services. Please
			authenticate with each service below.
		</p>

		{#if loading}
			<div class="flex items-center justify-center gap-2 py-4">
				<LoaderCircle class="size-4 animate-spin" />
				<span>Loading servers...</span>
			</div>
		{:else if error}
			<div class="notification-error p-3 text-sm">
				{error}
			</div>
		{:else}
			<div class="flex flex-col gap-3">
				{#each childServers as child (child.id)}
					<div class="flex items-center justify-between rounded border p-3">
						<div class="flex items-center gap-2">
							{#if child.icon}
								<img src={child.icon} alt={child.name} class="size-5" />
							{:else}
								<Server class="size-5" />
							{/if}
							<span class="font-medium">{child.name}</span>
						</div>

						{#if child.authenticated}
							<span class="text-sm text-green-600 dark:text-green-400">Authenticated</span>
						{:else if child.loading}
							<div class="flex items-center gap-2 text-sm">
								<LoaderCircle class="size-4 animate-spin" />
								<span>Checking...</span>
							</div>
						{:else if child.error}
							<span class="text-sm text-red-600 dark:text-red-400">{child.error}</span>
						{:else if child.oauthURL}
							<a
								href={child.oauthURL}
								target="_blank"
								class="button-primary text-sm"
								onclick={() => {
									setTimeout(() => {
										child.loading = true;
									}, 500);
								}}
							>
								Authenticate
							</a>
						{:else}
							<button
								class="button-secondary text-sm"
								onclick={() => refreshChildStatus(child.id)}
							>
								Refresh
							</button>
						{/if}
					</div>
				{/each}
			</div>
		{/if}

		{#if allAuthenticated}
			<div class="notification-success p-3 text-sm">
				All services authenticated successfully!
			</div>
		{/if}
	</dialog>
{:else}
	<!-- Standalone page mode -->
	<div class="flex min-h-screen items-center justify-center bg-gray-50 p-4 dark:bg-gray-900">
		<div class="w-full max-w-lg rounded-lg border bg-white p-6 shadow-lg dark:bg-gray-800">
			<div class="mb-4 flex items-center gap-3">
				<div class="rounded-md bg-gray-50 p-2 dark:bg-gray-700">
					{#if parentServer?.manifest?.icon}
						<img src={parentServer.manifest.icon} alt={parentServer.name} class="size-8" />
					{:else}
						<Server class="size-8" />
					{/if}
				</div>
				<h1 class="text-2xl font-bold">
					{parentServer?.name || 'MCP Server Authentication'}
				</h1>
			</div>

			<p class="mb-6 text-sm text-gray-600 dark:text-gray-300">
				This composite MCP server requires authentication with multiple services. Please
				authenticate with each service below.
			</p>

			{#if loading}
				<div class="flex items-center justify-center gap-2 py-8">
					<LoaderCircle class="size-6 animate-spin" />
					<span>Loading servers...</span>
				</div>
			{:else if error}
				<div class="notification-error p-4">
					{error}
				</div>
			{:else}
				<div class="flex flex-col gap-4">
					{#each childServers as child (child.id)}
						<div class="flex items-center justify-between rounded-lg border p-4">
							<div class="flex items-center gap-3">
								{#if child.icon}
									<img src={child.icon} alt={child.name} class="size-6" />
								{:else}
									<Server class="size-6" />
								{/if}
								<span class="text-lg font-medium">{child.name}</span>
							</div>

							{#if child.authenticated}
								<span class="text-green-600 dark:text-green-400">✓ Authenticated</span>
							{:else if child.loading}
								<div class="flex items-center gap-2">
									<LoaderCircle class="size-5 animate-spin" />
									<span>Checking...</span>
								</div>
							{:else if child.error}
								<span class="text-red-600 dark:text-red-400">{child.error}</span>
							{:else if child.oauthURL}
								<a
									href={child.oauthURL}
									target="_blank"
									class="button-primary"
									onclick={() => {
										setTimeout(() => {
											child.loading = true;
										}, 500);
									}}
								>
									Authenticate
								</a>
							{:else}
								<button class="button-secondary" onclick={() => refreshChildStatus(child.id)}>
									Refresh
								</button>
							{/if}
						</div>
					{/each}
				</div>
			{/if}

			{#if allAuthenticated}
				<div class="notification-success mt-6 p-4">
					<p class="font-semibold">All services authenticated successfully!</p>
					<p class="mt-1 text-sm">You can now close this page and return to your application.</p>
				</div>
			{/if}
		</div>
	</div>
{/if}
```

**Notes**:
- Component works as a standalone page
- Fetches parent and child component servers using new endpoint
- Checks auth status for each child
- Handles visibility change to detect OAuth completion
- Shows authenticate buttons for each unauthenticated child
- Automatically detects when all children are authenticated

### 4. Frontend: Create Route for Standalone Page

**File**: `/Users/nick/projects/obot-platform/obot/ui/user/src/routes/mcp/composite/[mcp_id]/+page.svelte`

```svelte
<script lang="ts">
	import McpCompositeOauth from '$lib/components/mcp/McpCompositeOauth.svelte';
	export let data: { mcpID: string };
	const mcpID = data.mcpID;
</script>

<McpCompositeOauth {mcpID} />
```

**Notes**:
- Simple route that extracts params and passes to component
- Component handles all logic in standalone page mode

### 5. Frontend: Update McpServerRequirements Component

**File**: `/Users/nick/projects/obot-platform/obot/ui/user/src/lib/components/chat/McpServerRequirements.svelte`

**Add to imports** (around line 11):
```typescript
import McpCompositeOauth from '$lib/components/mcp/McpCompositeOauth.svelte';
```

**Add to requirement types** (around line 26):
```typescript
type Requirement =
	| { type: 'oauth'; id: string; name: string; icon?: string; oauthURL: string }
	| { type: 'config'; id: string; mcpID: string }
	| { type: 'composite-oauth'; id: string; mcpID: string };
```

**Modify requirements derivation** (around line 31) to detect composite OAuth URLs:
```typescript
let requirements = $derived(
	(() => {
		const reqs: Requirement[] = [];

		// Config requirements
		reqs.push(
			...projectMcps.items
				.filter((m) => (m.configured === false || m.needsURL) && !closed.has(m.id!))
				.map((m) => ({ type: 'config', id: m.id!, mcpID: m.mcpID! }) as Requirement)
		);

		// OAuth requirements
		for (const m of projectMcps.items) {
			if (!m.authenticated && m.oauthURL && !closed.has(m.id!)) {
				// Check if this is a composite OAuth URL
				if (m.oauthURL.includes('/mcp/composite/')) {
					// Extract mcp_id from URL: /mcp/composite/{mcp_id}
					const parts = m.oauthURL.split('/');
					const mcpID = parts[parts.length - 1];
					reqs.push({
						type: 'composite-oauth',
						id: m.id!,
						mcpID
					} as Requirement);
				} else {
					reqs.push({
						type: 'oauth',
						id: m.id!,
						name: m.name!,
						icon: m.icon,
						oauthURL: m.oauthURL!
					} as Requirement);
				}
			}
		}

		return reqs;
	})()
);
```

**Add composite OAuth dialog handling** (after line 285, before the closing `{/if}`):
```svelte
{:else if requirements[0]?.type === 'composite-oauth'}
	{@const compositeOauth = requirements[0] as Extract<Requirement, { type: 'composite-oauth' }>}
	<McpCompositeOauth
		mcpID={compositeOauth.mcpID}
		onComplete={() => {
			closed.add(compositeOauth.id);
			// Refresh project MCPs
			ChatService.listProjectMCPs(assistantId, projectId)
				.then((refreshed) => validateOauthProjectMcps(assistantId, projectId, refreshed, true))
				.then((updated) => {
					projectMcps.items = updated;
				})
				.catch(() => {
					// ignore refresh errors
				});
		}}
	/>
{/if}
```

**Notes**:
- Detects composite OAuth URLs in requirement list
- Extracts `mcpID` from URL path
- Renders `McpCompositeOauth` component
- Refreshes project MCPs when authentication completes

### 6. Frontend: Update McpOauth Component (Optional)

**File**: `/Users/nick/projects/obot-platform/obot/ui/user/src/lib/components/mcp/McpOauth.svelte`

If `McpOauth` is used in other contexts where composite OAuth might appear, add detection:

**After line 75** in `loadOauthURL()` function:
```typescript
try {
	// ... existing OAuth URL loading code ...

	// Check if this is a composite OAuth URL
	if (oauthURL && oauthURL.includes('/mcp/composite/')) {
		// For composite OAuth, redirect to the page
		// (McpServerRequirements will handle it in dialog mode for Obot UI)
		window.location.href = oauthURL;
		return;
	}
} catch (err: unknown) {
	// ... existing error handling ...
}
```

**Notes**:
- Only needed if `McpOauth` is used outside of `McpServerRequirements`
- Redirects to composite page when composite OAuth URL detected

## Testing Plan

### 1. Unit Testing

**Backend Tests**:
- Test `checkForCompositeMCPAuth` with composite servers that have:
  - No children needing OAuth (should return empty string)
  - Some children needing OAuth (should return composite URL)
  - All children needing OAuth (should return composite URL)

- Test `oauthCallback` redirect logic:
  - Child server OAuth callback redirects to composite page
  - Regular server OAuth callback redirects to `/login_complete`

**Frontend Tests**:
- Test `McpCompositeOauth` component:
  - Renders correctly in dialog mode
  - Renders correctly in standalone page mode
  - Loads child servers correctly
  - Detects authentication status
  - Handles visibility change events

### 2. Integration Testing

**Test Scenario 1: External MCP Client**
1. External client connects to composite MCP server
2. CheckForMCPAuth returns `/mcp/composite/{mcp_id}`
3. Client is redirected to standalone page
4. Page shows list of child servers requiring OAuth
5. User clicks "Authenticate" for first child
6. Opens OAuth popup, completes flow
7. Callback redirects back to composite page
8. Page detects authentication via visibility change
9. Shows first child as authenticated
10. User repeats for remaining children
11. When all authenticated, page shows success message

**Test Scenario 2: Obot UI (Project)**
1. User adds composite MCP server to project
2. McpServerRequirements detects composite OAuth URL
3. Shows composite OAuth dialog
4. Dialog lists all child servers
5. User authenticates each child
6. Dialog auto-closes when all complete
7. Project can now use composite server

**Test Scenario 3: Mixed Authentication States**
1. Composite server has 3 children
2. User has already authenticated with child 1
3. CheckForMCPAuth only returns URL for children 2 and 3
4. Composite page shows child 1 as authenticated
5. Shows authenticate buttons only for children 2 and 3

### 3. Edge Cases

**Test Case 1: OAuth Errors**
- Child OAuth fails with error
- Error message displayed next to child server
- Other children remain authenticatable
- User can retry failed authentication

**Test Case 2: Network Issues**
- Network fails while loading servers
- Error message displayed
- User can refresh to retry

**Test Case 3: Partial Authentication**
- User authenticates some children but not all
- Closes dialog/page
- Later reconnects to composite server
- Only shows unauthenticated children

**Test Case 4: Token Expiration**
- User has authenticated all children
- One token expires
- CheckForMCPAuth detects expired token
- Returns composite URL again
- Page shows only expired child needs re-authentication

## API Reference

### Existing APIs (Reused)

```typescript
// List all MCP servers (including composites and children)
ChatService.listSingleOrRemoteMcpServers(): Promise<MCPCatalogServer[]>

// Get OAuth URL for a specific server
ChatService.getMcpServerOauthURL(serverID: string): Promise<string>

// Check if server has valid OAuth token (used internally)
tokenStore.ForUserAndMCP(userID, mcpID).HasValidToken(ctx): Promise<boolean>
```

### No New APIs Required

The implementation reuses all existing OAuth endpoints and APIs. The composite OAuth flow is purely an orchestration layer on top of existing single-server OAuth.

## Database Schema

### No Changes Required

Existing `MCPOAuthToken` table supports multiple tokens with same `OAuthAuthRequestID`:

```go
type MCPOAuthToken struct {
	// ... other fields ...

	MCPID              string `gorm:"primaryKey"`    // Child server ID
	UserID             string `gorm:"primaryKey"`    // User ID
	OAuthAuthRequestID string `gorm:"index"`         // Shared across all children (NOT unique)

	// ... token fields ...
}
```

Each child gets its own row with:
- Unique `(UserID, MCPID)` primary key
- Shared `OAuthAuthRequestID` (just an index for querying)
- Own OAuth token and credentials

## Security Considerations

### Token Isolation
- Each child server's OAuth token is stored separately
- Compromising one child's token doesn't expose others
- Existing token encryption applies to all children

### State Management
- OAuth state strings are cryptographically random
- Each child OAuth flow gets unique state
- State is validated on callback to prevent CSRF

### Access Control
- Access to composite server controlled via existing ACR
- Access to individual children not required to use composite
- Tokens scoped to user + child server pair

## Future Enhancements

### Skip Functionality
From `plan.md`: "Clients should be redirected to a page that allows them to Authenticate or Skip"

**Implementation**:
- Add "Skip" button next to each child's "Authenticate" button
- Skipped children stored in user preferences or session
- Skipped children excluded from tool/resource listings
- User can un-skip later to authenticate

**Code changes**:
```svelte
<!-- In McpCompositeOauth.svelte -->
<button
	class="button-secondary text-sm"
	onclick={() => skipChild(child.id)}
>
	Skip
</button>
```

### Batch Authentication
Allow user to click "Authenticate All" to open OAuth popups sequentially:
```svelte
<button
	class="button-primary"
	onclick={authenticateAll}
>
	Authenticate All Services
</button>
```

### Progress Indicators
Show overall progress: "2 of 5 services authenticated"

### Notifications
Browser notifications when returning to page after completing OAuth in popup

## Migration Path

### Phase 1: Backend (No User Impact)
1. Deploy backend changes to `CheckForMCPAuth` and `oauthCallback`
2. Existing servers unaffected (no composite servers exist yet)
3. Composite OAuth URLs returned but no UI to handle them yet

### Phase 2: Frontend
1. Deploy `McpCompositeOauth` component
2. Deploy route for standalone page
3. Update `McpServerRequirements` to handle composite OAuth
4. Now composite servers can be added and used

### Phase 3: Create Composite Servers
1. Admins can now create composite MCP catalog entries
2. Users can add them to projects
3. External clients can connect to them

## Rollback Plan

If issues arise:

1. **Remove composite servers from catalog** - Prevents new usage
2. **Frontend rollback** - Remove composite OAuth handling, show error for composite URLs
3. **Backend rollback** - Revert `CheckForMCPAuth` and `oauthCallback` changes

No database migrations means no data corruption risk on rollback.

## Success Metrics

- [ ] External MCP client can authenticate with composite server's children
- [ ] Obot UI shows composite OAuth dialog for project MCPs
- [ ] All child OAuth tokens stored correctly in database
- [ ] OAuth callbacks redirect correctly back to composite page
- [ ] Visibility change detection works for OAuth completion
- [ ] No regressions in single-server OAuth flows
- [ ] No new API endpoints required
- [ ] No database schema changes required

## Conclusion

This implementation provides 2nd level OAuth for composite MCP servers by:

1. **Extending existing OAuth flow** - No new endpoints or schemas needed
2. **Reusing state management** - Existing `MCPOAuthToken` table and state cache
3. **Unified UI component** - Works for both Obot UI and external clients
4. **Graceful integration** - Minimal changes to existing code paths
5. **Extensible design** - Easy to add skip functionality and other enhancements later

The implementation follows existing patterns and integrates cleanly with the current OAuth architecture.
