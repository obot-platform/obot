<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import SkillAccessPolicyForm from '$lib/components/admin/SkillAccessPolicyForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { profile } from '$lib/stores/index.js';
	import { goto } from '$lib/url';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { skillAccessPolicy } = $derived(data);
	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(skillAccessPolicy?.displayName ?? 'Skill Access Policy');
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<SkillAccessPolicyForm
			{skillAccessPolicy}
			onUpdate={() => {
				goto('/admin/skill-access-policies');
			}}
			readonly={profile.current.isAdminReadonly?.()}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
