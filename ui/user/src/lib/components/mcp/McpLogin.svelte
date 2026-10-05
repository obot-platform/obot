<script lang="ts">
	import { browser } from '$app/environment';
	import Loading from '$lib/icons/Loading.svelte';
	import CopyField from '../CopyField.svelte';

	let {
		url,
		onComplete,
		loading = false
	}: { url: string; onComplete?: () => void; loading?: boolean } = $props();
	const id = $props.id();
	const quote = (value: string) => "'" + value.replaceAll("'", "'\"'\"'") + "'";
	let loginURL = $derived(
		browser && url.startsWith('/') ? new URL(url, window.location.origin).href : url
	);
	let command = $derived(`obot mcp login --url ${quote(loginURL)}`);
</script>

<div class="flex flex-col gap-3 text-sm w-full min-w-0">
	<p>
		Run this command in a terminal to authenticate. Your browser and the CLI must run on the same
		computer.
	</p>
	<CopyField {id} value={command} label="Authentication command" variant="code" />
	<p class="text-muted-content text-xs">
		After login completes, return here to continue. If the command expires or its callback port is
		busy, continue to get a new command.
	</p>
	{#if onComplete}
		<button
			type="button"
			class="btn btn-primary self-start flex items-center justify-center gap-2"
			disabled={loading}
			onclick={onComplete}
		>
			{#if loading}
				<Loading class="text-primary size-4" />
				Validating authentication...
			{:else}
				Continue
			{/if}
		</button>
	{/if}
</div>
