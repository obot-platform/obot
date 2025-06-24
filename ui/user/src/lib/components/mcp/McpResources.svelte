<script lang="ts">
	import {
		ChatService,
		type McpServerResource,
		type Project,
		type ProjectMCP,
		type File
	} from '$lib/services';
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import {
		ChevronRight,
		LoaderCircle,
		HardDrive,
		X,
		Search,
		Download,
		ChevronsRight,
		Server
	} from 'lucide-svelte';
	import { DEFAULT_CUSTOM_SERVER_NAME } from '$lib/constants';
	import { responsive, errors } from '$lib/stores';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';

	interface Props {
		project: Project;
		threadID?: string;
	}

	type ServerResources = {
		mcp: ProjectMCP;
		resources: McpServerResource[];
	};

	let { project, threadID }: Props = $props();
	let dialog = $state<HTMLDialogElement>();
	let loading = $state(false);
	let projectResources = $state<ServerResources[]>([]);
	let addingResourceUri = $state('');
	let removingResourceUri = $state('');
	let searchQuery = $state('');
	let searchInput = $state<HTMLInputElement>();
	let currentThreadFiles = $state<File[]>([]);
	let loadingFiles = $state(false);

	const fileExtensions: Record<string, string> = {
		'text/plain': 'txt',
		'text/markdown': 'md',
		'application/javascript': 'js',
		'application/typescript': 'ts',
		'application/octet-stream': 'bin'
	};

	// Sort MCP servers lexicographically by name
	let projectMcps = $derived(
		[...getProjectMCPs().items].sort((a, b) => a.name.localeCompare(b.name))
	);

	// Fuzzy search function
	function fuzzyMatch(query: string, text: string): boolean {
		if (!query) return true;
		const searchLower = query.toLowerCase();
		const textLower = text.toLowerCase();
		return textLower.includes(searchLower);
	}

	// Filtered resource sets based on search query
	let filteredResources = $derived(
		projectResources
			.map((serverResources) => ({
				...serverResources,
				resources: serverResources.resources.filter(
					(resource) =>
						fuzzyMatch(searchQuery, resource.name) ||
						fuzzyMatch(searchQuery, serverResources.mcp.name)
				)
			}))
			.filter(
				(serverResources) =>
					fuzzyMatch(searchQuery, serverResources.mcp.name) || serverResources.resources.length > 0
			)
	);

	function fetchProjectResources() {
		loading = true;
		projectResources = [];
		for (const mcp of projectMcps) {
			ChatService.listProjectMcpServerResources(project.assistantID, project.id, mcp.id)
				.then((resources) => {
					if (resources.length < 1) {
						return;
					}

					projectResources.push({
						mcp,
						resources
					});
				})
				.catch((error) => {
					// 424 means resources not supported
					if (!error.message.includes('424')) {
						console.error('Failed to load resources for MCP server:', mcp.id, error);
					}
				});
		}
		loading = false;
	}

	async function loadThreadFiles() {
		if (!threadID) return;
		loadingFiles = true;
		try {
			const files = await ChatService.listFiles(project.assistantID, project.id, { threadID });
			currentThreadFiles = files.items;
		} catch (err) {
			console.error('Failed to load thread files:', err);
			currentThreadFiles = [];
		}
		loadingFiles = false;
	}

	function getFilename(mcpName: string, resourceName: string, mimeType: string) {
		const extension = fileExtensions[mimeType] ?? mimeType.split('/')?.[1] ?? 'txt';
		const filename = `obot-${mcpName}-resource-${resourceName}.${extension}`;
		return filename;
	}

	function resourceFileExists(resource: McpServerResource, mcp: ProjectMCP) {
		const filename = `obot-${mcp.name}-resource-${resource.name}`;
		return currentThreadFiles.some((file) => file.name.startsWith(filename));
	}

	async function getResourceFile(
		resource: McpServerResource,
		mcp: ProjectMCP
	): Promise<globalThis.File | undefined> {
		try {
			const response = await ChatService.readProjectMcpServerResource(
				project.assistantID,
				project.id,
				mcp.id,
				resource.uri
			);
			const filename = getFilename(mcp.name, resource.name, response.mimeType);

			let content;
			if (response.text) {
				content = response.text;
			} else if (response.blob) {
				// Convert base64 to binary
				const binaryContent = atob(response.blob);
				// Convert to ArrayBuffer
				const arrayBuffer = new ArrayBuffer(binaryContent.length);
				const uint8Array = new Uint8Array(arrayBuffer);
				for (let i = 0; i < binaryContent.length; i++) {
					uint8Array[i] = binaryContent.charCodeAt(i);
				}
				content = arrayBuffer;
			} else {
				throw new Error('Resource has no content (neither text nor blob)');
			}

			return new File([content], filename, { type: response.mimeType });
		} catch (err) {
			errors.append(`Failed to read resource from MCP server: ${err}`);
		}

		return;
	}

	async function downloadResource(resource: McpServerResource, mcp: ProjectMCP) {
		const file = await getResourceFile(resource, mcp);
		if (!file) return;

		const a = document.createElement('a');
		const url = URL.createObjectURL(file);
		a.href = url;
		a.download = file.name;
		a.click();
		a.remove();

		setTimeout(() => {
			window.URL.revokeObjectURL(url);
		}, 1000);
	}

	async function addResource(resource: McpServerResource, mcp: ProjectMCP) {
		if (!threadID) return;
		console.log('Adding resource', resource, mcp);

		addingResourceUri = resource.uri;
		const file = await getResourceFile(resource, mcp);
		if (!file) {
			console.log('Failed to get resource file');
			addingResourceUri = '';
			return;
		}

		try {
			await ChatService.saveFile(project.assistantID, project.id, file, { threadID });
			await loadThreadFiles();
		} catch (err) {
			errors.append(`Failed to save resource file to thread workspace: ${err}`);
		} finally {
			addingResourceUri = '';
		}
	}

	async function removeResource(resource: McpServerResource, mcp: ProjectMCP) {
		if (!threadID) return;

		removingResourceUri = resource.uri;
		try {
			const filename = getFilename(mcp.name, resource.name, resource.mimeType);
			await ChatService.deleteFile(project.assistantID, project.id, filename, { threadID });
			await loadThreadFiles();
		} catch (err) {
			errors.append(`Failed to remove resource file from thread workspace: ${err}`);
		} finally {
			removingResourceUri = '';
		}
	}

	export function open() {
		fetchProjectResources();
		loadThreadFiles();
		dialog?.showModal();

		// Focus search input after dialog opens
		setTimeout(() => {
			searchInput?.focus();
		}, 100);
	}

	export function close() {
		dialog?.close();
		searchQuery = '';
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			close();
		}
	}
</script>

<button class="button-icon-primary" onclick={open} use:tooltip={'Add Resource'}>
	<HardDrive class="size-5" />
</button>

<dialog
	bind:this={dialog}
	class="h-full w-full max-w-2xl p-0 md:max-h-[80vh]"
	class:mobile-screen-dialog={responsive.isMobile}
	use:clickOutside={close}
	onkeydown={handleKeydown}
>
	<div class="flex h-full flex-col">
		<h4
			class="default-dialog-title px-4 py-3"
			class:default-dialog-mobile-title={responsive.isMobile}
		>
			<span class="flex items-center gap-2">
				<HardDrive class="size-4" />
				Add MCP Resource
			</span>
			<button class:mobile-header-button={responsive.isMobile} onclick={close} class="icon-button">
				{#if responsive.isMobile}
					<ChevronRight class="size-6" />
				{:else}
					<X class="size-5" />
				{/if}
			</button>
		</h4>

		<div class="border-b border-gray-200 px-4 py-3 dark:border-gray-700">
			<div class="relative">
				<Search class="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-gray-400" />
				<input
					bind:this={searchInput}
					bind:value={searchQuery}
					type="text"
					placeholder="Search resources and servers..."
					class="w-full rounded-lg border border-gray-300 bg-white py-2 pr-4 pl-10 text-sm focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-white dark:focus:border-blue-400"
				/>
			</div>
		</div>

		<div
			class="default-scrollbar-thin bg-surface1 flex flex-1 flex-col overflow-y-auto p-2 dark:bg-gray-950"
		>
			{#if loading}
				<div class="flex h-full flex-col items-center justify-center">
					<LoaderCircle class="size-6 animate-spin" />
					<p class="mt-2 text-sm text-gray-500">Loading resources...</p>
				</div>
			{:else if filteredResources.length === 0}
				<div class="flex h-full flex-col items-center justify-center">
					<HardDrive class="size-12 text-gray-300" />
					<p class="mt-2 text-sm text-gray-500">
						{searchQuery ? 'No resources found matching your search' : 'No resources available'}
					</p>
				</div>
			{:else}
				{#each filteredResources as serverResources (serverResources.mcp.id)}
					{@const mcp = serverResources.mcp}
					{@const name = mcp.name || DEFAULT_CUSTOM_SERVER_NAME}
					{@const resources = serverResources.resources}
					<div class="mb-4">
						<div class="flex grow items-center gap-1 py-2 pl-1.5">
							<div class="rounded-md bg-gray-50 p-1 dark:bg-gray-600">
								{#if mcp.icon}
									<img src={mcp.icon} alt={name} class="size-4" />
								{:else}
									<Server class="size-4" />
								{/if}
							</div>
							<p class="text-xs font-light">
								{name}
							</p>
						</div>

						{#if resources.length > 0}
							<div class="mb-2 border-b border-gray-200 dark:border-gray-700"></div>
							<div class="flex flex-col gap-2">
								{#each resources as resource (resource.uri)}
									{@const alreadyAdded = resourceFileExists(resource, mcp)}
									<div class="resource flex items-center gap-2">
										<button
											class="icon-button"
											onclick={() => downloadResource(resource, mcp)}
											use:tooltip={'Download'}
										>
											<Download class="size-4" />
										</button>
										<button
											class="flex grow gap-4 text-left"
											onclick={() => {
												if (!alreadyAdded) {
													addResource(resource, mcp);
												} else {
													removeResource(resource, mcp);
												}
											}}
											disabled={loadingFiles ||
												addingResourceUri === resource.uri ||
												removingResourceUri === resource.uri}
										>
											<div>
												<p class="text-sm">{resource.name}</p>
												<p class="text-xs font-light text-gray-500">{resource.mimeType}</p>
											</div>
											<div class="flex grow"></div>
											{#if alreadyAdded}
												<div class="button-text flex items-center gap-1 p-2 pr-0 text-xs">
													{#if removingResourceUri === resource.uri}
														<LoaderCircle class="size-3 animate-spin" />
													{:else}
														Remove from thread files <ChevronsRight class="size-3" />
													{/if}
												</div>
											{:else}
												<div class="button-text flex items-center gap-1 p-2 pr-0 text-xs">
													{#if loadingFiles || addingResourceUri === resource.uri}
														<LoaderCircle class="size-3 animate-spin" />
													{:else}
														Add to thread files <ChevronsRight class="size-3" />
													{/if}
												</div>
											{/if}
										</button>
									</div>
								{/each}
							</div>
						{:else}
							<div class="p-4 text-center">
								<p class="text-sm text-gray-500">No resources available</p>
							</div>
						{/if}
					</div>
				{/each}
			{/if}
		</div>
	</div>
</dialog>

<style lang="postcss">
	.resource {
		display: flex;
		align-items: center;
		background-color: white;
		padding: 0.5rem;
		text-align: left;
		border-radius: 0.5rem;
		box-shadow: 0 1px 2px 0 rgb(0 0 0 / 0.05);
		transition-property: color, background-color, border-color;
		transition-duration: 300ms;

		&:disabled {
			opacity: 0.5;
			cursor: default;
		}

		&:not(:disabled) {
			&:hover {
				background-color: var(--surface2);
			}
		}

		:global(.dark) & {
			background-color: var(--surface2);
			border: 1px solid var(--surface3);

			&:not(:disabled) {
				&:hover {
					background-color: var(--surface3);
				}
			}
		}
	}
</style>
