<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import SkillForm from '$lib/components/admin/SkillForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import { TriangleAlert } from '@lucide/svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let skill = $derived(data.skill);

	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(skill?.displayName ?? m.skills_skill_fallback_title());
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#if data?.showLicenseError}
			<div class="my-12 flex w-md flex-col items-center gap-4 m-auto text-center">
				<TriangleAlert class="size-12 text-warning" />
				<h4 class="text-muted-content text-lg font-semibold">
					{m.skills_limited_functionality()}
				</h4>
				<p class="text-muted-content text-sm font-light">
					{m.skills_skill_license_error_prefix()}
					<a href="mailto:info@obot.ai" class="text-link">info@obot.ai</a
					>{m.skills_license_error_suffix()}
				</p>
			</div>
		{:else if skill}
			<SkillForm {skill} />
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
