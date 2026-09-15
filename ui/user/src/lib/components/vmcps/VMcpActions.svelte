<script lang="ts">
	import type { VMCP, VMCPInstance } from '$lib/services';
	import Confirm from '../Confirm.svelte';
	import VMcpDiffDialog from './VMcpDiffDialog.svelte';
	import VMcpSelectInstance from './VMcpSelectInstance.svelte';
	import { CircleAlert } from '@lucide/svelte';

	let selectInstanceDialog = $state<ReturnType<typeof VMcpSelectInstance>>();
	let diffDialog = $state<ReturnType<typeof VMcpDiffDialog>>();
	let showUpdateConfirm = $state(false);
	let updateName = $state('');
	let updating = $state(false);
	let onSelectInstance = $state<(instance: VMCPInstance) => void>();
	let pendingUpdate = $state<() => Promise<void>>();

	export function openSelectInstance(
		instances: VMCPInstance[],
		onSelect: (instance: VMCPInstance) => void
	) {
		onSelectInstance = onSelect;
		selectInstanceDialog?.open(instances);
	}

	export function openDiff(vmcp: VMCP) {
		diffDialog?.open(vmcp);
	}

	export function openUpdateConfirm(name: string, onConfirm: () => Promise<void>) {
		updateName = name;
		pendingUpdate = onConfirm;
		showUpdateConfirm = true;
	}
</script>

<VMcpSelectInstance
	bind:this={selectInstanceDialog}
	title="Select Connection to Disconnect"
	onSelectInstance={(instance) => onSelectInstance?.(instance)}
/>

<VMcpDiffDialog bind:this={diffDialog} />

<Confirm
	show={showUpdateConfirm}
	onsuccess={async () => {
		updating = true;
		try {
			await pendingUpdate?.();
			showUpdateConfirm = false;
		} finally {
			updating = false;
		}
	}}
	oncancel={() => (showUpdateConfirm = false)}
	loading={updating}
	type="info"
	title="Confirm Update"
>
	{#snippet msgContent()}
		<h4 class="flex items-center justify-center gap-2 text-lg font-semibold">
			<CircleAlert class="size-5" />
			{`Update ${updateName}?`}
		</h4>
	{/snippet}
	{#snippet note()}
		<p class="text-sm font-light">The vMCP will be updated to its latest catalog configuration.</p>
	{/snippet}
</Confirm>
