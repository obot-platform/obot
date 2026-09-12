<script lang="ts">
	import { isAbortError, parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type PendingCompositeAuth, type VMCP } from '$lib/services';
	import { isDeprecatedMCPServer } from '$lib/services/user/mcp';
	import McpDeprecatedNotice from './McpDeprecatedNotice.svelte';
	import { Server } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		compositeMcpId: string;
		vmcpId?: string;
		oauthAuthRequestId?: string;
		onComplete?: () => void;
		class?: string;
	}

	let {
		compositeMcpId,
		vmcpId,
		oauthAuthRequestId,
		onComplete,
		class: className
	}: Props = $props();

	type OAuthParent = VMCP;
	const metadataId = $derived(vmcpId || compositeMcpId);
	let compositeServer = $state<OAuthParent>();
	let componentInfos = $state<
		Record<string, { name?: string; icon?: string; deprecated?: boolean }>
	>({});
	let pending = $state<PendingCompositeAuth[]>([]);
	let loading = $state(true);
	let error = $state<string>('');
	const attempted = new SvelteSet<string>();
	const checking = new SvelteSet<string>();
	let rowErrors = $state<Record<string, string>>({});
	let completed = false;
	let abortController: AbortController | undefined;

	const allAuthenticated = $derived(pending.length === 0);
	const success = $derived(allAuthenticated && !loading && !error && checking.size === 0);
	const parentIcon = $derived(compositeServer ? compositeServer.icon : undefined);
	const parentDisplayName = $derived(
		compositeServer ? compositeServer.displayName : 'MCP Server Authentication'
	);

	// Complete only after every pending or in-flight authentication check succeeds.
	$effect(() => {
		if (completed || !success) return;
		completed = true;
		if (oauthAuthRequestId) {
			window.location.href = `/auth/oauth/complete/${encodeURIComponent(oauthAuthRequestId)}`;
		} else {
			onComplete?.();
		}
	});

	function getComponentSources(parent?: OAuthParent) {
		if (!parent) return [];
		return parent.components.map((component) => ({
			id: component.mcpServerCatalogEntryID || component.id,
			manifest: component.catalogEntry.manifest
		}));
	}

	async function fetchParentAndMeta(signal?: AbortSignal) {
		try {
			const parent = await UserService.getMCPServerOrVMCP(metadataId, { signal });
			if (signal?.aborted) return;
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
		} catch (err) {
			if (signal?.aborted || isAbortError(err)) return;
			// ignore; UI will fallback to IDs
		}
	}

	async function fetchPending(signal?: AbortSignal) {
		loading = true;
		error = '';
		try {
			const data = await UserService.checkCompositeOAuth(compositeMcpId, {
				oauthAuthRequestID: oauthAuthRequestId,
				signal
			});
			if (signal?.aborted) return;
			pending = data;
		} catch (err) {
			if (signal?.aborted || isAbortError(err)) return;
			const { message } = parseErrorContent(err);
			error = message;
		} finally {
			if (!signal?.aborted) loading = false;
		}
	}

	async function checkComponent(item: PendingCompositeAuth) {
		const id = item.mcpServerID;
		if (checking.has(id)) return;
		checking.add(id);
		delete rowErrors[id];
		rowErrors = { ...rowErrors };
		try {
			const result = await UserService.checkCompositeOAuthComponent(compositeMcpId, id, {
				oauthAuthRequestID: oauthAuthRequestId,
				signal: abortController?.signal
			});
			if (abortController?.signal.aborted) return;
			if (!result.authURL) {
				pending = pending.filter((candidate) => candidate.mcpServerID !== id);
			} else {
				pending = pending.map((candidate) =>
					candidate.mcpServerID === id ? { ...candidate, authURL: result.authURL! } : candidate
				);
			}
		} catch (err) {
			if (abortController?.signal.aborted || isAbortError(err)) return;
			const { message } = parseErrorContent(err);
			rowErrors = { ...rowErrors, [id]: message };
		} finally {
			checking.delete(id);
		}
	}

	function recordAttempt(item: PendingCompositeAuth) {
		attempted.add(item.mcpServerID);
	}

	function handleVisibilityChange() {
		if (document.visibilityState === 'visible') {
			if (error) {
				void fetchPending(abortController?.signal);
				return;
			}
			for (const item of pending) {
				if (attempted.has(item.mcpServerID)) void checkComponent(item);
			}
		}
	}

	onMount(() => {
		abortController = new AbortController();
		void fetchParentAndMeta(abortController.signal);
		void fetchPending(abortController.signal);
		document.addEventListener('visibilitychange', handleVisibilityChange);
		return () => {
			abortController?.abort();
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});
</script>

<div
	class={twMerge('colors-background flex min-h-screen items-center justify-center p-4', className)}
>
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
							{#if checking.has(item.mcpServerID)}
								<span class="flex items-center gap-2 text-sm" role="status">
									<Loading class="size-4" /> Checking for valid authentication…
								</span>
							{:else}
								<a
									href={item.authURL}
									rel="external noopener noreferrer"
									target="_blank"
									class="btn btn-primary"
									onclick={() => recordAttempt(item)}>Authenticate</a
								>
								{#if rowErrors[item.mcpServerID]}
									<button
										class="btn btn-secondary"
										type="button"
										onclick={() => void checkComponent(item)}>Retry</button
									>
								{/if}
							{/if}
						</div>
					</div>
					{#if rowErrors[item.mcpServerID]}
						<p class="text-error mt-2 text-sm">{rowErrors[item.mcpServerID]}</p>
					{/if}
				{/each}
			</div>
		{/if}

		{#if success}
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
