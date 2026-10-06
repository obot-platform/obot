<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { m } from '$lib/i18n';
	import type { MCPCatalogEntry, MCPCatalogServer } from '$lib/services';
	import {
		getMCPDisplayName,
		isDeprecatedMCPServer,
		requiresUserUpdate
	} from '$lib/services/user/mcp';
	import { formatTimeAgo } from '$lib/time';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import McpDeprecatedNotice from './McpDeprecatedNotice.svelte';
	import { CircleFadingArrowUp, Server, StepForward } from '@lucide/svelte';

	interface Props {
		onSelectServer: (server: MCPCatalogServer) => void;
		contextEntry?: MCPCatalogEntry;
	}

	let { onSelectServer, contextEntry }: Props = $props();

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
	title={m.mcps_deployments_select_your_server()}
>
	<Table
		data={servers}
		fields={['name', 'created']}
		headers={[
			{ title: m.core_name(), property: 'name' },
			{ title: m.core_col_created(), property: 'created' }
		]}
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
						<McpDeprecatedNotice
							deprecated={isDeprecatedMCPServer(contextEntry) || isDeprecatedMCPServer(d)}
						/>
						{#if requiresUserUpdate(d)}
							<span
								use:tooltip={{
									classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
									text: m.mcps_deployments_config_requires_attention()
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
