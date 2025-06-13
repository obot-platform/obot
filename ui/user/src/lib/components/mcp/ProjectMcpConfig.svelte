<script lang="ts">
	import { ChevronsRight, LoaderCircle, PencilLine, Server } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import HostedMcpForm from '$lib/components/mcp/HostedMcpForm.svelte';
	import RemoteMcpForm from '$lib/components/mcp/RemoteMcpForm.svelte';
	import { DEFAULT_CUSTOM_SERVER_NAME } from '$lib/constants';
	import { type MCPCatalogEntry, type MCPCatalogEntryManifest } from '$lib/services/admin/types';

	interface Props {
		entry?: Omit<MCPCatalogEntry, 'id'> & { id?: string };
		onCreate?: (newEntry: MCPCatalogEntryManifest) => void;
		onUpdate?: (updatedEntry: MCPCatalogEntryManifest) => void;
		readonly?: boolean;
		iconURL?: string;
	}
	let { entry, onCreate, onUpdate, readonly, iconURL }: Props = $props();
	let processing = $state(false);

	function isObotHosted(item: MCPCatalogEntryManifest) {
		// Prioritize command presence for determining if it's Obot-hosted
		// If there's no command but there's a URL, it should be treated as remote
		if (item.command && item.command !== '') {
			return true;
		}
		if (item.url && item.url !== '') {
			return false;
		}
		// If neither command nor URL is present, fall back to checking other properties
		return (item.args?.length ?? 0) > 0;
	}

	let config = $state<MCPCatalogEntryManifest>({
		env: entry?.manifest.env ?? [],
		args: entry?.manifest.args ?? [],
		command: entry?.manifest.command ?? '',
		url: entry?.manifest.url ?? '',
		headers: entry?.manifest.headers ?? []
	});
	let showObotHosted = $state(entry?.id ? isObotHosted(entry.manifest) : true);

	function init(isRemote?: boolean) {
		config = {
			env: entry?.manifest.env ?? [],
			args: entry?.manifest.args ?? [],
			command: entry?.manifest.command ?? '',
			url: entry?.manifest.url ?? '',
			headers: entry?.manifest.headers ?? []
		};
		showObotHosted = !isRemote;
	}

	async function handleSubmit() {
		if (entry) {
			onUpdate?.(config);
		} else {
			onCreate?.(config);
		}
	}
</script>

<div class="flex h-full w-full flex-col">
	<div class="flex w-full flex-col gap-2">
		<div class="flex w-full flex-col gap-2 self-center">
			<div class="flex items-center justify-between gap-8">
				{#if entry}
					<div class="flex flex-col gap-4">
						<div class="flex max-w-sm items-center gap-2">
							<div
								class="h-fit flex-shrink-0 self-start rounded-md bg-gray-50 p-1 dark:bg-gray-600"
							>
								{#if iconURL}
									<img src={iconURL} alt="entry icon" class="size-6" />
								{:else if entry.id}
									<Server class="size-6" />
								{/if}
							</div>
							<div class="flex flex-col gap-1">
								{#if entry.id}
									<h3 class="text-2xl leading-4.5 font-semibold">
										{entry.name || DEFAULT_CUSTOM_SERVER_NAME}
									</h3>
								{:else}
									<h3 class="text-2xl leading-4.5 font-semibold">Create Catalog Entry</h3>
								{/if}
							</div>
						</div>
					</div>
				{:else}
					<h3 class="flex items-center gap-2 text-xl font-semibold">
						<PencilLine class="size-5" /> Create MCP Config
					</h3>
				{/if}
			</div>
		</div>
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
