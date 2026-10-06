<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import AccessControlRuleForm from '$lib/components/admin/AccessControlRuleForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import { mcpServersAndEntries, profile } from '$lib/stores/index.js';
	import { goto } from '$lib/url';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { accessControlRule, workspaceId } = $derived(data);
	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(accessControlRule?.displayName ?? m.mcps_access_policies_mcp_registry());
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<AccessControlRuleForm
			{accessControlRule}
			onUpdate={() => {
				goto('/mcp-servers?view=access-policies');
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
