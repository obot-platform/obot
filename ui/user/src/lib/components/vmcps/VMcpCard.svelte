<script lang="ts">
	import { resolve } from '$app/paths';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type VMCP } from '$lib/services';
	import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
	import { errors, profile, vmcpInstances } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import Confirm from '../Confirm.svelte';
	import DotDotDot from '../DotDotDot.svelte';
	import VMcpCardActions from './VMcpCardActions.svelte';
	import VMcpDiffDialog from './VMcpDiffDialog.svelte';
	import VMcpSelectInstance from './VMcpSelectInstance.svelte';
	import {
		CircleAlert,
		CircleFadingArrowUp,
		ExternalLink,
		GitCompare,
		Trash2,
		Unplug
	} from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id: string;
		name: string;
		descriptionHTML?: string;
		connectURL?: string;
		connectButtonId?: string;
		connected?: boolean;
		onSelect?: () => void;
		onConnect?: (options?: VMcpConnectOptions) => void;
		hideTest?: boolean;
		onDelete?: () => void;
		icon: Snippet;
		children?: Snippet;
		class?: string;
		selectAriaLabel: string;
		userID?: string;
		note?: string;
		needsUpdate?: boolean;
		vmcp?: VMCP;
		onUpdate?: (vmcp: VMCP) => void;
	}

	let {
		id,
		name,
		descriptionHTML,
		connectURL,
		connectButtonId,
		connected,
		onSelect,
		onConnect,
		hideTest,
		onDelete,
		icon,
		children,
		class: clazz,
		selectAriaLabel,
		note,
		userID,
		needsUpdate,
		vmcp,
		onUpdate
	}: Props = $props();

	let isCreator = $derived(Boolean(userID && profile.current.id === userID));
	let canDelete = $derived(Boolean(profile.current.isAdmin?.() || isCreator));
	let canUpdate = $derived(canDelete);
	let canConnect = $derived(!userID || isCreator);
	let myInstances = $derived(
		vmcpInstances.current.items.filter(
			(instance) =>
				instance.vmcpID === id &&
				instance.userID === profile.current.id &&
				!instance.deleted
		)
	);
	let disconnecting = $state(false);
	let updating = $state(false);
	let showUpdateConfirm = $state(false);
	let selectInstanceDialog = $state<ReturnType<typeof VMcpSelectInstance>>();
	let diffDialog = $state<ReturnType<typeof VMcpDiffDialog>>();

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
			const updated = await UserService.triggerVMCPUpdate(id);
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
		selectInstanceDialog?.open(myInstances);
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
				{#if connected}
					<div class="badge badge-xs badge-secondary shrink-0 gap-1">
						<span class="status status-primary"></span>
						Connected
					</div>
				{/if}
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
				{#if connected && myInstances.length > 0}
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
				{#if needsUpdate && canUpdate}
					<button
						class="menu-button-primary"
						disabled={updating}
						onclick={(e) => {
							e.stopPropagation();
							showUpdateConfirm = true;
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
				{#if needsUpdate && vmcp}
					<button
						class="menu-button-primary"
						disabled={updating}
						onclick={(e) => {
							e.stopPropagation();
							diffDialog?.open(vmcp);
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
		<div class="pt-2 border-t border-base-200 dark:border-base-400 flex justify-between gap-4">
			<p class="text-muted-content text-xs font-light min-h-4">
				{note}
			</p>
		</div>
	{/if}
</div>

<VMcpSelectInstance
	bind:this={selectInstanceDialog}
	title="Select Connection to Disconnect"
	onSelectInstance={(instance) => disconnectInstance(instance.id)}
/>

<VMcpDiffDialog bind:this={diffDialog} />

<Confirm
	show={showUpdateConfirm}
	onsuccess={async () => {
		await handleUpdate();
		showUpdateConfirm = false;
	}}
	oncancel={() => (showUpdateConfirm = false)}
	loading={updating}
	type="info"
	title="Confirm Update"
>
	{#snippet msgContent()}
		<h4 class="flex items-center justify-center gap-2 text-lg font-semibold">
			<CircleAlert class="size-5" />
			{`Update ${name}?`}
		</h4>
	{/snippet}
	{#snippet note()}
		<p class="text-sm font-light">
			The vMCP will be updated to its latest catalog configuration.
		</p>
	{/snippet}
</Confirm>
