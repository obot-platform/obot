<script lang="ts">
	import HostedAgentPools from '$lib/components/admin/HostedAgentPools.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import type {
		HostedAgentPool,
		HostedAgentPoolAssignment,
		HostedAgentPoolDefaults
	} from '$lib/services/admin/types';
	import { profile } from '$lib/stores/index.js';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		pools: HostedAgentPool[];
		assignments: HostedAgentPoolAssignment[];
		poolDefaults?: HostedAgentPoolDefaults;
	}

	let {
		pools: initialPools,
		assignments: initialAssignments,
		poolDefaults: initialDefaults
	}: Props = $props();
	let pools = $state(untrack(() => initialPools));
	let assignments = $state(untrack(() => initialAssignments));
	let poolDefaults = $state(untrack(() => initialDefaults));
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	const duration = PAGE_TRANSITION_DURATION;
</script>

<div class="flex flex-col gap-4" in:fade={{ duration }}>
	<p class="text-muted-content text-sm font-light">
		A pool is a shared bucket of CPU and memory. Every agent placed in it draws from the same budget
		and can borrow whatever its neighbours are not using, so agents have no fixed size of their own.
	</p>
	<HostedAgentPools
		bind:pools
		bind:assignments
		bind:defaults={poolDefaults}
		readonly={isReadonly}
	/>
</div>
