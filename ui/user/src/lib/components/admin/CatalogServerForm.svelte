<script lang="ts">
	import {
		type MCPCatalogEntry,
		type MCPCatalogEntryFormData,
		type MCPCatalogEntryManifest,
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
				displayName: '',
				categories: [''],
				server: {
					name: '',
					description: '',
					env: [],
					args: [''],
					command: '',
					url: '',
					headers: [],
					icon: ''
				}
			};
		}

		if (item.type === 'mcpserver') {
			const server = item as MCPCatalogServer;
			return {
				displayName: server.name,
				categories: [],
				server: {
					icon: server.icon,
					name: server.name,
					description: server.description,
					env: server.env,
					args: server.args,
					command: server.command,
					url: server.url,
					headers: server.headers
				}
			};
		} else {
			const entry = item as MCPCatalogEntry;
			return {
				displayName: entry.commandManifest?.server.name ?? entry.urlManifest?.server.name ?? '',
				categories:
					entry.commandManifest?.metadata?.categories.split(',') ??
					entry.urlManifest?.metadata?.categories.split(',') ??
					[],
				server: {
					name: entry.commandManifest?.server.name ?? entry.urlManifest?.server.name ?? '',
					icon: entry.commandManifest?.server.icon ?? entry.urlManifest?.server.icon ?? '',
					env: (entry.commandManifest?.server.env ?? entry.urlManifest?.server.env ?? []).map(
						(env) => ({
							...env,
							value: ''
						})
					),
					description:
						entry.commandManifest?.server.description ??
						entry.urlManifest?.server.description ??
						'',
					args: entry.commandManifest?.server.args ?? entry.urlManifest?.server.args ?? [],
					command: entry.commandManifest?.server.command ?? entry.urlManifest?.server.command ?? '',
					url: entry.commandManifest?.server.url ?? entry.urlManifest?.server.url ?? '',
					headers: (
						entry.commandManifest?.server.headers ??
						entry.urlManifest?.server.headers ??
						[]
					).map((header) => ({
						...header,
						value: ''
					}))
				}
			};
		}
	}
	let formData = $state<MCPCatalogEntryFormData>(convertToFormData(entry));

	onMount(async () => {
		if (entry && type === 'multi' && catalogId) {
			AdminService.revealMcpCatalogServer(catalogId, entry.id).then((response) => {
				formData.server.env = formData.server.env?.map((env) => ({
					...env,
					value: response[env.key] ?? ''
				}));
			});
		}
	});

	function convertToEntryManifest(formData: MCPCatalogEntryFormData): MCPCatalogEntryManifest {
		const { categories, ...rest } = formData;
		return {
			...rest,
			metadata: {
				categories: categories.filter((c) => c).join(',')
			},
			server: {
				...rest.server,
				name: rest.displayName
			}
		};
	}

	function convertToServerManifest(formData: MCPCatalogEntryFormData): MCPCatalogServerManifest {
		const { categories, server, ...rest } = formData;
		return {
			...rest,
			...server,
			name: rest.displayName,
			metadata: {
				categories: categories.filter((c) => c).join(',')
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

		if (manifest.command && manifest.env && manifest.env.length > 0) {
			const envValues = Object.fromEntries(manifest.env.map((env) => [env.key, env.value]));
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
		const response = await handleFns[type]?.(catalogId);
		console.log(response);
		onSubmit?.();
	}
</script>

<h1 class="text-2xl font-semibold capitalize">
	{#if entry}
		{formData.displayName}
	{:else}
		Create {type} Server
	{/if}
</h1>
<div
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-8 rounded-lg border border-transparent bg-white p-4 shadow-sm"
>
	<div class="flex flex-col gap-8">
		<div class="flex flex-col gap-1">
			<label for="name" class="text-sm font-light capitalize">Name</label>
			<input
				type="text"
				id="name"
				bind:value={formData.displayName}
				class="text-input-filled dark:bg-black"
				disabled={readonly}
			/>
		</div>

		<div class="flex flex-col gap-1">
			<label for="name" class="text-sm font-light capitalize">Description</label>
			<input
				type="text"
				id="name"
				bind:value={formData.server.description}
				class="text-input-filled dark:bg-black"
				disabled={readonly}
			/>
		</div>

		<div class="flex flex-col gap-1">
			<label for="icon" class="text-sm font-light capitalize">Icon URL</label>
			<input
				type="text"
				id="icon"
				bind:value={formData.server.icon}
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
	<HostedMcpForm bind:config={formData.server} {readonly} type="single" />
{:else if type === 'multi'}
	<HostedMcpForm bind:config={formData.server} {readonly} type="multi" />
{:else if type === 'remote'}
	<RemoteMcpForm bind:config={formData.server} {readonly} />
{/if}

<div
	class="bg-surface1 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8 dark:bg-black"
>
	<button class="button flex items-center gap-1" onclick={() => onCancel?.()}> Cancel </button>
	<button class="button-primary flex items-center gap-1" onclick={handleSubmit}>
		{entry ? 'Update' : 'Create'} Server
	</button>
</div>
