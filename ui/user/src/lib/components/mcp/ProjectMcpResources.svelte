<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { ChatService } from '$lib/services';
	import {
		type McpServerResource,
		type McpServerResourceContent,
		type Project,
		type ProjectMCP,
		type File
	} from '$lib/services/chat/types';
	import { errors, responsive } from '$lib/stores';
	import { ChevronRight, ChevronsRight, Download, LoaderCircle, X } from 'lucide-svelte';

	interface Props {
		project: Project;
		mcp?: ProjectMCP;
		resources: McpServerResource[];
	}

	let { project, mcp, resources }: Props = $props();
	let currentWorkspaceFiles = $state<File[]>([]);
	let dialog = $state<HTMLDialogElement>();
	let loadingFiles = $state(false);
	let addingFileUri = $state('');

	async function checkFileExists(filename: string) {
		try {
			const files = await ChatService.listFiles(project.assistantID, project.id);

			const fileExists = files.items.some((file) => file.name === filename);
			return fileExists;
		} catch (err) {
			console.error('Failed to check if file exists:', err);
			return false;
		}
	}

	function getExtensionFromMimeType(mimeType: string): string {
		const mimeToExt: Record<string, string> = {
			'text/plain': 'txt',
			'text/markdown': 'md',
			'application/javascript': 'js',
			'application/typescript': 'ts',
			'application/octet-stream': 'bin'
		};

		return mimeToExt[mimeType] ?? mimeType.split('/')?.[1] ?? 'txt';
	}

	function convertResourceContentToFile(
		filename: string,
		resourceContent: McpServerResourceContent
	) {
		let content;
		if (resourceContent.text) {
			content = resourceContent.text;
		} else if (resourceContent.blob) {
			// Convert base64 to binary
			const binaryContent = atob(resourceContent.blob);
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
		return new File([content], filename, { type: resourceContent.mimeType });
	}

	function getFilename(mcpName: string, resourceName: string, mimeType: string) {
		const extension = getExtensionFromMimeType(mimeType);
		const filename = `obot-${mcpName}-resource-${resourceName}.${extension}`;
		return filename;
	}

	async function saveResourceToWorkspace(
		resourceName: string,
		resourceContent: McpServerResourceContent
	) {
		if (!mcp) return;
		try {
			const filename = getFilename(mcp.name, resourceName, resourceContent.mimeType);
			const fileExists = await checkFileExists(filename);
			if (!fileExists) {
				const file = convertResourceContentToFile(filename, resourceContent);
				return ChatService.saveFile(project.assistantID, project.id, file);
			}
		} catch (err) {
			console.error('Failed to create or open file:', err);
			errors.append('An error occurred while saving the resource to the workspace.');
		}
	}

	async function handleAddResource(resource: McpServerResource, download: boolean = false) {
		if (!project?.assistantID || !project.id || !mcp) return;
		addingFileUri = resource.uri;

		let response: McpServerResourceContent;
		try {
			response = await ChatService.readProjectMcpServerResource(
				project.assistantID,
				project.id,
				mcp.id,
				resource.uri
			);
		} catch (err) {
			console.error('Failed to read resource from MCP server:', err);
			errors.append('Failed to read resource from MCP server.');
			return;
		}

		if (!download) {
			await saveResourceToWorkspace(resource.name, response);
			loadExistingWorkspaceFiles();
		} else {
			const filename = getFilename(mcp.name, resource.name, response.mimeType);
			const file = convertResourceContentToFile(filename, response);
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

		addingFileUri = '';
	}

	function isAlreadyAdded(resource: McpServerResource) {
		if (!mcp) return false;
		const filename = `obot-${mcp.name}-resource-${resource.name}`;
		return currentWorkspaceFiles.some((file) => file.name.startsWith(filename));
	}

	async function loadExistingWorkspaceFiles() {
		loadingFiles = true;
		const files = await ChatService.listFiles(project.assistantID, project.id);
		currentWorkspaceFiles = files.items;
		loadingFiles = false;
	}

	export function open() {
		loadExistingWorkspaceFiles();
		dialog?.showModal();
	}

	export function close() {
		dialog?.close();
	}
</script>

<dialog
	bind:this={dialog}
	class="h-full w-full max-w-lg p-0 md:max-h-[75vh]"
	class:mobile-screen-dialog={responsive.isMobile}
	use:clickOutside={() => dialog?.close()}
>
	<div class="flex h-full flex-col">
		{#if mcp}
			<h4
				class="default-dialog-title px-4 py-2"
				class:default-dialog-mobile-title={responsive.isMobile}
			>
				<span class="flex items-center gap-2">
					<img src={mcp.icon} class="size-4" alt={mcp.name} />
					Resources
				</span>
				<button
					class:mobile-header-button={responsive.isMobile}
					onclick={() => {
						dialog?.close();
					}}
					class="icon-button"
				>
					{#if responsive.isMobile}
						<ChevronRight class="size-6" />
					{:else}
						<X class="size-5" />
					{/if}
				</button>
			</h4>
			<div
				class="default-scrollbar-thin bg-surface1 flex flex-1 flex-col gap-2 overflow-y-auto p-2 dark:bg-gray-950"
			>
				{#each resources as resource}
					{@const alreadyAdded = isAlreadyAdded(resource)}
					<div class="resource flex items-center gap-2">
						<button class="icon-button" onclick={() => handleAddResource(resource, true)}>
							<Download class="size-4" />
						</button>
						<button
							class="flex grow gap-4 text-left"
							onclick={() => handleAddResource(resource)}
							disabled={loadingFiles || addingFileUri === resource.uri || alreadyAdded}
						>
							<div>
								<p class="text-sm">{resource.name}</p>
								<p class="text-xs font-light text-gray-500">{resource.mimeType}</p>
							</div>
							<div class="flex grow"></div>
							{#if alreadyAdded}
								<span class="p-2 pr-0 text-xs text-gray-500">Added</span>
							{:else}
								<div class="button-text flex items-center gap-1 p-2 pr-0 text-xs">
									{#if loadingFiles || addingFileUri === resource.uri}
										<LoaderCircle class="size-3 animate-spin" />
									{:else}
										Add to Project <ChevronsRight class="size-3" />
									{/if}
								</div>
							{/if}
						</button>
					</div>
				{/each}
			</div>
		{/if}
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
