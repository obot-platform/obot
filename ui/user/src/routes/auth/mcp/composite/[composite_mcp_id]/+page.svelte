<script lang="ts">
	import McpCompositeOauth from '$lib/components/mcp/McpCompositeOauth.svelte';
	export let data: { compositeMcpId: string; oauthAuthRequestId?: string };
</script>

<McpCompositeOauth
	compositeMcpId={data.compositeMcpId}
	oauthAuthRequestId={data.oauthAuthRequestId}
/>
