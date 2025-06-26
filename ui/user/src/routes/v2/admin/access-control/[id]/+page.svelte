<script lang="ts">
	import { goto } from '$app/navigation';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import {
		fetchMcpServerAndEntries,
		initMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte.js';
	import { ChevronLeft } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { accessControlRule: initialRule } = data;
	let accessControlRule = $state(initialRule);
	const duration = PAGE_TRANSITION_DURATION;
	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;

	initMcpServerAndEntries();

	onMount(async () => {
		fetchMcpServerAndEntries(defaultCatalogId);
	});
</script>

<Layout>
	<div class="my-4 h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<AccessControlRuleForm
			{accessControlRule}
			onUpdate={() => {
				goto('/v2/admin/access-control');
			}}
		>
			{#snippet topContent()}
				<a
					href={`/v2/admin/access-control`}
					class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
				>
					<ChevronLeft class="size-6" />
					Back to Access Control
				</a>
			{/snippet}
		</AccessControlRuleForm>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {accessControlRule?.displayName ?? 'Access Control Rule'}</title>
</svelte:head>
