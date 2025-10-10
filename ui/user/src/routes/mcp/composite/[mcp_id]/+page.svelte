<script lang="ts">
	import McpCompositeOauth from '$lib/components/mcp/McpCompositeOauth.svelte';
	export let data: { mcpID: string };
	const mcpID = data.mcpID;
</script>

<McpCompositeOauth {mcpID} />
