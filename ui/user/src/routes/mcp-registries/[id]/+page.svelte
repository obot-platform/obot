<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import { MCP_PUBLISHER_ALL_OPTION, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import {
		fetchMcpServerAndEntries,
		getPoweruserWorkspace,
		initMcpServerAndEntries
	} from '$lib/context/poweruserWorkspace.svelte.js';
	import { goto } from '$lib/url';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let { accessControlRule, workspaceId } = $derived(data);
	const duration = PAGE_TRANSITION_DURATION;

	initMcpServerAndEntries();

	onMount(async () => {
		if (workspaceId) {
			fetchMcpServerAndEntries(workspaceId);
		}
	});

	let title = $derived(accessControlRule?.displayName ?? 'MCP Registry');
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<AccessControlRuleForm
			{accessControlRule}
			onUpdate={() => {
				goto('/mcp-registries');
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
