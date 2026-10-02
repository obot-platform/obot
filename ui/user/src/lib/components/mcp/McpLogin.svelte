<script lang="ts">
	import { browser } from '$app/environment';
	import { getLocalMcpConfig } from '$lib/services/user/mcp';
	import CopyField from '../CopyField.svelte';

	let { url, callbackPaths = [] }: { url: string; callbackPaths?: string[] } = $props();
	const id = $props.id();
	const quote = (value: string) => "'" + value.replaceAll("'", "'\"'\"'") + "'";
	let connectURL = $derived(
		browser && url.startsWith('/') ? new URL(url, window.location.origin).href : url
	);
	let command = $derived(
		`obot mcp login --url ${quote(connectURL)}` +
			getLocalMcpConfig(url, callbackPaths)
				.args.slice(3)
				.map((arg) => ` ${arg === '--callback-path' ? arg : quote(arg)}`)
				.join('')
	);
</script>

<div class="flex flex-col gap-3 text-sm">
	<p>
		Run this command in a terminal to authenticate before using the MCP inspector or listing tools.
		Your browser and the CLI must run on the same computer.
	</p>
	<CopyField {id} value={command} label="Authentication command" variant="code" />
	<p class="text-muted-content text-xs">After login completes, return here to continue.</p>
</div>
