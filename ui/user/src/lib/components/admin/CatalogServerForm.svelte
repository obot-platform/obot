<script lang="ts">
	import {
		type MCPCatalogEntry,
		type MCPCatalogEntryFormData,
		type MCPCatalogEntryServerManifest,
		type MCPCatalogServerManifest
	} from '$lib/services/admin/types';
	import { Plus, Trash2 } from 'lucide-svelte';
	import HostedMcpForm from '../mcp/HostedMcpForm.svelte';
	import RemoteMcpForm from '../mcp/RemoteMcpForm.svelte';
	import { AdminService, type MCPCatalogServer } from '$lib/services';
	import { onMount } from 'svelte';

	interface Props {
		catalogId?: string;
		entry?: MCPCatalogEntry | MCPCatalogServer;
		type?: 'single' | 'multi' | 'remote';
		readonly?: boolean;
		onCancel?: () => void;
		onSubmit?: () => void;
	}

	function getType(entry?: MCPCatalogEntry | MCPCatalogServer) {
		if (!entry) return undefined;
		if (entry.type === 'mcpserver' && 'command' in entry) {
			return entry.command ? 'multi' : 'remote';
		} else if ('commandManifest' in entry || 'urlManifest' in entry) {
			return 'single';
		}
	}

	let {
		catalogId,
		entry,
		readonly,
		type: newType = 'single',
		onCancel,
		onSubmit
	}: Props = $props();
	let type = $state(getType(entry) ?? newType);

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
				categories: [],
				icon: server.manifest.icon,
				name: server.manifest.name,
				description: server.manifest.description,
				env: server.manifest.env,
				args: server.manifest.args,
				command: server.manifest.command,
				url: server.manifest.url,
				headers: server.manifest.headers
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
				env: (entry.commandManifest?.env ?? entry.urlManifest?.env ?? []).map((env) => ({
					...env,
					value: ''
				})),
				description: entry.commandManifest?.description ?? entry.urlManifest?.description ?? '',
				args: entry.commandManifest?.args ?? entry.urlManifest?.args ?? [],
				command: entry.commandManifest?.command ?? entry.urlManifest?.command ?? '',
				fixedURL: entry.commandManifest?.fixedURL ?? entry.urlManifest?.fixedURL ?? '',
				hostname: entry.commandManifest?.hostname ?? entry.urlManifest?.hostname ?? '',
				headers: (entry.commandManifest?.headers ?? entry.urlManifest?.headers ?? []).map(
					(header) => ({
						...header,
						value: ''
					})
				)
			};
		}
	}
	let formData = $state<MCPCatalogEntryFormData>(convertToFormData(entry));

	onMount(async () => {
		if (entry && type === 'multi' && catalogId) {
			AdminService.revealMcpCatalogServer(catalogId, entry.id).then((response) => {
				formData.env = formData.env?.map((env) => ({
					...env,
					value: response[env.key] ?? ''
				}));
			});
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
		if (entry) {
			const response = await AdminService.updateMCPCatalogEntry(catalogId, entry.id, manifest);
			return response;
		} else {
			const response = await AdminService.createMCPCatalogEntry(catalogId, manifest);
			return response;
		}
	}

	async function handleServerSubmit(catalogId: string) {
		const manifest = convertToServerManifest(formData);
		let response: MCPCatalogServer;
		if (entry) {
			response = await AdminService.updateMCPCatalogServer(catalogId, entry.id, manifest);
		} else {
			response = await AdminService.createMCPCatalogServer(catalogId, manifest);
		}

		if (manifest.manifest.command && manifest.manifest.env && manifest.manifest.env.length > 0) {
			const envValues = Object.fromEntries(
				manifest.manifest.env.map((env) => [env.key, env.value])
			);
			await AdminService.configureMCPCatalogServer(catalogId, response.id, envValues);
		}
		return response;
	}

	async function handleSubmit() {
		if (!catalogId) return;
		const handleFns = {
			single: handleEntrySubmit,
			multi: handleServerSubmit,
			remote: handleServerSubmit
		};
		await handleFns[type]?.(catalogId);
		onSubmit?.();
	}
</script>

{#if !entry}
	<h1 class="text-2xl font-semibold capitalize">
		Create {type} Server
	</h1>
{/if}
<div
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-8 rounded-lg border border-transparent bg-white p-4 shadow-sm"
>
	<div class="flex flex-col gap-8">
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
	<HostedMcpForm bind:config={formData} {readonly} type="single" />
{:else if type === 'multi'}
	<HostedMcpForm bind:config={formData} {readonly} type="multi" />
{:else if type === 'remote'}
	<RemoteMcpForm bind:config={formData} {readonly} />
{/if}

<div
	class="bg-surface1 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8 dark:bg-black"
>
	<button class="button flex items-center gap-1" onclick={() => onCancel?.()}> Cancel </button>
	<button class="button-primary flex items-center gap-1" onclick={handleSubmit}>
		{entry ? 'Update' : 'Create'} Server
	</button>
</div>
