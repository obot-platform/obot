<script lang="ts">
	import { resolve } from '$app/paths';
	import Loading from '$lib/icons/Loading.svelte';
	import { toInlineHTMLFromMarkdown } from '$lib/markdown';
	import { UserService, type VMCP, type VMCPInstance } from '$lib/services';
	import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
	import { vmcpConnectURL, vmcpNeedsUpdate } from '$lib/services/vmcps/utils';
	import { errors, profile, vmcpInstances } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import DotDotDot from '../DotDotDot.svelte';
	import VMcpCardActions from './VMcpCardActions.svelte';
	import { CircleFadingArrowUp, ExternalLink, GitCompare, Trash2, Unplug } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		vmcp: VMCP;
		onSelect?: () => void;
		onConnect?: (options?: VMcpConnectOptions) => void;
		hideTest?: boolean;
		onDelete?: () => void;
		icon: Snippet;
		children?: Snippet;
		class?: string;
		selectAriaLabel: string;
		note?: string;
		onUpdate?: (vmcp: VMCP) => void;
		openSelectInstance?: (
			instances: VMCPInstance[],
			onSelect: (instance: VMCPInstance) => void
		) => void;
		openDiff?: (vmcp: VMCP) => void;
		openUpdateConfirm?: (vmcp: VMCP, onConfirm: () => Promise<void>) => void;
	}

	let {
		vmcp,
		onSelect,
		onConnect,
		hideTest,
		onDelete,
		icon,
		children,
		class: clazz,
		selectAriaLabel,
		note,
		onUpdate,
		openSelectInstance,
		openDiff,
		openUpdateConfirm
	}: Props = $props();

	let id = $derived(vmcp.id);
	let name = $derived(vmcp.displayName || 'Untitled vMCP');
	let descriptionHTML = $derived(
		vmcp.description ? toInlineHTMLFromMarkdown(vmcp.description) : undefined
	);
	let connectURL = $derived(vmcpConnectURL(vmcp));
	let connectButtonId = $derived(`btn-connect-to-server-${vmcp.id}`);
	let needsUpdate = $derived(vmcpNeedsUpdate(vmcp));
	let isCreator = $derived(Boolean(vmcp.userID && profile.current.id === vmcp.userID));
	let canDelete = $derived(Boolean(profile.current.isAdmin?.() || isCreator));
	let canUpdate = $derived(canDelete);
	let canConnect = $derived(!vmcp.userID || isCreator);
	let myInstances = $derived(
		vmcpInstances.current.items.filter(
			(instance) =>
				instance.vmcpID === vmcp.id && instance.userID === profile.current.id && !instance.deleted
		)
	);
	let connected = $derived(myInstances.length > 0);
	let disconnecting = $state(false);
	let updating = $state(false);

	async function disconnectInstance(instanceID: string) {
		disconnecting = true;
		try {
			await UserService.deleteVMCPInstance(instanceID);
			vmcpInstances.remove(instanceID);
			success.add(`Disconnected from ${name}.`);
		} catch {
			errors.append('Failed to disconnect from vMCP.');
		} finally {
			disconnecting = false;
		}
	}

	async function handleUpdate() {
		updating = true;
		try {
			await UserService.triggerVMCPUpdate(id);
			const updated = await UserService.getVMCP(id);
			onUpdate?.(updated);
			success.add(`Updated ${name}.`);
		} catch {
			errors.append('Failed to update vMCP.');
		} finally {
			updating = false;
		}
	}

	async function handleDisconnect(toggle: (open?: boolean) => void) {
		if (myInstances.length === 1) {
			await disconnectInstance(myInstances[0].id);
			toggle(false);
			return;
		}
		openSelectInstance?.(myInstances, (instance) => disconnectInstance(instance.id));
		toggle(false);
	}
</script>

<div class={twMerge('relative flex flex-col', onSelect && 'pointer-events-none', clazz)}>
	{#if onSelect}
		<button
			type="button"
			class="pointer-events-auto absolute inset-0 z-0 rounded-[inherit] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
			aria-label={selectAriaLabel}
			onclick={onSelect}
		></button>
	{/if}
	<div class="flex items-start gap-2">
		{@render icon()}
		<div class="min-w-0 grow">
			<div class="flex min-w-0 items-center gap-2">
				<p class="truncate text-sm font-semibold">{name}</p>
			</div>
			<p class="text-muted-content mt-0.5 line-clamp-2 text-xs font-light min-h-8">
				<!-- eslint-disable-next-line svelte/no-at-html-tags -- sanitized by toInlineHTMLFromMarkdown -->
				{@html descriptionHTML}
			</p>
		</div>
		<DotDotDot
			placement="bottom-start"
			class="pointer-events-auto relative z-10 size-9 shrink-0"
			classes={{ menu: 'min-w-48' }}
			ariaLabel={`Actions for ${name}`}
		>
			{#snippet children({ toggle })}
				<a
					class="menu-button justify-between"
					href={resolve(`/audit-logs?mcp_id=${encodeURIComponent(id)}`)}
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
					href={resolve(`/usage?mcp_id=${encodeURIComponent(id)}`)}
					target="_blank"
					rel="noopener"
					onclick={(e) => {
						e.stopPropagation();
						toggle(false);
					}}
				>
					View Usage <ExternalLink class="size-4" />
				</a>
				{#if openSelectInstance && connected && myInstances.length > 0}
					<button
						class="menu-button"
						disabled={disconnecting}
						onclick={async (e) => {
							e.stopPropagation();
							await handleDisconnect(toggle);
						}}
					>
						{#if disconnecting}
							<Loading class="size-4" />
						{:else}
							<Unplug class="size-4" />
						{/if}
						Disconnect
					</button>
				{/if}
				{#if openUpdateConfirm && needsUpdate && canUpdate}
					<button
						class="menu-button-primary"
						disabled={updating}
						onclick={(e) => {
							e.stopPropagation();
							openUpdateConfirm(vmcp, handleUpdate);
							toggle(false);
						}}
					>
						{#if updating}
							<Loading class="size-4" />
						{:else}
							<CircleFadingArrowUp class="size-4" />
						{/if}
						Update VMCP
					</button>
				{/if}
				{#if openDiff && needsUpdate}
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
				{#if canDelete}
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
			{/snippet}
		</DotDotDot>
	</div>

	{#if children}
		{@render children()}
	{/if}

	<div class="pointer-events-auto relative z-10">
		<VMcpCardActions
			{id}
			{connectURL}
			{connectButtonId}
			{onConnect}
			{hideTest}
			disabled={!canConnect}
		/>
	</div>

	{#if note}
		<div
			class="pt-2 border-t border-base-200 dark:border-base-400 flex items-center justify-between gap-4"
		>
			<p class="text-muted-content text-xs font-light min-h-4">
				{note}
			</p>

			{#if needsUpdate}
				<div class="badge badge-xs shrink-0 gap-1 badge-soft badge-primary">
					<span class="status status-primary"></span>
					Update Available
				</div>
			{:else}
			<div
				class={twMerge(
					'badge badge-xs shrink-0 gap-1',
					!connected
						? 'badge-soft badge-secondary dark:bg-base-200 dark:border-base-200'
						: 'badge-soft badge-primary'
				)}
				role="status"
				aria-label={connected ? 'Connected' : 'Not Connected'}
			>
				<span class={twMerge('status', connected ? 'status-primary' : 'status-secondary')}></span>
				{connected ? 'Connected' : 'Connected'}
			</div>
			{/if}
		</div>
	{/if}
</div>
