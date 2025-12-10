<script lang="ts">
	import { goto } from '$lib/url';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import BackLink from '$lib/components/BackLink.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { MCP_PUBLISHER_ALL_OPTION, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { browser } from '$app/environment';
	import {
		fetchMcpServerAndEntries,
		getPoweruserWorkspace,
		initMcpServerAndEntries
	} from '$lib/context/poweruserWorkspace.svelte.js';

	let { data } = $props();
	const { accessControlRule: initialRule, workspaceId } = data;
	let accessControlRule = $state(initialRule);
	const duration = PAGE_TRANSITION_DURATION;

	const defaultRoute = '/mcp-publisher/access-control';
	let fromURL = $state(defaultRoute);

	onMount(() => {
		if (browser) {
			const urlParams = new URLSearchParams(window.location.search);
			fromURL = urlParams.get('from') || defaultRoute;
		}
	});

	initMcpServerAndEntries();

	onMount(async () => {
		if (workspaceId) {
			fetchMcpServerAndEntries(workspaceId);
		}
	});
</script>

<Layout showUserLinks>
	<div class="my-4 h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<AccessControlRuleForm
			{accessControlRule}
			onUpdate={() => {
				goto('/mcp-publisher/access-control');
			}}
			entity="workspace"
			id={workspaceId}
			mcpEntriesContextFn={getPoweruserWorkspace}
			all={MCP_PUBLISHER_ALL_OPTION}
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
