<script lang="ts">
	import { onMount } from 'svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { AdminService } from '$lib/services';
	import type { MCPCapacityInfo } from '$lib/services/admin/types';
	import { Info, LoaderCircle } from 'lucide-svelte';

	let capacityInfo = $state<MCPCapacityInfo | null>(null);
	let loading = $state(true);

	const sourceLabels: Record<string, string> = {
		resourceQuota: 'ResourceQuota',
		deployments: 'Deployments'
	};

	async function fetchCapacity() {
		try {
			capacityInfo = await AdminService.getMCPCapacity();
		} catch {
			// Silently fail - banner just won't show
			capacityInfo = null;
		}
	}

	onMount(() => {
		fetchCapacity().finally(() => {
			loading = false;
		});

		// Poll every 60 seconds for changes from other users
		const interval = setInterval(fetchCapacity, 60000);
		return () => clearInterval(interval);
	});

	function formatValue(value: string | undefined): string {
		if (!value) return '0';
		return value;
	}

	// Export refresh function for parent components to call
	export function refresh() {
		fetchCapacity();
	}
</script>

{#if loading}
	<div class="bg-surface2 dark:bg-surface1 mb-4 flex items-center justify-center rounded-md p-4">
		<LoaderCircle class="size-5 animate-spin" />
	</div>
{:else if capacityInfo && !capacityInfo.error}
	<div class="bg-surface2 dark:bg-surface1 mb-4 rounded-md p-4 shadow-sm">
		<div class="mb-3 flex items-center gap-1">
			<h3 class="text-sm font-semibold">MCP Namespace Capacity</h3>
			<span
				class="text-on-surface1"
				use:tooltip={{
					text: `Data source: ${sourceLabels[capacityInfo.source] || capacityInfo.source}`,
					disablePortal: true
				}}
			>
				<Info class="size-3.5" />
			</span>
		</div>

		<div class="grid grid-cols-3 gap-4">
			<!-- Active Deployments -->
			<div class="flex flex-col">
				<span class="text-on-surface1 text-xs">Active Deployments</span>
				<span class="text-lg font-semibold">{capacityInfo.activeDeployments}</span>
			</div>

			<!-- CPU -->
			<div class="flex flex-col">
				<span class="text-on-surface1 text-xs">CPU Requested</span>
				<span class="text-lg font-semibold">
					{#if capacityInfo.cpuLimit}
						{formatValue(capacityInfo.cpuRequested)} / {formatValue(capacityInfo.cpuLimit)}
					{:else}
						{formatValue(capacityInfo.cpuRequested)}
					{/if}
				</span>
			</div>

			<!-- Memory -->
			<div class="flex flex-col">
				<span class="text-on-surface1 text-xs">Memory Requested</span>
				<span class="text-lg font-semibold">
					{#if capacityInfo.memoryLimit}
						{formatValue(capacityInfo.memoryRequested)} / {formatValue(capacityInfo.memoryLimit)}
					{:else}
						{formatValue(capacityInfo.memoryRequested)}
					{/if}
				</span>
			</div>
		</div>
	</div>
{/if}
