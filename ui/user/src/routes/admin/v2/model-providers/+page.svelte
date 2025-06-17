<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import ModelProvider from '$lib/components/admin/ModelProvider.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import {
		CommonModelProviderIds,
		PAGE_TRANSITION_DURATION,
		RecommendedModelProviders
	} from '$lib/constants';
	import { AdminService } from '$lib/services/index.js';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let { modelProviders } = data;

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="my-8" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<div
			class="flex flex-col gap-8"
			in:fly={{ x: 100, delay: duration, duration }}
			out:fly={{ x: -100, duration }}
		>
			<h1 class="text-2xl font-semibold">Model Providers</h1>
		</div>
		<div class="grid grid-cols-2 gap-4 py-8 md:grid-cols-3 lg:grid-cols-4">
			{#each modelProviders as modelProvider}
				<ModelProvider
					{modelProvider}
					recommended={RecommendedModelProviders.includes(modelProvider.id)}
				/>
			{/each}
		</div>
	</div>
</Layout>

<!-- <Confirm
	msg={`Are you sure you want to delete this project?`}
	show={Boolean(deletingTaskRun)}
	onsuccess={async () => {
		if (!deletingTaskRun) return;
		loading = true;
		// tasks = await AdminService.listTasks();
		loading = false;
		deletingTaskRun = undefined;
	}}
	oncancel={() => (deletingTaskRun = undefined)}
	{loading}
/> -->

<svelte:head>
	<title>Obot | Model Providers</title>
</svelte:head>
