<script lang="ts">
	import { goto } from '$lib/url';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { fly } from 'svelte/transition';
	import { mcpServersAndEntries, profile } from '$lib/stores/index.js';

	let { data } = $props();
	const { accessControlRule: initialRule, workspaceId } = data;
	let accessControlRule = $state(initialRule);
	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(accessControlRule?.displayName ?? 'MCP Registry');
</script>

<Layout {title} showBackButton>
	<div class="mb-4 h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<AccessControlRuleForm
			{accessControlRule}
			onUpdate={() => {
				goto('/admin/mcp-registries');
			}}
			entity="workspace"
			id={workspaceId}
			mcpEntriesContextFn={() => mcpServersAndEntries.current}
			readonly={profile.current.isAdminReadonly?.()}
			isAdminView
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
