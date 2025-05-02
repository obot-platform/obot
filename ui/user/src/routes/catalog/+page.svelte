<script lang="ts">
	import Notifications from '$lib/components/Notifications.svelte';
	import type { PageProps } from './$types';
	import { q, qIsSet } from '$lib/url';
	import { ChevronLeft } from 'lucide-svelte';
	import { sortByFeaturedNameOrder, sortByPreferredMcpOrder } from '$lib/sort';
	import McpCatalog from '$lib/components/mcp/McpCatalog.svelte';
	import Navbar from '$lib/components/Navbar.svelte';
	import { goto } from '$app/navigation';
	import AgentCatalog from '$lib/components/agents/AgentCatalog.svelte';

	let { data }: PageProps = $props();
	const mcps = $derived(data.mcps.sort(sortByPreferredMcpOrder));
	const shares = $derived(data.shares.sort(sortByFeaturedNameOrder));

	function handleSelectMcp(mcpId: string) {
		goto(`/mcp?id=${mcpId}`);
	}

	const type = q('type');
</script>

<div class="flex h-full flex-col items-center">
	<Navbar />
	{#if type === 'agents'}
		<main class="colors-background relative flex w-full flex-col items-center justify-center py-12">
			<div class="flex w-full max-w-(--breakpoint-2xl) flex-col items-center justify-center">
				<AgentCatalog {shares} tools={data.tools} />
			</div>
		</main>
	{:else}
		<main class="colors-background relative flex w-full flex-col items-center justify-center pb-12">
			{#if qIsSet('from')}
				{@const from = decodeURIComponent(q('from'))}
				<div class="mt-8 flex w-full max-w-(--breakpoint-2xl) flex-col justify-start">
					<a
						href={from}
						class="button-text flex w-fit items-center gap-1 pb-0 text-base font-semibold text-black md:text-lg dark:text-white"
					>
						<ChevronLeft class="size-5" /> Back To Chat
					</a>
				</div>
			{/if}

			<div
				class="flex w-full max-w-(--breakpoint-xl) flex-col items-center justify-center gap-2 px-4 py-4"
				class:mt-12={!qIsSet('from')}
			>
				{#if qIsSet('new')}
					<h2 class="text-3xl font-semibold md:text-4xl">Welcome To Obot</h2>
				{:else}
					<h2 class="text-3xl font-semibold md:text-4xl">MCP Servers</h2>
				{/if}
				<p class="mb-8 max-w-full text-center text-base font-light md:max-w-md">
					Browse over evergrowing catalog of MCP servers and find the perfect one to set up your
					agent with.
				</p>
			</div>

			<McpCatalog
				{mcps}
				inline
				onSubmitMcp={handleSelectMcp}
				submitText="Create agent with server"
			/>
		</main>
	{/if}
	<Notifications />
</div>

<svelte:head>
	<title>Obot | {type === 'agents' ? 'Agent Catalog' : 'MCP Servers'}</title>
</svelte:head>
