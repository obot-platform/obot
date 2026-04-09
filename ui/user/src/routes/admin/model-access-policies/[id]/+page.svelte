<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import ModelAccessPolicyForm from '$lib/components/admin/ModelAccessPolicyForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { profile } from '$lib/stores/index.js';
	import { goto } from '$lib/url';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { modelAccessPolicy } = $derived(data);
	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(modelAccessPolicy?.displayName ?? 'Model Access Policy');
</script>

<Layout {title} showBackButton>
	<div class="mb-4 h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<ModelAccessPolicyForm
			{modelAccessPolicy}
			onUpdate={() => {
				goto('/admin/model-access-policies');
			}}
			readonly={profile.current.isAdminReadonly?.()}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
