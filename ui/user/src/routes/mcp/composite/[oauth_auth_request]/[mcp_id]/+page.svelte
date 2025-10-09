<script lang="ts">
	import McpCompositeOauth from '$lib/components/mcp/McpCompositeOauth.svelte';
	import { page } from '$app/stores';

	const mcpID = $derived($page.params.mcp_id);
	const oauthAuthRequestID = $derived($page.params.oauth_auth_request);
</script>

<McpCompositeOauth {mcpID} {oauthAuthRequestID} asDialog={false} />
