<script lang="ts">
	import {
		ChatService,
		type McpServerResource,
		type Project,
		type ProjectMCP,
		type File
	} from '$lib/services';
	import { getProjectMCPs, type ProjectMcpItem } from '$lib/context/projectMcps.svelte';
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
	import { poll } from '$lib/utils';
	import { onMount } from 'svelte';

	interface Props {
		project: Project;
		threadID?: string;
		currentThreadFiles?: File[];
	}

	type ServerResources = {
		mcp: ProjectMcpItem;
		resources: McpServerResource[];
	};

	let { project, threadID = $bindable(), currentThreadFiles = $bindable() }: Props = $props();
	let dialog = $state<HTMLDialogElement>();
	let loading = $state(false);
	let projectResources = $state<ServerResources[]>([]);
	let targetResourceUri = $state('');
	let searchQuery = $state('');
	let searchInput = $state<HTMLInputElement>();
	let loadingFiles = $state(false);

	const fileExtensions: Record<string, string> = {
		'text/plain': 'txt',
		'text/markdown': 'md',
		'application/javascript': 'js',
		'application/typescript': 'ts',
		'application/octet-stream': 'bin'
	};

	// Sort MCP servers lexicographically by name + mcpID
	let projectMcps = $derived(
		[...getProjectMCPs().items].sort((a, b) => getMcpFQN(a).localeCompare(getMcpFQN(b)))
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
						fuzzyMatch(searchQuery, getMcpFQN(serverResources.mcp))
				)
			}))
			.filter(
				(serverResources) =>
					fuzzyMatch(searchQuery, getMcpFQN(serverResources.mcp)) ||
					serverResources.resources.length > 0
			)
	);

	function fetchProjectResources() {
		loading = true;
		projectResources = [];
		for (const mcp of projectMcps) {
			if (!mcp.authenticated) {
				// Omit unauthenticated MCP servers
				continue;
			}

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
			errors.append(`Failed to load thread files: ${err}`);
			currentThreadFiles = [];
		}
		loadingFiles = false;
	}

	function getMcpFQN(mcp: ProjectMCP) {
		return (mcp.name ?? '') + '-' + mcp.id;
	}

	function getFilename(mcp: ProjectMCP, resourceName: string, mimeType: string) {
		const extension = fileExtensions[mimeType] ?? mimeType.split('/')?.[1] ?? 'txt';
		const filename = `obot-${getMcpFQN(mcp)}-resource-${resourceName}.${extension}`;
		return filename;
	}

	function resourceFileExists(resource: McpServerResource, mcp: ProjectMCP) {
		const filename = getFilename(mcp, resource.name, resource.mimeType);
		return currentThreadFiles?.some((file) => file.name.startsWith(filename)) ?? false;
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
			const filename = getFilename(mcp, resource.name, response.mimeType);

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
		if (!file) {
			errors.append(`Failed to download resource from MCP server`);
			return;
		}

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

	// Creates a new thread, wait for it to be ready, and updates the threadID prop.
	async function initThread() {
		let thread = await ChatService.createThread(project.assistantID, project.id);
		await poll(async () => {
			thread = await ChatService.getThread(project.assistantID, project.id, thread.id);
			return thread.ready === true;
		});
		threadID = thread.id;
	}

	async function addResource(resource: McpServerResource, mcp: ProjectMCP) {
		try {
			if (!threadID) {
				// No thread ID given, this can happen when a user/admin navigates to chat from
				// the "Account Dropdown". Create a new thread. Since the threadID prop is bindable,
				// the change will propagate to the parent component.
				await initThread();
			}

			const file = await getResourceFile(resource, mcp);
			if (!file) {
				errors.append(`Failed to get resource file`);
				return;
			}

			await ChatService.saveFile(project.assistantID, project.id, file, { threadID });
			await loadThreadFiles();
		} catch (err) {
			errors.append(`Failed to save resource file to thread workspace: ${err}`);
		} finally {
			targetResourceUri = '';
		}
	}

	async function removeResource(resource: McpServerResource, mcp: ProjectMCP) {
		try {
			if (!threadID) return;
			const filename = getFilename(mcp, resource.name, resource.mimeType);
			await ChatService.deleteFile(project.assistantID, project.id, filename, { threadID });
			await loadThreadFiles();
		} catch (err) {
			errors.append(`Failed to remove resource file from thread workspace: ${err}`);
		} finally {
			targetResourceUri = '';
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

	onMount(() => {
		fetchProjectResources();
	});
</script>

{#if (!loading && filteredResources.length > 0) || loading}
	<button
		class="button mt-3 -mr-3 -mb-3 flex min-h-9 items-center justify-end gap-1 text-sm"
		onclick={open}
	>
		{#if loading}
			<LoaderCircle class="size-4 animate-spin" />
		{:else}
			<HardDrive class="size-4" />
			Add from MCP Servers
		{/if}
	</button>
{/if}

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
				MCP Server Resources
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
				<Search class="text-on-surface1 absolute top-1/2 left-3 size-4 -translate-y-1/2" />
				<input
					bind:this={searchInput}
					bind:value={searchQuery}
					type="text"
					placeholder="Search by MCP server or resource name..."
					class="focus:border-primary focus:ring-primary dark:focus:border-primary/50 bg-background dark:text-on-background w-full rounded-lg border border-gray-300 py-2 pr-4 pl-10 text-sm focus:ring-1 focus:outline-none dark:border-gray-600 dark:bg-gray-800"
				/>
			</div>
		</div>

		<div
			class="default-scrollbar-thin bg-surface1 flex flex-1 flex-col overflow-y-auto p-2 dark:bg-gray-950"
		>
			{#if loading}
				<div class="flex h-full flex-col items-center justify-center">
					<LoaderCircle class="size-6 animate-spin" />
					<p class="text-on-surface1 mt-2 text-sm">Loading resources...</p>
				</div>
			{:else if filteredResources.length === 0}
				<div class="flex h-full flex-col items-center justify-center">
					<HardDrive class="size-12 text-gray-300" />
					<p class="text-on-surface1 mt-2 text-sm">
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
								{name} ({mcp.id})
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
												targetResourceUri = resource.uri;
												if (!alreadyAdded) {
													addResource(resource, mcp);
													return;
												}
												removeResource(resource, mcp);
											}}
											disabled={loadingFiles || targetResourceUri !== ''}
										>
											<div>
												<p class="text-sm">{resource.name}</p>
												<p class="text-on-surface1 text-xs font-light">{resource.mimeType}</p>
											</div>
											<div class="flex grow"></div>
											{#if alreadyAdded}
												<div class="button-text flex items-center gap-1 p-2 pr-0 text-xs">
													{#if targetResourceUri === resource.uri}
														<LoaderCircle class="size-3 animate-spin" />
													{:else}
														Remove from thread files <ChevronsRight class="size-3" />
													{/if}
												</div>
											{:else}
												<div class="button-text flex items-center gap-1 p-2 pr-0 text-xs">
													{#if loadingFiles || targetResourceUri === resource.uri}
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
								{#if mcp.authenticated}
									<p class="text-on-surface1 text-sm">No resources available</p>
								{:else}
									<p class="text-on-surface1 text-sm">Authentication required to view resources</p>
								{/if}
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
