<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { MCPCatalogServer } from '$lib/services';
	import { getMCPDisplayName, requiresUserUpdate } from '$lib/services/user/mcp';
	import { formatTimeAgo } from '$lib/time';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import { CircleFadingArrowUp, Server, StepForward } from '@lucide/svelte';

	interface Props {
		onSelectServer: (server: MCPCatalogServer) => void;
	}

	let { onSelectServer }: Props = $props();

	let selectServerDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let servers = $state<MCPCatalogServer[]>([]);

	export function open(initServers: MCPCatalogServer[] = []) {
		servers = initServers;
		selectServerDialog?.open();
	}

	export function close() {
		selectServerDialog?.close();
	}
</script>

<ResponsiveDialog
	class="bg-base-200 dark:bg-base-100"
	bind:this={selectServerDialog}
	title="Select Your Server"
>
	<Table
		data={servers}
		fields={['name', 'created']}
		onClickRow={async (d) => {
			selectServerDialog?.close();
			onSelectServer?.(d);
		}}
		disablePortal
	>
		{#snippet onRenderColumn(property, d)}
			{#if property === 'name'}
				<div class="flex shrink-0 items-center gap-2">
					<div class="icon">
						{#if d.manifest.icon}
							<img src={d.manifest.icon} alt={d.manifest.name} class="size-6" />
						{:else}
							<Server class="size-6" />
						{/if}
					</div>
					<p class="flex items-center gap-2">
						{getMCPDisplayName(d)}
						{#if requiresUserUpdate(d)}
							<span
								use:tooltip={{
									classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
									text: 'Configuration requires your attention'
								}}
							>
								<CircleFadingArrowUp class="text-primary size-4" />
							</span>
						{/if}
					</p>
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
