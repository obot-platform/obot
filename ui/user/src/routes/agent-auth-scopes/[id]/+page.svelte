<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import AgentAuthScopeDetails from '$lib/components/agent-auth-scope/AgentAuthScopeDetails.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { ApiKeysService } from '$lib/services';
	import { goto } from '$lib/url';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { apiKey } = $derived(data);
	let title = $derived(apiKey?.name || 'Agent Auth Scope');
	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#if apiKey}
			<AgentAuthScopeDetails
				agentAuthScope={{ ...apiKey, prefix: `ok1-${apiKey.userId}-${apiKey.id}-*****` }}
				onDelete={async () => {
					await ApiKeysService.deleteAnyApiKey(apiKey.id.toString());
					goto('/agent-auth-scopes', { replaceState: true });
				}}
			/>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
