<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import { profile } from '$lib/stores/index.js';
	import ProfileForm from '../ProfileForm.svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { modelAccessPolicy } = $derived(data);
	const duration = PAGE_TRANSITION_DURATION;

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let title = $derived(modelAccessPolicy?.displayName ?? m.models_profiles_new());
</script>

<Layout>
	<div class="h-full w-full py-4" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#key modelAccessPolicy?.id}
			<ProfileForm {modelAccessPolicy} {readonly} />
		{/key}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
