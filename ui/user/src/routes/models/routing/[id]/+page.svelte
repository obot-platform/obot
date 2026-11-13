<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import { profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import RoutingForm from '../RoutingForm.svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let rule = $derived(data.rule);
	const duration = PAGE_TRANSITION_DURATION;

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let title = $derived(rule?.name ?? m.models_routing_edit_title());

	function backToList() {
		goto('/models?view=routing');
	}
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#key rule?.id}
			<RoutingForm
				{rule}
				{readonly}
				onCancel={backToList}
				onUpdate={(updatedRule) => {
					if (updatedRule && rule?.id === updatedRule?.id) {
						rule = updatedRule;
					}
				}}
			/>
		{/key}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
