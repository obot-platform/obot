<script lang="ts">
	import { ChevronsRight, LoaderCircle, PencilLine, Server } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import HostedMcpForm from '$lib/components/mcp/HostedMcpForm.svelte';
	import RemoteMcpForm from '$lib/components/mcp/RemoteMcpForm.svelte';
	import { DEFAULT_CUSTOM_SERVER_NAME } from '$lib/constants';
	import {
		type MCPCatalogEntry,
		type MCPCatalogEntryServerManifest
	} from '$lib/services/admin/types';
	import { onMount } from 'svelte';
	import { AdminService } from '$lib/services';

	interface Props {
		entry?: Omit<MCPCatalogEntry, 'id'> & { id?: string };
		onClose?: () => void;
		readonly?: boolean;
		catalogID?: string;
	}
	let { catalogID, entry, onClose, readonly }: Props = $props();
	let processing = $state(false);

	let config = $state<MCPCatalogEntryServerManifest>();
	let showObotHosted = $state(entry?.id ? typeof entry.commandManifest !== 'undefined' : true);
	let name = $derived(entry?.commandManifest?.server.name ?? entry?.urlManifest?.server.name ?? '');
	let iconURL = $derived(
		entry?.commandManifest?.server.icon ?? entry?.urlManifest?.server.icon ?? ''
	);

	function init(isRemote?: boolean) {
		config = {
			env: entry?.commandManifest?.server.env ?? entry?.urlManifest?.server.env ?? [],
			args: entry?.commandManifest?.server.args ?? entry?.urlManifest?.server.args ?? [],
			command: entry?.commandManifest?.server.command ?? '',
			url: entry?.urlManifest?.server.url ?? '',
			headers: entry?.urlManifest?.server.headers ?? entry?.commandManifest?.server.headers ?? [],
			name: entry?.commandManifest?.server.name ?? entry?.urlManifest?.server.name ?? ''
		};
		showObotHosted = !isRemote;
	}

	async function handleSubmit() {
		if (!config || !catalogID) return;
		processing = true;
		try {
			if (entry?.id) {
				await AdminService.updateMCPCatalogEntry(catalogID, entry.id, {
					server: config
				});
			} else {
				await AdminService.createMCPCatalogEntry(catalogID, {
					server: config
				});
			}
			onClose?.();
		} catch (e) {
			console.error(e);
		} finally {
			processing = false;
		}
	}

	onMount(() => {
		init();
	});
</script>

<div class="flex h-full w-full flex-col">
	<div class="flex w-full flex-col gap-2">
		{#if entry}
			<div class="flex items-center gap-2">
				<div class="h-fit flex-shrink-0 rounded-md bg-gray-50 p-1 dark:bg-gray-600">
					{#if iconURL}
						<img src={iconURL} alt="entry icon" class="size-6" />
					{:else}
						<Server class="size-6" />
					{/if}
				</div>
				<div class="flex w-full flex-col gap-1">
					{#if entry.id}
						<h3 class="text-2xl leading-4.5 font-semibold">
							{name || DEFAULT_CUSTOM_SERVER_NAME}
						</h3>
					{:else if config}
						<input
							class="ghost-input w-full text-2xl leading-4.5 font-semibold"
							bind:value={config.name}
							placeholder="Catalog Entry Name..."
						/>
					{/if}
				</div>
			</div>
		{/if}
	</div>

	<div class="mt-4 flex w-full flex-col gap-2 pt-4 pb-2">
		<div class="flex w-full self-center">
			<div class="flex w-full gap-1">
				<button
					class={twMerge(
						'flex-1 py-3 disabled:cursor-not-allowed disabled:opacity-50',
						showObotHosted &&
							'dark:bg-surface2 dark:border-surface3 rounded-md bg-white shadow-sm dark:border'
					)}
					onclick={() => init()}
					disabled={readonly}
				>
					Obot Hosted
				</button>
				<button
					class={twMerge(
						'flex-1 py-3 disabled:cursor-not-allowed disabled:opacity-50',
						!showObotHosted &&
							'dark:bg-surface2 dark:border-surface3 rounded-md bg-white shadow-sm dark:border'
					)}
					onclick={() => init(true)}
					disabled={readonly}
				>
					Remote
				</button>
			</div>
		</div>
	</div>

	{#if config}
		<div class="relative flex flex-col gap-4 pb-4">
			<div
				class="dark:bg-surface2 dark:border-surface3 flex w-full flex-col gap-4 self-center rounded-lg bg-white px-4 py-8 shadow-sm md:px-8 dark:border"
			>
				{#if showObotHosted}
					<HostedMcpForm bind:config custom {readonly} />
				{:else}
					<RemoteMcpForm bind:config custom {readonly} />
				{/if}
			</div>
		</div>
	{/if}

	<div class="flex grow"></div>

	{#if !readonly}
		<div class="flex w-full flex-col gap-2 self-center">
			<div class="flex justify-end">
				<button
					disabled={processing}
					class="button-primary flex items-center gap-1"
					onclick={handleSubmit}
				>
					{#if processing}
						<LoaderCircle class="size-4 animate-spin" />
					{:else}
						{entry?.id ? 'Update' : 'Save'} Entry <ChevronsRight class="size-4" />
					{/if}
				</button>
			</div>
		</div>
	{/if}
</div>
