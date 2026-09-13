<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import VMcpInstanceInfo from '$lib/components/vmcps/VMcpInstanceInfo.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const duration = PAGE_TRANSITION_DURATION;
	let usersMap = $derived(new Map(data.users.map((user) => [user.id, user])));
	let title = $derived(`${data.vmcp.displayName} | ${data.instance.id}`);
</script>

<Layout {title} showBackButton>
	<div class="flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		<VMcpInstanceInfo vmcp={data.vmcp} instance={data.instance} {usersMap} hideTitle />
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
