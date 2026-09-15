<script lang="ts">
	import type { VMCP } from '$lib/services';
	import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
	import VMcpActions from './VMcpActions.svelte';
	import VMcpCard from './VMcpCard.svelte';
	import type { Snippet } from 'svelte';

	let {
		vmcp,
		selectAriaLabel,
		onDelete,
		onConnect,
		onUpdate,
		icon,
		provideSelectInstance = true,
		provideDiff = true,
		provideUpdateConfirm = true
	}: {
		vmcp: VMCP;
		selectAriaLabel: string;
		onDelete?: () => void;
		onConnect?: (options?: VMcpConnectOptions) => void;
		onUpdate?: (vmcp: VMCP) => void;
		icon: Snippet;
		provideSelectInstance?: boolean;
		provideDiff?: boolean;
		provideUpdateConfirm?: boolean;
	} = $props();

	let vmcpActions = $state<ReturnType<typeof VMcpActions>>();
</script>

<VMcpActions bind:this={vmcpActions} />
<VMcpCard
	{vmcp}
	{selectAriaLabel}
	{onDelete}
	{onConnect}
	{onUpdate}
	{icon}
	openSelectInstance={provideSelectInstance ? vmcpActions?.openSelectInstance : undefined}
	openDiff={provideDiff ? vmcpActions?.openDiff : undefined}
	openUpdateConfirm={provideUpdateConfirm ? vmcpActions?.openUpdateConfirm : undefined}
/>
