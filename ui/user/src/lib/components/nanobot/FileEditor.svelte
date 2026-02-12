<script lang="ts">
	import type { ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import type { ResourceContents } from '$lib/services/nanobot/types';
	import { X } from 'lucide-svelte';
	import MarkdownEditor from './MarkdownEditor.svelte';
	import { isSafeImageMimeType } from '$lib/services/nanobot/utils';
	import { getLayout } from '$lib/context/nanobotLayout.svelte';

	interface Props {
		filename: string;
		chat: ChatService;
		open?: boolean;
		onClose?: () => void;
		quickBarAccessOpen?: boolean;
		threadContentWidth?: number;
	}

	let {
		filename,
		chat,
		open,
		onClose,
		quickBarAccessOpen,
		threadContentWidth = 0
	}: Props = $props();

	const name = $derived(filename.split('/').pop()?.split('.').shift() || '');
	let resource = $state<ResourceContents | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let mounted = $state(false);

	let widthDvw = $state(50);
	let widthPx = $state(0);
	let isResizing = $state(false);
	let maxThreadContentWidthSeen = $state(0);
	let containerWidth = $state(0);
	let rootEl = $state<HTMLDivElement | null>(null);
	let recalculateAnimationFrameId = 0;

	let layout = getLayout();

	const MIN_WIDTH_PX = 300;
	const MAX_DVW = 50;
	const MAX_DVW_FILL = 90;
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
		const startPx = widthPx;

		function onMouseMove(e: MouseEvent) {
			const deltaX = startX - e.clientX;
			if (containerWidth > 0) {
				const maxPx = Math.floor(containerWidth * (MAX_DVW / 100));
				widthPx = Math.max(MIN_WIDTH_PX, Math.min(maxPx, startPx + deltaX));
			} else {
				const vw = getViewportWidth();
				const deltaDvw = (deltaX / vw) * 100;
				let newDvw = startDvw + deltaDvw;
				newDvw = Math.max(getMinDvw(), Math.min(MAX_DVW, newDvw));
				widthDvw = newDvw;
			}
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
		const step = containerWidth > 0 ? 40 : 2;
		if (containerWidth > 0) {
			const maxPx = Math.floor(containerWidth * (MAX_DVW / 100));
			if (e.key === 'ArrowLeft') {
				e.preventDefault();
				widthPx = Math.min(maxPx, widthPx + step);
			} else if (e.key === 'ArrowRight') {
				e.preventDefault();
				widthPx = Math.max(MIN_WIDTH_PX, widthPx - step);
			}
		} else {
			const minDvw = getMinDvw();
			if (e.key === 'ArrowLeft') {
				e.preventDefault();
				widthDvw = Math.min(MAX_DVW, widthDvw + step);
			} else if (e.key === 'ArrowRight') {
				e.preventDefault();
				widthDvw = Math.max(minDvw, widthDvw - step);
			}
		}
	}

	function getThreadRefWidth(): number {
		return maxThreadContentWidthSeen > 0
			? maxThreadContentWidthSeen
			: threadContentWidth > 0
				? threadContentWidth
				: 400;
	}

	function getSidebarWidth(): number {
		return layout.sidebarOpen ? 300 : 0;
	}

	function getQuickBarWidth(): number {
		return quickBarAccessOpen ? 384 : 64;
	}

	function calculateRemainingPx(refWidth: number): number {
		const sidebarWidth = getSidebarWidth();
		const quickBarAccessWidth = getQuickBarWidth();
		return refWidth - sidebarWidth - getThreadRefWidth() - quickBarAccessWidth;
	}

	function calculateInitialWidthDvw(): number {
		const vw = getViewportWidth();
		const remainingWidth = calculateRemainingPx(vw);
		const computedDvw = (remainingWidth / vw) * 100;
		return Math.max(getMinDvw(), Math.min(MAX_DVW_FILL, computedDvw));
	}

	function calculateInitialWidthPx(): number {
		const remaining = calculateRemainingPx(containerWidth);
		const maxPx = Math.floor(containerWidth * (MAX_DVW_FILL / 100));
		return Math.max(MIN_WIDTH_PX, Math.min(maxPx, remaining));
	}

	$effect(() => {
		if (!open || !rootEl?.parentElement) return;
		const parent = rootEl.parentElement;
		const syncWidth = (w: number) => {
			containerWidth = w;
			if (w > 0) {
				const remaining = w - getSidebarWidth() - getThreadRefWidth() - getQuickBarWidth();
				const maxPx = Math.floor(w * (MAX_DVW_FILL / 100));
				widthPx = Math.max(MIN_WIDTH_PX, Math.min(maxPx, remaining));
			}
		};
		const ro = new ResizeObserver((entries) => {
			const entry = entries[0];
			if (entry) syncWidth(entry.contentRect.width);
		});
		ro.observe(parent);
		syncWidth(parent.clientWidth);
		return () => ro.disconnect();
	});

	$effect(() => {
		const w = threadContentWidth;
		if (w > 0 && w > maxThreadContentWidthSeen) {
			maxThreadContentWidthSeen = w;
		}
	});

	$effect(() => {
		if (!open) {
			maxThreadContentWidthSeen = 0;
			return;
		}
		requestAnimationFrame(() => {
			mounted = true;
			if (containerWidth > 0) {
				widthPx = calculateInitialWidthPx();
			} else {
				widthDvw = calculateInitialWidthDvw();
			}
		});
	});

	$effect(() => {
		void quickBarAccessOpen;
		void layout.sidebarOpen;
		if (recalculateAnimationFrameId) cancelAnimationFrame(recalculateAnimationFrameId);
		recalculateAnimationFrameId = requestAnimationFrame(() => {
			if (containerWidth > 0) {
				widthPx = calculateInitialWidthPx();
			} else {
				widthDvw = calculateInitialWidthDvw();
			}
		});
		return () => {
			if (recalculateAnimationFrameId) {
				cancelAnimationFrame(recalculateAnimationFrameId);
			}
		};
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

	const visible = $derived(mounted && open);
	let justOpened = $state(false);

	// Normalized 0–100 slider value for ARIA (reflects active sizing mode)
	const ariaSliderValue = $derived.by(() => {
		if (containerWidth > 0) {
			const maxPx = Math.floor(containerWidth * (MAX_DVW / 100));
			const range = maxPx - MIN_WIDTH_PX;
			if (range <= 0) return 0;
			const pct = ((widthPx - MIN_WIDTH_PX) / range) * 100;
			return Math.round(Math.max(0, Math.min(100, pct)));
		}
		const minDvw = getMinDvw();
		const range = MAX_DVW - minDvw;
		if (range <= 0) return 100;
		const pct = ((widthDvw - minDvw) / range) * 100;
		return Math.round(Math.max(0, Math.min(100, pct)));
	});

	$effect(() => {
		if (!visible) return;
		justOpened = true;
		const t = setTimeout(() => {
			justOpened = false;
		}, 300);
		return () => clearTimeout(t);
	});
</script>

<div
	bind:this={rootEl}
	class="relative h-dvh shrink-0 overflow-hidden {justOpened
		? 'transition-[opacity,width,min-width]'
		: 'transition-opacity'} duration-300 ease-out {visible ? 'opacity-100' : 'opacity-0'}"
	style="width: {visible ? (containerWidth > 0 ? widthPx : widthDvw) : 0}{visible
		? containerWidth > 0
			? 'px'
			: 'dvw'
		: 'px'}; min-width: {visible ? MIN_WIDTH_PX : 0}px; max-width: {visible && containerWidth > 0
		? Math.floor(containerWidth * (MAX_DVW_FILL / 100))
		: MAX_DVW_FILL}{visible && containerWidth > 0 ? 'px' : 'dvw'};"
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
		aria-valuenow={ariaSliderValue}
		aria-valuemin={0}
		aria-valuemax={100}
		aria-label="Resize file editor"
		tabindex="0"
	></div>

	<div class="bg-base-200 flex h-full w-full flex-col">
		<div class="border-base-300 flex items-center gap-2 border-b px-4 py-2">
			<div class="flex grow items-center justify-between">
				{#if loading}
					<span class="loading loading-spinner loading-xs"></span>
				{:else}
					<span class="truncate text-sm font-medium">{name}</span>
					{#if mimeType}
						<span class="text-base-content/60 text-xs">{mimeType}</span>
					{/if}
				{/if}
			</div>
			{#if onClose}
				<button
					class="btn btn-sm btn-square tooltip tooltip-left"
					data-tip="Close"
					onclick={onClose}
				>
					<X class="size-4" />
				</button>
			{/if}
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
