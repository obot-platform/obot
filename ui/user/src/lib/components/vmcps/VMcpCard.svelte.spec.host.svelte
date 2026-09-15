<script lang="ts">
	import type { VMCP, VMCPInstance } from '$lib/services';
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
		provideUpdateConfirm = true,
		provideEditConfiguration = true,
		owner
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
		provideEditConfiguration?: boolean;
		owner?: string;
	} = $props();

	let vmcpActions = $state<ReturnType<typeof VMcpActions>>();

	function openSelectInstance(
		instances: VMCPInstance[],
		onSelect: (instance: VMCPInstance) => void,
		title?: string
	) {
		vmcpActions?.openSelectInstance(instances, onSelect, title);
	}

	function openDiff(target: VMCP) {
		vmcpActions?.openDiff(target);
	}

	function openUpdateConfirm(target: VMCP, onConfirm: () => Promise<void>) {
		vmcpActions?.openUpdateConfirm(target, onConfirm);
	}

	function openEditInstanceConfiguration(target: VMCP, instance: VMCPInstance) {
		void vmcpActions?.openEditInstanceConfiguration(target, instance);
	}
</script>

<VMcpActions bind:this={vmcpActions} />
<VMcpCard
	{vmcp}
	{selectAriaLabel}
	{onDelete}
	{onConnect}
	{onUpdate}
	{icon}
	openSelectInstance={provideSelectInstance ? openSelectInstance : undefined}
	openDiff={provideDiff ? openDiff : undefined}
	openUpdateConfirm={provideUpdateConfirm ? openUpdateConfirm : undefined}
	openEditInstanceConfiguration={provideEditConfiguration
		? openEditInstanceConfiguration
		: undefined}
	{owner}
/>
