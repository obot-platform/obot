<script lang="ts">
	import { m } from '$lib/i18n';
	import {
		editVMcpInstanceConfiguration,
		updateVMcp,
		vmcpIsUpdating,
		vmcpItemContext,
		type OpenEditInstanceConfiguration,
		type OpenSelectInstance,
		type OpenUpdateConfirm
	} from '$lib/runes/vmcps/vmcpItem.svelte';
	import type { VMCP } from '$lib/services';
	import { onDestroy } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		vmcp: VMCP;
		showEmpty?: boolean;
		class?: string;
		onUpdated?: (vmcp: VMCP) => void;
		openSelectInstance?: OpenSelectInstance;
		openUpdateConfirm?: OpenUpdateConfirm;
		openEditInstanceConfiguration?: OpenEditInstanceConfiguration;
	}

	let {
		vmcp,
		showEmpty = false,
		class: clazz,
		onUpdated,
		openSelectInstance,
		openUpdateConfirm,
		openEditInstanceConfiguration
	}: Props = $props();

	let destroyed = false;
	let ctx = $derived(vmcpItemContext(vmcp));
	let updating = $derived(vmcpIsUpdating(vmcp.id));
	let canEditInstanceConfiguration = $derived(
		Boolean(openEditInstanceConfiguration) && ctx.canEditInstanceConfiguration
	);
	let badgeClass = $derived(twMerge('badge badge-xs shrink-0 gap-1 badge-soft text-nowrap', clazz));

	onDestroy(() => {
		destroyed = true;
	});
</script>

{#if ctx.needsAdminConfiguration}
	<div class={twMerge(badgeClass, 'badge-warning')} role="status">
		<span class="status status-warning" aria-hidden="true"></span>
		<span>{m.core_mcp_value_configuration_required()}</span>
	</div>
{:else if openUpdateConfirm && ctx.needsUpdate && ctx.canUpdate}
	<button
		class={twMerge(badgeClass, 'badge-primary')}
		disabled={updating}
		onclick={(e) => {
			e.stopPropagation();
			openUpdateConfirm(vmcp, () => updateVMcp(vmcp, onUpdated, () => destroyed));
		}}
	>
		<span class="status status-primary"></span>
		{m.vmcps_update_available()}
	</button>
{:else if ctx.instancesNeedingConfiguration.length > 0 && canEditInstanceConfiguration}
	<button
		class={twMerge(badgeClass, 'badge-warning')}
		onclick={(e) => {
			e.stopPropagation();
			editVMcpInstanceConfiguration(vmcp, openSelectInstance, openEditInstanceConfiguration);
		}}
	>
		<span class="status status-warning"></span>
		{m.core_mcp_value_not_configured()}
	</button>
{:else if ctx.connected}
	<div class={twMerge(badgeClass, 'badge-primary')} role="status">
		<span class="status status-primary" aria-hidden="true"></span>
		<span>{m.core_mcp_value_connected()}</span>
	</div>
{:else if showEmpty}
	<span class="text-muted-content text-xs">—</span>
{/if}
