<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Devices from '$lib/components/admin/devices/Devices.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | Devices</title>
</svelte:head>

<Layout title="Devices">
	<div
		class="flex h-full w-full flex-col gap-4"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		<Devices devices={data?.devices} users={data?.users} />
	</div>
</Layout>
