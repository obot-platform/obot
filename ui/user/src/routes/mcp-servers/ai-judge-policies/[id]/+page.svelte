<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import MessagePolicyForm from '$lib/components/admin/MessagePolicyForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import { profile } from '$lib/stores/index.js';
	import { goto } from '$lib/url';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { messagePolicy } = $derived(data);
	const duration = PAGE_TRANSITION_DURATION;
	const listHref = '/mcp-servers?view=ai-judge-policies';

	let title = $derived(messagePolicy?.displayName ?? m.mcps_ai_judge_policies_message_policy());
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<MessagePolicyForm
			{messagePolicy}
			{listHref}
			onUpdate={() => {
				goto(listHref);
			}}
			readonly={profile.current.isAdminReadonly?.()}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
