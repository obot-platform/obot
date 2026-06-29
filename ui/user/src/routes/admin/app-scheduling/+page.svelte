<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import SchedulingForm from '$lib/components/admin/SchedulingForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { formatSchedulingResources } from '$lib/format.js';
	import { type AppK8sSettings } from '$lib/services';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;
	let { data } = $props();
	let k8sSettings = $derived<AppK8sSettings | undefined>({
		available: data.k8sSettings?.available ?? false,
		resources: data.k8sSettings?.resources ?? '',
		affinity: data.k8sSettings?.affinity ?? '',
		tolerations: data.k8sSettings?.tolerations ?? '',
		runtimeClassName: data.k8sSettings?.runtimeClassName ?? ''
	});
	let resourceInfo = $state(untrack(() => formatSchedulingResources(data.k8sSettings?.resources)));
</script>

<Layout classes={{ container: 'pb-0' }} title="App Scheduling">
	<div class="relative h-full w-full" transition:fade={{ duration }}>
		{#if k8sSettings && k8sSettings.available}
			<div class="flex flex-col gap-8">
				<SchedulingForm
					readonly
					locked
					{resourceInfo}
					affinity={k8sSettings.affinity}
					tolerations={k8sSettings.tolerations}
					runtimeClassName={k8sSettings.runtimeClassName}
					type="app"
				/>
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | App Scheduling</title>
</svelte:head>
