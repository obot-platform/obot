<script lang="ts">
	import type { ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import type { ResourceContents } from '$lib/services/nanobot/types';
	import { X } from 'lucide-svelte';
	import MarkdownEditor from './MarkdownEditor.svelte';
	import { isSafeImageMimeType } from '$lib/services/nanobot/utils';
	import { getLayout } from '$lib/context/nanobotLayout.svelte';
	import { twMerge } from 'tailwind-merge';

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

	function getViewportWidth(): number {
		return typeof window !== 'undefined' && window.visualViewport
			? window.visualViewport.width
			: typeof document !== 'undefined'
				? document.documentElement.clientWidth
				: 1024;
	}

	function getEffectiveRefWidth(): number {
		return containerWidth > 0 ? containerWidth : getViewportWidth();
	}

	function handleResizeStart(e: MouseEvent) {
		e.preventDefault();
		isResizing = true;

		const startX = e.clientX;
		const startPx = widthPx;

		function onMouseMove(e: MouseEvent) {
			const deltaX = startX - e.clientX;
			const refWidth = getEffectiveRefWidth();
			const maxPx = Math.min(
				Math.floor(refWidth * (MAX_DVW / 100)),
				getMaxFileEditorWidthPx(refWidth)
			);
			widthPx = Math.max(MIN_WIDTH_PX, Math.min(maxPx, startPx + deltaX));
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
		const step = 40;
		const refWidth = getEffectiveRefWidth();
		const maxPx = Math.min(
			Math.floor(refWidth * (MAX_DVW / 100)),
			getMaxFileEditorWidthPx(refWidth)
		);
		if (e.key === 'ArrowLeft') {
			e.preventDefault();
			widthPx = Math.min(maxPx, widthPx + step);
		} else if (e.key === 'ArrowRight') {
			e.preventDefault();
			widthPx = Math.max(MIN_WIDTH_PX, widthPx - step);
		}
	}

	function getThreadRefWidth(): number {
		return maxThreadContentWidthSeen > 0
			? maxThreadContentWidthSeen
			: threadContentWidth > 0
				? threadContentWidth
				: 400;
	}

	function getQuickBarWidth(): number {
		return quickBarAccessOpen ? 384 : 72;
	}

	// refWidth is the width of our container (flex row: thread + file editor + quick bar), already excluding left sidebar
	function getMaxFileEditorWidthPx(refWidth: number): number {
		return refWidth - getThreadRefWidth() - getQuickBarWidth();
	}

	function calculateRemainingPx(refWidth: number): number {
		return refWidth - getThreadRefWidth() - getQuickBarWidth();
	}

	function calculateInitialWidthPx(refWidth: number): number {
		if (refWidth <= 0) return MIN_WIDTH_PX;
		const remaining = calculateRemainingPx(refWidth);
		const maxByFill = Math.floor(refWidth * (MAX_DVW_FILL / 100));
		const maxByThread = getMaxFileEditorWidthPx(refWidth);
		const maxPx = Math.min(maxByFill, maxByThread);
		return Math.max(MIN_WIDTH_PX, Math.min(maxPx, remaining));
	}

	$effect(() => {
		if (!open || !rootEl?.parentElement) return;
		const parent = rootEl.parentElement;
		const syncWidth = (w: number) => {
			containerWidth = w;
			if (w > 0) {
				widthPx = calculateInitialWidthPx(w);
			}
		};
		const ro = new ResizeObserver((entries) => {
			const entry = entries[0];
			if (entry) syncWidth(entry.contentRect.width);
		});
		ro.observe(parent);
		syncWidth(parent.clientWidth);

		// Recalculate on zoom: visual viewport resize doesn't always trigger ResizeObserver on the parent
		const onViewportResize = () => {
			if (rootEl?.parentElement) {
				syncWidth(rootEl.parentElement.clientWidth);
			}
		};
		let cleanupViewport: (() => void) | undefined;
		const vv = typeof window !== 'undefined' ? window.visualViewport : null;
		if (vv) {
			vv.addEventListener('resize', onViewportResize);
			cleanupViewport = () => vv.removeEventListener('resize', onViewportResize);
		}

		return () => {
			ro.disconnect();
			cleanupViewport?.();
		};
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
			widthPx = calculateInitialWidthPx(getEffectiveRefWidth());
		});
	});

	$effect(() => {
		void quickBarAccessOpen;
		void layout.sidebarOpen;
		if (recalculateAnimationFrameId) cancelAnimationFrame(recalculateAnimationFrameId);
		recalculateAnimationFrameId = requestAnimationFrame(() => {
			widthPx = calculateInitialWidthPx(getEffectiveRefWidth());
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
		return () => cleanup?.();
	});

	// Derive the content to display
	let content = $derived(resource?.text ?? '');
	let mimeType = $derived(resource?.mimeType ?? 'text/plain');

	const visible = $derived(mounted && open);
	let justOpened = $state(false);

	function getPanelDimensionsPx(): { width: number; minWidth: number; maxWidth: number } {
		if (!visible) {
			return { width: 0, minWidth: 0, maxWidth: 0 };
		}
		const refWidth = getEffectiveRefWidth();
		const maxByFill = Math.floor(refWidth * (MAX_DVW_FILL / 100));
		const maxByThread = getMaxFileEditorWidthPx(refWidth);
		return {
			width: widthPx,
			minWidth: MIN_WIDTH_PX,
			maxWidth: Math.min(maxByFill, maxByThread)
		};
	}

	const panelDimensionsPx = $derived(getPanelDimensionsPx());

	const ariaSliderValue = $derived.by(() => {
		const refWidth = getEffectiveRefWidth();
		const maxPx = Math.min(
			Math.floor(refWidth * (MAX_DVW / 100)),
			getMaxFileEditorWidthPx(refWidth)
		);
		const range = maxPx - MIN_WIDTH_PX;
		if (range <= 0) return 0;
		const pct = ((widthPx - MIN_WIDTH_PX) / range) * 100;
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
	class={twMerge(
		'relative h-dvh shrink-0 overflow-hidden duration-300 ease-out',
		justOpened ? 'transition-[opacity,width,min-width]' : 'transition-opacity',
		visible ? 'opacity-100' : 'opacity-0'
	)}
	style="width: {panelDimensionsPx.width}px; min-width: {panelDimensionsPx.minWidth}px; max-width: {panelDimensionsPx.maxWidth}px;"
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
