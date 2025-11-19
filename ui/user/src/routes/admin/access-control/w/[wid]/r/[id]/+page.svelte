<script lang="ts">
	import { goto } from '$app/navigation';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import {
		fetchMcpServerAndEntries,
		getAdminMcpServerAndEntries,
		initMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte.js';
	import { profile } from '$lib/stores/index.js';

	let { data } = $props();
	const { accessControlRule: initialRule, workspaceId } = data;
	let accessControlRule = $state(initialRule);
	const duration = PAGE_TRANSITION_DURATION;

	initMcpServerAndEntries();

	onMount(async () => {
		const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;
		fetchMcpServerAndEntries(defaultCatalogId);
	});

	let title = $derived(accessControlRule?.displayName ?? 'Access Control Rule');
</script>

<Layout {title} showBackButton>
	<div class="mb-4 h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<AccessControlRuleForm
			{accessControlRule}
			onUpdate={() => {
				goto('/admin/access-control');
			}}
			entity="workspace"
			id={workspaceId}
			mcpEntriesContextFn={getAdminMcpServerAndEntries}
			readonly={profile.current.isAdminReadonly?.()}
			isAdminView
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
