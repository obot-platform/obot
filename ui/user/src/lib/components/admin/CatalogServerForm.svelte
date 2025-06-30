<script lang="ts">
	import {
		type MCPCatalogEntry,
		type MCPCatalogEntryFormData,
		type MCPCatalogEntryServerManifest,
		type MCPCatalogServerManifest
	} from '$lib/services/admin/types';
	import { Info, Plus, Trash2 } from 'lucide-svelte';
	import SingleMultiMcpForm from '../mcp/SingleMultiMcpForm.svelte';
	import RemoteMcpForm from '../mcp/RemoteMcpForm.svelte';
	import { AdminService, type MCPCatalogServer } from '$lib/services';
	import { onMount, type Snippet } from 'svelte';

	interface Props {
		catalogId?: string;
		entry?: MCPCatalogEntry | MCPCatalogServer;
		type?: 'single' | 'multi' | 'remote';
		readonly?: boolean;
		onCancel?: () => void;
		onSubmit?: () => void;
		hideTitle?: boolean;
		readonlyMessage?: Snippet;
	}

	function getType(entry?: MCPCatalogEntry | MCPCatalogServer) {
		if (!entry) return undefined;
		if ('manifest' in entry) {
			return 'multi';
		} else if ('commandManifest' in entry || 'urlManifest' in entry) {
			return entry.commandManifest ? 'single' : 'remote';
		}
	}

	let {
		catalogId,
		entry,
		readonly,
		type: newType = 'single',
		onCancel,
		onSubmit,
		hideTitle,
		readonlyMessage
	}: Props = $props();
	let type = $derived(getType(entry) ?? newType);

	function convertToFormData(item?: MCPCatalogEntry | MCPCatalogServer): MCPCatalogEntryFormData {
		if (!item) {
			return {
				categories: [''],
				name: '',
				description: '',
				env: [],
				args: [''],
				command: '',
				fixedURL: '',
				url: '',
				headers: [],
				icon: ''
			};
		}

		if (item.type === 'mcpserver') {
			const server = item as MCPCatalogServerManifest;
			return {
				categories: server.manifest.metadata?.categories?.split(',') ?? [],
				icon: server.manifest.icon ?? '',
				name: server.manifest.name ?? '',
				description: server.manifest.description ?? '',
				env:
					server.manifest.env?.map((env) => ({
						...env,
						value: ''
					})) ?? [],
				args: server.manifest.args ?? [],
				command: server.manifest.command ?? '',
				url: server.manifest.url ?? '',
				headers:
					server.manifest.headers?.map((header) => ({
						...header,
						value: ''
					})) ?? []
			};
		} else {
			const entry = item as MCPCatalogEntry;
			return {
				categories:
					entry.commandManifest?.metadata?.categories?.split(',') ??
					entry.urlManifest?.metadata?.categories?.split(',') ??
					[],
				name: entry.commandManifest?.name ?? entry.urlManifest?.name ?? '',
				icon: entry.commandManifest?.icon ?? entry.urlManifest?.icon ?? '',
				env:
					(entry.commandManifest?.env ?? entry.urlManifest?.env ?? []).map((env) => ({
						...env,
						value: ''
					})) ?? [],
				description: entry.commandManifest?.description ?? entry.urlManifest?.description ?? '',
				args: entry.commandManifest?.args ?? entry.urlManifest?.args ?? [],
				command: entry.commandManifest?.command ?? entry.urlManifest?.command ?? '',
				fixedURL: entry.commandManifest?.fixedURL ?? entry.urlManifest?.fixedURL ?? '',
				hostname: entry.commandManifest?.hostname ?? entry.urlManifest?.hostname ?? '',
				headers:
					(entry.commandManifest?.headers ?? entry.urlManifest?.headers ?? []).map((header) => ({
						...header,
						value: ''
					})) ?? []
			};
		}
	}
	let formData = $state<MCPCatalogEntryFormData>(convertToFormData(entry));

	async function revealCatalogServer(catalogId: string, entryId: string) {
		try {
			const response = await AdminService.revealMcpCatalogServer(catalogId, entryId);
			formData.env = formData.env?.map((env) => ({
				...env,
				value: response[env.key] ?? ''
			}));
			formData.headers = formData.headers?.map((header) => ({
				...header,
				value: response[header.key] ?? ''
			}));
		} catch (error) {
			if (error instanceof Error && error.message.includes('404')) {
				// ignore, 404 means no credentials were set
				return;
			}
		}
	}

	onMount(() => {
		if ((type === 'multi' || type === 'remote') && entry && catalogId) {
			revealCatalogServer(catalogId, entry.id);
		}
	});

	function convertCategoriesToMetadata(categories: string[]) {
		const validCategories = categories.filter((c) => c);
		return validCategories
			? {
					metadata: {
						categories: validCategories.join(',')
					}
				}
			: undefined;
	}

	function convertToEntryManifest(
		formData: MCPCatalogEntryFormData
	): MCPCatalogEntryServerManifest {
		const { categories, ...rest } = formData;
		return {
			...rest,
			...convertCategoriesToMetadata(categories)
		};
	}

	function convertToServerManifest(formData: MCPCatalogEntryFormData): MCPCatalogServerManifest {
		const { categories, ...rest } = formData;
		return {
			manifest: {
				...rest,
				...convertCategoriesToMetadata(categories)
			}
		};
	}

	async function handleEntrySubmit(catalogId: string) {
		const manifest = convertToEntryManifest(formData);

		let response: MCPCatalogEntry;
		if (entry) {
			response = await AdminService.updateMCPCatalogEntry(catalogId, entry.id, manifest);
		} else {
			response = await AdminService.createMCPCatalogEntry(catalogId, manifest);
		}

		// TODO: header fixed values
		return response;
	}

	async function handleServerSubmit(catalogId: string) {
		const serverManifest = convertToServerManifest(formData);

		let response: MCPCatalogServer;
		if (entry) {
			response = await AdminService.updateMCPCatalogServer(
				catalogId,
				entry.id,
				serverManifest.manifest
			);
		} else {
			response = await AdminService.createMCPCatalogServer(catalogId, serverManifest);
		}

		let envValues: Record<string, string> = {};
		if (serverManifest.manifest.env) {
			envValues = Object.fromEntries(
				serverManifest.manifest.env.map((env) => [env.key, env.value])
			);
		}

		if (Object.keys(envValues).length > 0) {
			await AdminService.configureMCPCatalogServer(catalogId, response.id, envValues);
		}
		return response;
	}

	async function handleSubmit() {
		if (!catalogId) return;
		const handleFns = {
			single: handleEntrySubmit,
			multi: handleServerSubmit,
			remote: handleEntrySubmit
		};
		await handleFns[type]?.(catalogId);
		onSubmit?.();
	}
</script>

{#if !hideTitle}
	<h1 class="text-2xl font-semibold capitalize">
		{#if entry}
			{formData.name}
		{:else}
			Create {type} Server
		{/if}
	</h1>
{/if}

<div
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-8 rounded-lg border border-transparent bg-white p-4 shadow-sm"
>
	<div class="flex flex-col gap-8">
		{#if readonly && readonlyMessage}
			<div class="notification-info p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6" />
					<div>
						{@render readonlyMessage()}
					</div>
				</div>
			</div>
		{/if}

		<div class="flex flex-col gap-1">
			<label for="name" class="text-sm font-light capitalize">Name</label>
			<input
				type="text"
				id="name"
				bind:value={formData.name}
				class="text-input-filled dark:bg-black"
				disabled={readonly}
			/>
		</div>

		<div class="flex flex-col gap-1">
			<label for="name" class="text-sm font-light capitalize">Description</label>
			<input
				type="text"
				id="name"
				bind:value={formData.description}
				class="text-input-filled dark:bg-black"
				disabled={readonly}
			/>
		</div>

		<div class="flex flex-col gap-1">
			<label for="icon" class="text-sm font-light capitalize">Icon URL</label>
			<input
				type="text"
				id="icon"
				bind:value={formData.icon}
				class="text-input-filled dark:bg-black"
				disabled={readonly}
			/>
		</div>

		<div class="flex flex-col gap-1">
			<span class="text-sm font-light capitalize">Categories</span>
			{#each formData.categories as _category, index}
				<div class="flex w-full items-center gap-2">
					<div class="flex grow items-center gap-2">
						<input
							type="text"
							id={`category-${index}`}
							bind:value={formData.categories[index]}
							class="text-input-filled dark:bg-black"
							disabled={readonly}
						/>
					</div>
					{#if !readonly}
						<button class="icon-button" onclick={() => formData.categories.splice(index, 1)}>
							<Trash2 class="size-4" />
						</button>
					{/if}
				</div>
			{/each}
			{#if !readonly}
				<div class="mt-3 flex justify-end">
					<button
						class="button flex items-center gap-1 text-xs"
						onclick={() => formData.categories.push('')}
					>
						<Plus class="size-4" /> Category
					</button>
				</div>
			{/if}
		</div>
	</div>
</div>

{#if type === 'single'}
	<SingleMultiMcpForm bind:config={formData} {readonly} type="single" />
{:else if type === 'multi'}
	<SingleMultiMcpForm bind:config={formData} {readonly} type="multi" />
{:else if type === 'remote'}
	<RemoteMcpForm bind:config={formData} {readonly} />
{/if}

{#if !readonly}
	<div
		class="bg-surface1 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8 dark:bg-black"
	>
		<button class="button flex items-center gap-1" onclick={() => onCancel?.()}> Cancel </button>
		<button class="button-primary flex items-center gap-1" onclick={handleSubmit}>
			{entry ? 'Update' : 'Save'}
		</button>
	</div>
{/if}
