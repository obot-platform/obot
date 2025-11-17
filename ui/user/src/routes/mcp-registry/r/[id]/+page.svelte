<script lang="ts">
	import { goto } from '$app/navigation';
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

	const defaultRoute = '/mcp-registry/created';
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

	let title = $derived(accessControlRule?.displayName ?? 'Registry');
</script>

<Layout
	{title}
	showBackButton
	onBackButtonClick={() => {
		goto('/mcp-registry');
	}}
>
	<div class="my-4 h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<AccessControlRuleForm
			{accessControlRule}
			onUpdate={() => {
				goto('/mcp-registry');
			}}
			entity="workspace"
			id={workspaceId}
			mcpEntriesContextFn={getPoweruserWorkspace}
			all={MCP_PUBLISHER_ALL_OPTION}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
