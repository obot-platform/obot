<script lang="ts">
	import type { VMCPInstance } from '$lib/services';
	import { formatTimeAgo } from '$lib/time';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import { Layers, StepForward } from '@lucide/svelte';

	interface Props {
		onSelectInstance: (instance: VMCPInstance) => void;
		title?: string;
	}

	let { onSelectInstance, title = 'Select Your Connection' }: Props = $props();

	let selectInstanceDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let instances = $state<VMCPInstance[]>([]);

	export function open(initInstances: VMCPInstance[] = []) {
		instances = initInstances;
		selectInstanceDialog?.open();
	}

	export function close() {
		selectInstanceDialog?.close();
	}
</script>

<ResponsiveDialog class="bg-base-200 dark:bg-base-100" bind:this={selectInstanceDialog} {title}>
	<Table
		data={instances}
		fields={['id', 'created']}
		headers={[{ title: 'Connection', property: 'id' }]}
		onClickRow={async (d) => {
			selectInstanceDialog?.close();
			onSelectInstance?.(d);
		}}
		disablePortal
	>
		{#snippet onRenderColumn(property, d)}
			{#if property === 'id'}
				<div class="flex shrink-0 items-center gap-2">
					<div class="icon">
						<Layers class="size-6" />
					</div>
					<p class="font-mono text-sm">{d.id}</p>
				</div>
			{:else if property === 'created'}
				{formatTimeAgo(d.created).relativeTime}
			{/if}
		{/snippet}
		{#snippet actions()}
			<IconButton class="hover:dark:bg-base-100/50">
				<StepForward class="size-4" />
			</IconButton>
		{/snippet}
	</Table>
</ResponsiveDialog>
