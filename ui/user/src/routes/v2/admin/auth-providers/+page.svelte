<script lang="ts">
	import ModelProvider from '$lib/components/admin/ModelProvider.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION, RecommendedModelProviders } from '$lib/constants';
	import { fade } from 'svelte/transition';
	import ProviderConfigure from '$lib/components/admin/ProviderConfigure.svelte';
	import type { AuthProvider } from '$lib/services/admin/types.js';

	let { data } = $props();
	let { authProviders } = data;

	let providerConfigure = $state<ReturnType<typeof ProviderConfigure>>();
	let configuringAuthProvider = $state<AuthProvider>();

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="my-4" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<h1 class="text-2xl font-semibold">Auth Providers</h1>
		</div>
		<div class="grid grid-cols-2 gap-4 py-8 md:grid-cols-3 lg:grid-cols-4">
			{#each authProviders as authProvider}
				<ModelProvider
					modelProvider={authProvider}
					recommended={RecommendedModelProviders.includes(authProvider.id)}
					onConfigure={() => {
						configuringAuthProvider = authProvider;
						providerConfigure?.open();
					}}
				/>
			{/each}
		</div>
	</div>
</Layout>

<ProviderConfigure bind:this={providerConfigure} provider={configuringAuthProvider} />

<svelte:head>
	<title>Obot | Auth Providers</title>
</svelte:head>
