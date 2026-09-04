<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		UserService,
		type MCPCatalogServer,
		type PendingCompositeAuth,
		type VMCP
	} from '$lib/services';
	import { getMCPDisplayName, isDeprecatedMCPServer } from '$lib/services/user/mcp';
	import McpDeprecatedNotice from './McpDeprecatedNotice.svelte';
	import { Server } from '@lucide/svelte';
	import { onMount } from 'svelte';

	interface Props {
		compositeMcpId: string;
		vmcpId?: string;
		oauthAuthRequestId?: string;
		onComplete?: () => void;
	}

	let { compositeMcpId, vmcpId, oauthAuthRequestId, onComplete }: Props = $props();

	type PendingItem = PendingCompositeAuth & {
		loading?: boolean;
	};

	type OAuthParent = MCPCatalogServer | VMCP;
	type ComponentSource = {
		id?: string;
		manifest?: { name?: string; icon?: string; metadata?: { deprecated?: string } };
		disabled?: boolean;
	};

	const metadataId = $derived(vmcpId || compositeMcpId);
	const isVMCP = $derived(metadataId.startsWith('vmcp1'));

	let compositeServer = $state<OAuthParent>();
	let componentInfos = $state<
		Record<string, { name?: string; icon?: string; deprecated?: boolean }>
	>({});
	let pending = $state<PendingItem[]>([]);
	let loading = $state(true);
	let error = $state<string>('');

	const allAuthenticated = $derived(pending.length === 0);
	const componentSources = $derived(getComponentSources(compositeServer));
	const enabledCount = $derived(
		componentSources.filter((component) => isVMCP || component.disabled !== true).length
	);
	const parentIcon = $derived(
		isVMCP && compositeServer && 'components' in compositeServer
			? compositeServer.icon
			: !isVMCP && compositeServer && 'manifest' in compositeServer
				? compositeServer.manifest.icon
				: undefined
	);
	const parentDisplayName = $derived(
		isVMCP && compositeServer && 'displayName' in compositeServer
			? compositeServer.displayName
			: getMCPDisplayName(
					!isVMCP && compositeServer && 'manifest' in compositeServer ? compositeServer : undefined,
					'MCP Server Authentication'
				)
	);

	// trigger onComplete when done
	$effect(() => {
		if (onComplete && allAuthenticated && !loading && !error) {
			onComplete();
		}
	});

	function getComponentSources(parent?: OAuthParent): ComponentSource[] {
		if (!parent) return [];
		if ('components' in parent) {
			return parent.components.map((component) => ({
				id: component.mcpServerCatalogEntryID || component.id,
				manifest: component.catalogEntry.manifest
			}));
		}
		return (parent.manifest?.compositeConfig?.componentServers || []).map((component) => ({
			id: component.catalogEntryID,
			manifest: component.manifest,
			disabled: component.disabled
		}));
	}

	async function fetchParentAndMeta() {
		try {
			const parent = await UserService.getMCPServerOrVMCP(metadataId);
			compositeServer = parent;

			componentInfos = getComponentSources(parent).reduce(
				(
					acc: Record<string, { name?: string; icon?: string; deprecated?: boolean }>,
					c: ComponentSource
				) => {
					const id = c.id;
					if (!id) return acc;
					acc[id] = {
						name: c.manifest?.name,
						icon: c.manifest?.icon,
						deprecated: isDeprecatedMCPServer(c)
					};
					return acc;
				},
				{}
			);
		} catch (_err) {
			// ignore; UI will fallback to IDs
		}
	}

	async function fetchPending() {
		loading = true;
		error = '';
		try {
			const data = await UserService.checkCompositeOAuth(compositeMcpId, {
				oauthAuthRequestID: oauthAuthRequestId
			});
			pending = data.map((d) => ({ ...d }));
		} catch (_err) {
			const { message } = parseErrorContent(_err);
			error = message;
		} finally {
			loading = false;
		}
	}

	function setItemLoading(id: string, value: boolean) {
		pending = pending.map((p) => (p.mcpServerID === id ? { ...p, loading: value } : p));
	}

	async function skip(id: string) {
		if (isVMCP) return;
		setItemLoading(id, true);
		try {
			const item = pending.find((p) => p.mcpServerID === id);
			if (!item || !item.catalogEntryID) return;

			// Prevent disabling the last enabled component (no banner; button is hidden/disabled)
			if (enabledCount <= 1) return;

			// Use configure endpoint to set disabled=true for this component
			const payload: Record<string, { config: Record<string, string>; disabled: boolean }> = {
				[item.catalogEntryID]: { config: {}, disabled: true }
			};
			await UserService.configureCompositeMcpServer(compositeMcpId, payload);

			// Re-check pending from server; item should disappear
			await fetchParentAndMeta();
			await fetchPending();
		} catch (err) {
			const { message } = parseErrorContent(err);
			error = message;
		} finally {
			setItemLoading(id, false);
		}
	}

	function handleVisibilityChange() {
		if (document.visibilityState === 'visible') {
			fetchPending();
		}
	}

	onMount(() => {
		fetchParentAndMeta();
		fetchPending();
		document.addEventListener('visibilitychange', handleVisibilityChange);
		return () => document.removeEventListener('visibilitychange', handleVisibilityChange);
	});
</script>

<div class="colors-background flex min-h-screen items-center justify-center p-4">
	<div class="popover w-full max-w-lg p-6">
		<div class="mb-6 flex items-center gap-3">
			<div class="bg-base-200 shrink-0 rounded-md p-2">
				{#if parentIcon}
					<img src={parentIcon} alt={parentDisplayName} class="size-8" />
				{:else}
					<Server class="size-8" />
				{/if}
			</div>
			<h1 class="text-2xl font-semibold">
				{parentDisplayName}
			</h1>
		</div>

		{#if !allAuthenticated}
			<p class="mb-6 text-sm">
				This composite MCP server requires authentication with multiple services. Please
				authenticate with each service below.
			</p>
		{/if}

		{#if loading && pending.length === 0}
			<div class="flex items-center justify-center gap-2 py-8">
				<Loading class="size-6" />
				<span>Loading servers...</span>
			</div>
		{:else if error}
			<div class="notification-error">
				{error}
			</div>
		{:else}
			<div class="flex flex-col gap-4">
				{#each pending as item (item.mcpServerID)}
					<div
						class="border-base-400 bg-base-200 flex items-center justify-between rounded-lg border p-4"
					>
						<div class="flex items-center gap-3">
							{#if item.icon || componentInfos[item.catalogEntryID || '']?.icon}
								<img
									src={item.icon || componentInfos[item.catalogEntryID || '']?.icon}
									alt="icon"
									class="size-6"
								/>
							{:else}
								<Server class="size-6" />
							{/if}
							<span class="text-base font-medium"
								>{item.name ||
									componentInfos[item.catalogEntryID || '']?.name ||
									item.mcpServerID}</span
							>
							<McpDeprecatedNotice
								deprecated={componentInfos[item.catalogEntryID || '']?.deprecated}
								child
							/>
						</div>
						<div class="flex items-center gap-2">
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external OAuth URL -->
							<a href={item.authURL} rel="external" target="_blank" class="btn btn-primary"
								>Authenticate</a
							>
							{#if !isVMCP && enabledCount > 1}
								<button
									class="btn btn-text"
									disabled={item.loading}
									onclick={() => skip(item.mcpServerID)}
								>
									{#if item.loading}
										<Loading class="size-4" />
									{:else}
										Skip
									{/if}
								</button>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}

		{#if allAuthenticated}
			<div class="notification-info mt-6 flex justify-center">
				<div class="flex flex-col items-center gap-2">
					<p class="text-center font-semibold">All services authenticated successfully!</p>
					<p class="text-center text-sm font-light">
						You can close this window and return to the application.
					</p>
				</div>
			</div>
		{/if}
	</div>
</div>
