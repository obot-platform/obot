<script lang="ts">
	import { goto } from '$app/navigation';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import BackLink from '$lib/components/BackLink.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { browser } from '$app/environment';
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

	const defaultRoute = `/admin/access-control`;
	let fromURL = $state(defaultRoute);

	onMount(() => {
		if (browser) {
			const urlParams = new URLSearchParams(window.location.search);
			fromURL = urlParams.get('from') || defaultRoute;
		}
	});

	initMcpServerAndEntries();

	onMount(async () => {
		const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;
		fetchMcpServerAndEntries(defaultCatalogId);
	});
</script>

<Layout>
	<div class="my-4 h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<AccessControlRuleForm
			{accessControlRule}
			onUpdate={() => {
				goto('/admin/access-control');
			}}
			entity="workspace"
			id={workspaceId}
			mcpEntriesContextFn={getAdminMcpServerAndEntries}
			readonly={profile.current.isAdminReadonly?.()}
		>
			{#snippet topContent()}
				<BackLink
					currentLabel={accessControlRule?.displayName ?? 'Access Control Rule'}
					{fromURL}
				/>
			{/snippet}
		</AccessControlRuleForm>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {accessControlRule?.displayName ?? 'Access Control Rule'}</title>
</svelte:head>
