<script lang="ts">
	import type { ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import type { ResourceContents } from '$lib/services/nanobot/types';
	import { X } from 'lucide-svelte';
	import MarkdownEditor from './MarkdownEditor.svelte';
	import { isSafeImageMimeType } from '$lib/services/nanobot/utils';

	interface Props {
		filename: string;
		chat: ChatService;
		onClose: () => void;
	}

	let { filename, chat, onClose }: Props = $props();

	let resource = $state<ResourceContents | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let mounted = $state(false);

	$effect(() => {
		requestAnimationFrame(() => {
			mounted = true;
		});
	});

	$effect(() => {
		// Reset state when filename changes
		resource = null;
		loading = true;
		error = null;

		let cleanup: (() => void) | undefined;

		const loadResource = async () => {
			// Try to find resource in current list
			let match = chat.resources.find((r) => r.name === filename);

			// If not found, refresh the resources list and try again
			if (!match) {
				const refreshed = await chat.listResources({ useDefaultSession: true });
				if (refreshed?.resources) {
					match = refreshed.resources.find((r) => r.name === filename);
				}
			}

			if (!match) {
				loading = false;
				return;
			}

			try {
				const result = await chat.readResource(match.uri);
				if (result.contents?.length) {
					resource = result.contents[0];
				}
				loading = false;

				// Subscribe to live updates
				cleanup = chat.watchResource(match.uri, (updatedResource) => {
					resource = updatedResource;
				});
			} catch (e) {
				error = e instanceof Error ? e.message : String(e);
				loading = false;
			}
		};

		loadResource();

		// Cleanup subscription when component unmounts or filename changes
		return () => cleanup?.();
	});

	// Derive the content to display
	let content = $derived(resource?.text ?? '');
	let mimeType = $derived(resource?.mimeType ?? 'text/plain');
</script>

<div
	class="h-[calc(100dvh-4rem)] overflow-hidden transition-[max-width,opacity] duration-300 ease-out {mounted
		? 'max-w-[500px] opacity-100'
		: 'max-w-0 opacity-0'}"
>
	<div class="bg-base-200 flex h-full w-[500px] flex-col">
		<div class="border-base-300 flex items-center gap-2 border-b px-4 py-2">
			<div class="flex grow items-center justify-between">
				<span class="truncate text-sm font-medium">{filename}</span>
				{#if mimeType}
					<span class="text-base-content/60 text-xs">{mimeType}</span>
				{/if}
			</div>
			<button class="btn btn-sm btn-square tooltip tooltip-left" data-tip="Close" onclick={onClose}>
				<X class="size-4" />
			</button>
		</div>

		<div class="flex-1 overflow-auto p-4 pt-0">
			{#if loading}
				<div class="flex h-full items-center justify-center">
					<span class="loading loading-spinner loading-md"></span>
				</div>
			{:else if error}
				<div class="alert alert-error">
					<span>Failed to load resource: {error}</span>
				</div>
			{:else if resource?.blob}
				<!-- Binary content - show as image if possible -->
				{#if mimeType.startsWith('image/') && isSafeImageMimeType(mimeType)}
					<img
						src="data:{mimeType};base64,{resource.blob}"
						alt={filename}
						class="h-auto max-w-full"
					/>
				{:else}
					<div class="text-base-content/60">Binary content ({mimeType})</div>
				{/if}
			{:else if content}
				<MarkdownEditor value={content} />
			{:else}
				<div class="text-base-content/60 italic">The contents of this file are empty.</div>
			{/if}
		</div>
	</div>
</div>
