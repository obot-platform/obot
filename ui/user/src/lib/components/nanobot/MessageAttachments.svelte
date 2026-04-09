<script module lang="ts">
	export function getFileIcon(type?: string) {
		if (type?.startsWith('image/')) {
			return '🖼️';
		} else if (type === 'application/pdf') {
			return '📄';
		} else if (type?.includes('text/') || type?.includes('json')) {
			return '📝';
		} else if (type?.includes('spreadsheet') || type?.includes('csv')) {
			return '📊';
		} else if (type?.includes('document')) {
			return '📋';
		}
		return '📎';
	}
</script>

<script lang="ts">
	import FileItem from '$lib/components/nanobot/FileItem.svelte';
	import { X } from 'lucide-svelte';

	let {
		uploadingFiles = [],
		uploadedFiles = [],
		cancelUpload,
		removeSelectedResource,
		selectedResources = []
	} = $props();
</script>

{#snippet item<T>(
	label: string,
	_type: string,
	loading: boolean,
	name: string,
	id: T,
	onClick?: (id: T) => void
)}
	<div class="border-base-300 flex items-center gap-1.5 rounded-full border px-2 py-1 text-xs">
		{#if loading}
			<span class="loading loading-xs loading-spinner"></span>
		{:else}
			<FileItem uri={name} compact />
		{/if}
		<span class="max-w-28 truncate">{name}</span>
		<button
			type="button"
			onclick={() => onClick?.(id)}
			class="btn btn-ghost btn-xs h-4 w-4 rounded-full p-0"
			aria-label={label}
		>
			<X class="h-2.5 w-2.5" />
		</button>
	</div>
{/snippet}

{#if uploadedFiles.length > 0 || uploadingFiles.length > 0 || selectedResources.length > 0}
	<div class="flex flex-wrap gap-2">
		<!-- Uploading files with spinner -->
		{#each uploadingFiles as uploadingFile (uploadingFile.id)}
			{@render item(
				'Cancel upload',
				'',
				true,
				uploadingFile.file.name,
				uploadingFile.id,
				cancelUpload
			)}
		{/each}
		<!-- Uploaded files -->
		{#each uploadedFiles as uploadedFile (uploadedFile.id)}
			{@render item(
				'Remove file',
				uploadedFile.file.type,
				false,
				uploadedFile.file.name,
				uploadedFile.id,
				cancelUpload
			)}
		{/each}
		<!-- Selected resources -->
		{#each selectedResources as resource (resource.uri)}
			{@render item(
				'Remove resource',
				resource.mimeType || '',
				false,
				resource.title || resource.name,
				resource,
				removeSelectedResource
			)}
		{/each}
	</div>
{/if}
