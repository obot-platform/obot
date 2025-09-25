<script lang="ts">
	import { goto } from '$app/navigation';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import BackLink from '$lib/components/BackLink.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import {
		fetchMcpServerAndEntries,
		getAdminMcpServerAndEntries,
		initMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte.js';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { browser } from '$app/environment';
	import { profile } from '$lib/stores/index.js';

	let { data } = $props();
	const { accessControlRule: initialRule } = data;
	let accessControlRule = $state(initialRule);
	const duration = PAGE_TRANSITION_DURATION;
	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;

	let fromURL = $state('/admin/access-control');

	onMount(() => {
		if (browser) {
			const urlParams = new URLSearchParams(window.location.search);
			fromURL = urlParams.get('from') || '/access-control';
		}
	});

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
				goto('/admin/access-control');
			}}
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
