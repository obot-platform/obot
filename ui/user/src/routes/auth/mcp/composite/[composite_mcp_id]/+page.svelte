<script lang="ts">
	import McpCompositeOauth from '$lib/components/mcp/McpCompositeOauth.svelte';

	export let data: { compositeMcpId: string; vmcpId?: string; oauthAuthRequestId?: string };
</script>

<McpCompositeOauth
	compositeMcpId={data.compositeMcpId}
	vmcpId={data.vmcpId}
	oauthAuthRequestId={data.oauthAuthRequestId}
/>
