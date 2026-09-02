<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import { MCP_PUBLISHER_ALL_OPTION, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		fetchMcpServerAndEntries,
		getPoweruserWorkspace,
		initMcpServerAndEntries
	} from '$lib/context/poweruserWorkspace.svelte';
	import { mcpServersAndEntries, profile } from '$lib/stores/index.js';
	import { goto } from '$lib/url';
	import { onMount, untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { accessControlRule, workspaceId } = $derived(data);
	const duration = PAGE_TRANSITION_DURATION;
	const isAdmin = $derived(profile.current.hasAdminAccess?.());

	if (!untrack(() => profile.current.hasAdminAccess?.())) {
		initMcpServerAndEntries();
	}

	onMount(() => {
		if (!isAdmin && workspaceId) {
			fetchMcpServerAndEntries(workspaceId);
		}
	});

	let title = $derived(accessControlRule?.displayName ?? 'MCP Registry');
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#if isAdmin}
			<AccessControlRuleForm
				{accessControlRule}
				onUpdate={() => {
					goto('/mcp-servers?view=access-policies');
				}}
				mcpEntriesContextFn={() => mcpServersAndEntries.current}
				readonly={profile.current.isAdminReadonly?.()}
			/>
		{:else}
			<AccessControlRuleForm
				{accessControlRule}
				onUpdate={() => {
					goto('/mcp-servers?view=access-policies');
				}}
				entity="workspace"
				id={workspaceId}
				mcpEntriesContextFn={getPoweruserWorkspace}
				all={MCP_PUBLISHER_ALL_OPTION}
				readonly={profile.current.isAdminReadonly?.()}
			/>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
