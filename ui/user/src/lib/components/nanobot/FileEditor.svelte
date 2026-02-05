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

	// Resizable width state
	let containerRef = $state<HTMLDivElement | null>(null);
	let widthPercent = $state(50); // Initial width: 50%
	let isResizing = $state(false);

	const MIN_WIDTH_PX = 500;
	const MAX_WIDTH_PERCENT = 65;
	const MIN_WIDTH_PERCENT = 10;

	function handleResizeStart(e: MouseEvent) {
		e.preventDefault();
		isResizing = true;

		const startX = e.clientX;
		const startWidth = widthPercent;

		function onMouseMove(e: MouseEvent) {
			if (!containerRef?.parentElement) return;

			const parentWidth = containerRef.parentElement.clientWidth;
			const deltaX = startX - e.clientX;
			const deltaPercent = (deltaX / parentWidth) * 100;
			let newPercent = startWidth + deltaPercent;

			// Calculate min percent based on MIN_WIDTH_PX
			const minPercentFromPx = (MIN_WIDTH_PX / parentWidth) * 100;
			const effectiveMinPercent = Math.max(MIN_WIDTH_PERCENT, minPercentFromPx);

			// Clamp to min/max
			newPercent = Math.max(effectiveMinPercent, Math.min(MAX_WIDTH_PERCENT, newPercent));
			widthPercent = newPercent;
		}

		function onMouseUp() {
			isResizing = false;
			document.removeEventListener('mousemove', onMouseMove);
			document.removeEventListener('mouseup', onMouseUp);
		}

		document.addEventListener('mousemove', onMouseMove);
		document.addEventListener('mouseup', onMouseUp);
	}

	function handleResizeKeydown(e: KeyboardEvent) {
		if (!containerRef?.parentElement) return;

		const parentWidth = containerRef.parentElement.clientWidth;
		const minPercentFromPx = (MIN_WIDTH_PX / parentWidth) * 100;
		const effectiveMinPercent = Math.max(MIN_WIDTH_PERCENT, minPercentFromPx);
		const step = 2; // 2% per key press

		if (e.key === 'ArrowLeft') {
			e.preventDefault();
			widthPercent = Math.min(MAX_WIDTH_PERCENT, widthPercent + step);
		} else if (e.key === 'ArrowRight') {
			e.preventDefault();
			widthPercent = Math.max(effectiveMinPercent, widthPercent - step);
		}
	}

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
	bind:this={containerRef}
	class="relative h-[calc(100dvh-4rem)] overflow-hidden transition-[opacity] duration-300 ease-out {mounted
		? 'opacity-100'
		: 'opacity-0'}"
	style="width: {mounted ? widthPercent : 0}%; min-width: {mounted
		? MIN_WIDTH_PX
		: 0}px; max-width: {MAX_WIDTH_PERCENT}%;"
>
	<!-- Resize handle -->
	<div
		class="hover:bg-base-300/75 absolute top-0 left-0 z-10 h-full w-1 cursor-ew-resize transition-colors {isResizing
			? 'bg-base-300/75'
			: 'bg-transparent'}"
		onmousedown={handleResizeStart}
		onkeydown={handleResizeKeydown}
		role="slider"
		aria-orientation="horizontal"
		aria-valuenow={widthPercent}
		aria-valuemin={MIN_WIDTH_PERCENT}
		aria-valuemax={MAX_WIDTH_PERCENT}
		aria-label="Resize file editor"
		tabindex="0"
	></div>

	<div class="bg-base-200 flex h-full w-full flex-col">
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
