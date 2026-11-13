<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import { profile } from '$lib/stores/index.js';
	import ProfileForm from '../ProfileForm.svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let modelAccessPolicy = $derived(data.modelAccessPolicy);
	const duration = PAGE_TRANSITION_DURATION;

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let title = $derived(modelAccessPolicy?.displayName ?? m.models_profiles_new());
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#key modelAccessPolicy?.id}
			<ProfileForm
				{modelAccessPolicy}
				{readonly}
				onUpdate={(updatedProfile) => {
					if (updatedProfile.id === modelAccessPolicy?.id) {
						modelAccessPolicy = updatedProfile;
					}
				}}
			/>
		{/key}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
