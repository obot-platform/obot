<script lang="ts">
	import McpCompositeOauth from '$lib/components/mcp/McpCompositeOauth.svelte';
	import { m } from '$lib/i18n';

	export let data: { compositeMcpId: string; vmcpId?: string; oauthAuthRequestId?: string };
</script>

<McpCompositeOauth
	compositeMcpId={data.compositeMcpId}
	vmcpId={data.vmcpId}
	oauthAuthRequestId={data.oauthAuthRequestId}
/>

<svelte:head>
	<title>Obot | {m.vmcps_authenticate()}</title>
</svelte:head>
