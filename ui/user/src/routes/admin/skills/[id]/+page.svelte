<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import SkillForm from '$lib/components/admin/SkillForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let skill = $derived(data.skill);
	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(skill?.displayName ?? 'Skill');
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#if skill}
			<SkillForm {skill} />
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
