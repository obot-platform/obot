<script lang="ts">
	import { browser } from '$app/environment';
	import { m } from '$lib/i18n';
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
		{m.mcps_login_instructions()}
	</p>
	<a
		class="link text-xs"
		href="https://docs.obot.ai/installation/cli-setup"
		target="_blank"
		rel="noopener noreferrer">{m.mcps_login_install()}</a
	>
	<CopyField {id} value={command} label={m.mcps_login_command()} variant="code" />
	<p class="text-muted-content text-xs">
		{m.mcps_login_retry_instructions()}
	</p>
	{#if onComplete}
		<button
			type="button"
			class="btn btn-primary mt-2 self-end flex items-center justify-center gap-2"
			disabled={loading}
			onclick={onComplete}
		>
			{#if loading}
				<Loading class="text-primary size-4" />
				{m.mcps_login_validating()}
			{:else}
				{m.mcps_login_continue()}
			{/if}
		</button>
	{/if}
</div>
