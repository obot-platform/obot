<script lang="ts">
	import { resolve } from '$app/paths';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		editVMcpInstanceConfiguration,
		resetVMcpConnection,
		updateVMcp,
		vmcpActionProgress,
		vmcpIsDisconnecting,
		vmcpItemContext,
		type OpenDiff,
		type OpenEditInstanceConfiguration,
		type OpenSelectInstance,
		type OpenUpdateConfirm
	} from '$lib/runes/vmcps/vmcpItem.svelte';
	import type { VMCP } from '$lib/services';
	import { profile } from '$lib/stores';
	import {
		CircleFadingArrowUp,
		ExternalLink,
		GitCompare,
		Power,
		ServerCog,
		Trash2
	} from '@lucide/svelte';
	import { onDestroy } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		vmcp: VMCP;
		toggle: (open?: boolean) => void;
		onDelete?: () => void;
		onUpdated?: (vmcp: VMCP) => void;
		openSelectInstance?: OpenSelectInstance;
		openDiff?: OpenDiff;
		openUpdateConfirm?: OpenUpdateConfirm;
		openEditInstanceConfiguration?: OpenEditInstanceConfiguration;
	}

	let {
		vmcp,
		toggle,
		onDelete,
		onUpdated,
		openSelectInstance,
		openDiff,
		openUpdateConfirm,
		openEditInstanceConfiguration
	}: Props = $props();

	let destroyed = false;
	let ctx = $derived(vmcpItemContext(vmcp));
	let disconnecting = $derived(
		vmcpIsDisconnecting(
			vmcp.id,
			ctx.myInstances.map((instance) => instance.id)
		)
	);
	let updating = $derived(vmcpActionProgress.updatingId === vmcp.id);
	let canEditInstanceConfiguration = $derived(
		Boolean(openEditInstanceConfiguration) && ctx.canEditInstanceConfiguration
	);

	onDestroy(() => {
		destroyed = true;
	});
</script>

<button
	class="menu-button"
	disabled={disconnecting}
	onclick={(e) => {
		e.stopPropagation();
		void resetVMcpConnection(vmcp, toggle, openSelectInstance);
	}}
>
	{#if disconnecting}
		<Loading class="size-4" />
	{:else}
		<Power class="size-4" />
	{/if}
	Reset
</button>
{#if openUpdateConfirm && ctx.needsUpdate && ctx.canUpdate}
	<button
		class="menu-button-primary"
		disabled={updating}
		onclick={(e) => {
			e.stopPropagation();
			openUpdateConfirm(vmcp, () => updateVMcp(vmcp, onUpdated, () => destroyed));
			toggle(false);
		}}
	>
		{#if updating}
			<Loading class="size-4" />
		{:else}
			<CircleFadingArrowUp class="size-4" />
		{/if}
		Update vMCP
	</button>
{/if}
{#if canEditInstanceConfiguration}
	<button
		class={twMerge(
			'menu-button',
			ctx.instancesNeedingConfiguration.length > 0 &&
				'bg-warning/10 text-warning hover:bg-warning/30'
		)}
		onclick={(e) => {
			e.stopPropagation();
			editVMcpInstanceConfiguration(
				vmcp,
				openSelectInstance,
				openEditInstanceConfiguration,
				toggle
			);
		}}
	>
		<ServerCog class="size-4" /> Edit Configuration
	</button>
{/if}
{#if openDiff && ctx.needsUpdate}
	<button
		class="menu-button-primary"
		disabled={updating}
		onclick={(e) => {
			e.stopPropagation();
			openDiff(vmcp);
			toggle(false);
		}}
	>
		<GitCompare class="size-4" /> View Diff
	</button>
{/if}
{#if ctx.isCreator || profile.current.hasAdminAccess?.()}
	<a
		class="menu-button justify-between"
		href={resolve(`/audit-logs?mcp_id=${encodeURIComponent(vmcp.id)}`)}
		target="_blank"
		rel="noopener"
		onclick={(e) => {
			e.stopPropagation();
			toggle(false);
		}}
	>
		View Audit Logs <ExternalLink class="size-4" />
	</a>
	<a
		class="menu-button justify-between"
		href={resolve(`/usage?mcp_id=${encodeURIComponent(vmcp.id)}`)}
		target="_blank"
		rel="noopener"
		onclick={(e) => {
			e.stopPropagation();
			toggle(false);
		}}
	>
		View Usage <ExternalLink class="size-4" />
	</a>
{/if}
{#if ctx.canDelete}
	<button
		class="menu-button-destructive"
		onclick={(e) => {
			e.stopPropagation();
			onDelete?.();
			toggle(false);
		}}
	>
		<Trash2 class="size-4" />
		Delete
	</button>
{/if}
