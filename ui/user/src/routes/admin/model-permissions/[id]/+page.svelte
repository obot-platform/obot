<script lang="ts">
	import { goto } from '$lib/url';
	import ModelPermissionRuleForm from '$lib/components/admin/ModelPermissionRuleForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { fly } from 'svelte/transition';
	import { profile } from '$lib/stores/index.js';

	let { data } = $props();
	const { modelPermissionRule } = $derived(data);
	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(modelPermissionRule?.displayName ?? 'Model Permission Rule');
</script>

<Layout {title} showBackButton>
	<div class="mb-4 h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<ModelPermissionRuleForm
			{modelPermissionRule}
			onUpdate={() => {
				goto('/admin/model-permissions');
			}}
			readonly={profile.current.isAdminReadonly?.()}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
