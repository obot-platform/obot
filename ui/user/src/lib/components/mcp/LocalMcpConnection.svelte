<script lang="ts">
	import { getLocalMcpConfig } from '$lib/services/user/mcp';
	import CopyField from '../CopyField.svelte';

	let {
		id,
		url,
		callbackPaths = []
	}: { id: string; url: string; callbackPaths?: string[] } = $props();
	let config = $derived(
		JSON.stringify({ mcpServers: { [id]: getLocalMcpConfig(url, callbackPaths) } }, null, 2)
	);
</script>

<details class="my-4">
	<summary class="cursor-pointer text-sm font-semibold"
		>Connect with the Obot CLI (localhost OAuth)</summary
	>
	<p class="text-muted-content my-3 text-xs">
		Install the Obot CLI and make <code>obot</code> available on your PATH. Use this STDIO configuration
		for providers that require localhost OAuth callbacks. Your browser and the CLI must run on the same
		computer.
	</p>
	<CopyField value={config} id={`local-mcp-config-${id}`} variant="code" />
</details>
