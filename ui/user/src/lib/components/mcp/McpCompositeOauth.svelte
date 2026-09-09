<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type PendingCompositeAuth, type VMCP } from '$lib/services';
	import { isDeprecatedMCPServer } from '$lib/services/user/mcp';
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

	type OAuthParent = VMCP;
	const metadataId = $derived(vmcpId || compositeMcpId);
	let compositeServer = $state<OAuthParent>();
	let componentInfos = $state<
		Record<string, { name?: string; icon?: string; deprecated?: boolean }>
	>({});
	let pending = $state<PendingCompositeAuth[]>([]);
	let loading = $state(true);
	let error = $state<string>('');

	const allAuthenticated = $derived(pending.length === 0);
	const parentIcon = $derived(compositeServer ? compositeServer.icon : undefined);
	const parentDisplayName = $derived(
		compositeServer ? compositeServer.displayName : 'MCP Server Authentication'
	);

	// trigger onComplete when done
	$effect(() => {
		if (onComplete && allAuthenticated && !loading && !error) {
			onComplete();
		}
	});

	function getComponentSources(parent?: OAuthParent) {
		if (!parent) return [];
		return parent.components.map((component) => ({
			id: component.mcpServerCatalogEntryID || component.id,
			manifest: component.catalogEntry.manifest
		}));
	}

	async function fetchParentAndMeta() {
		try {
			const parent = await UserService.getMCPServerOrVMCP(metadataId);
			if (!('components' in parent)) return;
			compositeServer = parent;

			componentInfos = getComponentSources(parent).reduce(
				(acc: Record<string, { name?: string; icon?: string; deprecated?: boolean }>, c) => {
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
			pending = data;
		} catch (_err) {
			const { message } = parseErrorContent(_err);
			error = message;
		} finally {
			loading = false;
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
				This vMCP requires authentication with multiple services. Please authenticate with each
				service below.
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
