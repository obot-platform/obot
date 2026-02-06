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

	const name = $derived(filename.split('/').pop()?.split('.').shift() || '');
	let resource = $state<ResourceContents | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let mounted = $state(false);

	let widthDvw = $state(50);
	let isResizing = $state(false);

	const MIN_WIDTH_PX = 500;
	const MAX_DVW = 75;
	const MIN_DVW = 10;

	function getViewportWidth(): number {
		return typeof window !== 'undefined' && window.visualViewport
			? window.visualViewport.width
			: typeof document !== 'undefined'
				? document.documentElement.clientWidth
				: 1024;
	}

	function getMinDvw(): number {
		const vw = getViewportWidth();
		const minDvwFromPx = (MIN_WIDTH_PX / vw) * 100;
		return Math.max(MIN_DVW, minDvwFromPx);
	}

	function handleResizeStart(e: MouseEvent) {
		e.preventDefault();
		isResizing = true;

		const startX = e.clientX;
		const startDvw = widthDvw;

		function onMouseMove(e: MouseEvent) {
			const vw = getViewportWidth();
			const deltaX = startX - e.clientX;
			const deltaDvw = (deltaX / vw) * 100;
			let newDvw = startDvw + deltaDvw;
			newDvw = Math.max(getMinDvw(), Math.min(MAX_DVW, newDvw));
			widthDvw = newDvw;
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
		const step = 2;
		const minDvw = getMinDvw();

		if (e.key === 'ArrowLeft') {
			e.preventDefault();
			widthDvw = Math.min(MAX_DVW, widthDvw + step);
		} else if (e.key === 'ArrowRight') {
			e.preventDefault();
			widthDvw = Math.max(minDvw, widthDvw - step);
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
			try {
				const result = await chat.readResource(filename);
				if (result.contents?.length) {
					resource = result.contents[0];
				}
				loading = false;

				// Subscribe to live updates
				cleanup = chat.watchResource(filename, (updatedResource) => {
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
	class="relative h-dvh shrink-0 overflow-hidden transition-[opacity] duration-300 ease-out {mounted
		? 'opacity-100'
		: 'opacity-0'}"
	style="width: {mounted ? widthDvw : 0}dvw; min-width: {mounted
		? MIN_WIDTH_PX
		: 0}px; max-width: {MAX_DVW}dvw;"
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
		aria-valuenow={widthDvw}
		aria-valuemin={MIN_DVW}
		aria-valuemax={MAX_DVW}
		aria-label="Resize file editor"
		tabindex="0"
	></div>

	<div class="bg-base-200 flex h-full w-full flex-col">
		<div class="border-base-300 flex items-center gap-2 border-b px-4 py-2">
			<div class="flex grow items-center justify-between">
				<span class="truncate text-sm font-medium">{name}</span>
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
