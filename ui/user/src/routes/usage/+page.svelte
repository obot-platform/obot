<script lang="ts">
	import TabLayout, { type TabView } from '$lib/components/TabLayout.svelte';
	import { profile } from '$lib/stores';
	import LlmUsageView from './LlmUsageView.svelte';
	import McpUsageView from './McpUsageView.svelte';

	let hasAdminAccess = $derived(Boolean(profile.current.hasAdminAccess?.()));
	let views = $derived.by(() => {
		const items: TabView[] = [{ label: 'MCP', value: 'mcp', content: mcp }];
		if (hasAdminAccess) {
			items.push({ label: 'LLM', value: 'llm', content: llm });
		}
		return items;
	});
</script>

<svelte:head>
	<title>Obot | Usage</title>
</svelte:head>

<TabLayout
	title="Usage"
	defaultView="mcp"
	{views}
	classes={{
		childrenContainer: 'max-w-none',
		noSidebarTitle: 'pl-4 md:pl-8 mx-auto md:max-w-(--breakpoint-xl) pt-4'
	}}
/>

{#snippet mcp()}
	<McpUsageView />
{/snippet}

{#snippet llm()}
	<LlmUsageView />
{/snippet}
