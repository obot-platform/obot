<script lang="ts">
	import McpClientLogin from '$lib/components/mcp/McpClientLogin.svelte';

	let { data }: { data: { authRequestID: string } } = $props();
</script>

<McpClientLogin
	authRequestID={data.authRequestID}
	onComplete={() => {
		window.location.href = `/auth/oauth/complete/${encodeURIComponent(data.authRequestID)}`;
	}}
/>
