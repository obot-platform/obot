<script lang="ts">
	import { m } from '$lib/i18n';
	import { doGet } from '$lib/services/http';
	import ClaudeAccountMappings from './ClaudeAccountMappings.svelte';
	import type { ClaudeConfig } from './types';
	import { onMount } from 'svelte';

	let config = $state<ClaudeConfig>({ enabled: false });
	let error = $state('');
	let ready = $state(false);

	const organizations = $derived([
		...new Set((config.environments ?? []).map((environment) => environment.organizationID))
	]);

	async function load() {
		error = '';
		try {
			config = (await doGet('/claude-environments')) as ClaudeConfig;
			ready = true;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	onMount(() => {
		void load();
	});
</script>

{#if error}
	<p role="alert" class="text-error">{error}</p>
{/if}
{#if ready && !config.enabled}
	<p class="text-muted-content">{m.hosted_agents_env_disabled()}</p>
{:else if ready}
	<div class="w-full max-w-4xl space-y-4">
		{#each organizations as organization (organization)}
			<ClaudeAccountMappings
				{organization}
				environments={(config.environments ?? []).filter(
					(environment) => environment.organizationID === organization
				)}
				onsaved={load}
			/>
		{:else}
			<p class="text-muted-content">{m.hosted_agents_env_empty()}</p>
		{/each}
	</div>
{/if}
