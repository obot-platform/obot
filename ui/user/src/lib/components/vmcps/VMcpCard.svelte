<script lang="ts">
	import { resolve } from '$app/paths';
	import Loading from '$lib/icons/Loading.svelte';
	import { toInlineHTMLFromMarkdown } from '$lib/markdown';
	import { UserService, type VMCP, type VMCPInstance } from '$lib/services';
	import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
	import {
		vmcpConnectURL,
		vmcpHasUserAllowedConfiguration,
		vmcpInstanceNeedsUserConfiguration,
		vmcpNeedsUpdate
	} from '$lib/services/vmcps/utils';
	import { errors, profile, vmcpInstances } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { poll } from '$lib/utils';
	import DotDotDot from '../DotDotDot.svelte';
	import VMcpCardActions from './VMcpCardActions.svelte';
	import {
		CircleFadingArrowUp,
		ExternalLink,
		GitCompare,
		Pencil,
		Power,
		ServerCog,
		Trash2
	} from '@lucide/svelte';
	import { onDestroy, type Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		vmcp: VMCP;
		owner?: string;
		connectEl?: HTMLElement;
		onSelect?: () => void;
		onEditDetails?: () => void;
		onConnect?: (options?: VMcpConnectOptions) => void;
		hideTest?: boolean;
		onDelete?: () => void;
		icon: Snippet;
		children?: Snippet;
		class?: string;
		selectAriaLabel: string;
		onUpdate?: (vmcp: VMCP) => void;
		openSelectInstance?: (
			instances: VMCPInstance[],
			onSelect: (instance: VMCPInstance) => void,
			title?: string
		) => void;
		openDiff?: (vmcp: VMCP) => void;
		openUpdateConfirm?: (vmcp: VMCP, onConfirm: () => Promise<void>) => void;
		openEditInstanceConfiguration?: (vmcp: VMCP, instance: VMCPInstance) => void;
	}

	let {
		vmcp,
		owner,
		connectEl = $bindable(),
		onSelect,
		onEditDetails,
		onConnect,
		hideTest,
		onDelete,
		icon,
		children,
		class: clazz,
		selectAriaLabel,
		onUpdate,
		openSelectInstance,
		openDiff,
		openUpdateConfirm,
		openEditInstanceConfiguration
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
	let instancesNeedingConfiguration = $derived(
		myInstances.filter((instance) => vmcpInstanceNeedsUserConfiguration(instance))
	);
	let instanceNeedingConfiguration = $derived(instancesNeedingConfiguration[0]);
	let canEditInstanceConfiguration = $derived(
		Boolean(openEditInstanceConfiguration && vmcpHasUserAllowedConfiguration(vmcp)) &&
			myInstances.length > 0
	);
	let disconnecting = $state(false);
	let updating = $state(false);
	let destroyed = false;
	let hasActions = $derived(
		isCreator || profile.current.hasAdminAccess?.() || myInstances.length > 0
	);

	onDestroy(() => {
		destroyed = true;
	});

	async function disconnectInstance(instanceID: string) {
		disconnecting = true;
		try {
			await UserService.deleteVMCPInstance(instanceID);
			vmcpInstances.remove(instanceID);
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
			let updated: VMCP | undefined;
			await poll(
				async () => {
					if (destroyed) return true;
					updated = await UserService.getVMCP(id);
					return destroyed || !vmcpNeedsUpdate(updated);
				},
				{ interval: 1000 }
			);
			if (destroyed || !updated || vmcpNeedsUpdate(updated)) return;
			onUpdate?.(updated);
			success.add(`Updated ${name}.`);
		} catch {
			if (!destroyed) {
				errors.append('Failed to update vMCP.');
			}
		} finally {
			if (!destroyed) {
				updating = false;
			}
		}
	}

	async function handleDisconnect(toggle: (open?: boolean) => void) {
		if (myInstances.length === 1) {
			await disconnectInstance(myInstances[0].id);
			toggle(false);
			return;
		}
		openSelectInstance?.(
			myInstances,
			(instance) => disconnectInstance(instance.id),
			'Select Connection to Disconnect'
		);
		toggle(false);
	}

	function handleEditInstanceConfiguration(toggle?: (open?: boolean) => void) {
		if (myInstances.length === 0) return;
		if (myInstances.length === 1 || !openSelectInstance) {
			openEditInstanceConfiguration?.(vmcp, myInstances[0]);
			toggle?.(false);
			return;
		}
		openSelectInstance(
			myInstances,
			(instance) => openEditInstanceConfiguration?.(vmcp, instance),
			'Select Connection to Configure'
		);
		toggle?.(false);
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
		{#if hasActions}
			<DotDotDot
				placement="bottom-start"
				class="pointer-events-auto relative z-10 size-9 shrink-0"
				classes={{ menu: 'min-w-48' }}
				ariaLabel={`Actions for ${name}`}
			>
				{#snippet children({ toggle })}
					{#if onEditDetails}
						<button class="menu-button" onclick={onEditDetails}>
							<Pencil class="size-4" /> Edit Details
						</button>
					{/if}
					<button
						class="menu-button"
						disabled={disconnecting}
						onclick={async (e) => {
							e.stopPropagation();
							if (openSelectInstance && connected && myInstances.length > 0) {
								await handleDisconnect(toggle);
							} else {
								disconnecting = true;
								await new Promise((resolve) => setTimeout(resolve, 1000));
								disconnecting = false;
							}
						}}
					>
						{#if disconnecting}
							<Loading class="size-4" />
						{:else}
							<Power class="size-4" />
						{/if}
						Reset
					</button>
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
							Update vMCP
						</button>
					{/if}
					{#if canEditInstanceConfiguration}
						<button
							class={twMerge(
								'menu-button',
								instancesNeedingConfiguration.length > 0 &&
									'bg-warning/10 text-warning hover:bg-warning/30'
							)}
							onclick={(e) => {
								e.stopPropagation();
								handleEditInstanceConfiguration(toggle);
							}}
						>
							<ServerCog class="size-4" /> Edit Configuration
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
					{#if isCreator || profile.current.hasAdminAccess?.()}
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
		{/if}
	</div>

	{#if children}
		{@render children()}
	{/if}

	<div class="pointer-events-auto relative z-10">
		<VMcpCardActions
			{id}
			{connectURL}
			{connectButtonId}
			bind:connectEl
			{onConnect}
			{hideTest}
			disabled={!canConnect}
		/>
	</div>

	<div
		class="pt-2 border-t border-base-200 dark:border-base-400 flex items-center justify-between gap-4"
	>
		<p class="text-muted-content text-xs font-light min-h-4">
			{owner}
		</p>

		{#if needsUpdate && canUpdate && openUpdateConfirm}
			<button
				class="pointer-events-auto relative z-10 badge badge-xs shrink-0 gap-1 badge-soft badge-primary"
				onclick={() => openUpdateConfirm(vmcp, handleUpdate)}
			>
				<span class="status status-primary"></span>
				Update Available
			</button>
		{:else if instanceNeedingConfiguration && canEditInstanceConfiguration}
			<button
				class="pointer-events-auto relative z-10 badge badge-xs shrink-0 gap-1 badge-soft badge-warning"
				onclick={() => handleEditInstanceConfiguration()}
			>
				<span class="status status-warning"></span>
				Not Configured
			</button>
		{:else if connected}
			<div class="badge badge-xs shrink-0 gap-1 badge-soft badge-primary" role="status">
				<span class="status status-primary" aria-hidden="true"></span>
				<span>Connected</span>
			</div>
		{/if}
	</div>
</div>
