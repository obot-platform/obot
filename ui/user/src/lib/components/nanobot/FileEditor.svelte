<script lang="ts">
	import RawEditor from '$lib/components/editor/RawEditor.svelte';
	import { getLayout } from '$lib/context/nanobotLayout.svelte';
	import { formatBase64ToBlob } from '$lib/format';
	import type { ResourceContents } from '$lib/services/nanobot/types';
	import { isSafeImageMimeType } from '$lib/services/nanobot/utils';
	import { responsive } from '$lib/stores';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { tryDecodeURIComponent } from '$lib/url';
	import FileItem from './FileItem.svelte';
	import MarkdownEditor from './MarkdownEditor.svelte';
	import OfficeDocumentPreview from './OfficeDocumentPreview.svelte';
	import PDF from './PDF.svelte';
	import { Download, X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		filename: string;
		open?: boolean;
		onClose?: () => void;
		quickBarAccessOpen?: boolean;
		threadContentWidth?: number;
	}

	let { filename, open, onClose, quickBarAccessOpen, threadContentWidth = 0 }: Props = $props();

	const name = $derived(tryDecodeURIComponent(filename.split('/').pop() || ''));
	let resource = $state<ResourceContents | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let mounted = $state(false);

	let widthPx = $state(0);
	let isResizing = $state(false);
	let maxThreadContentWidthSeen = $state(0);
	let rootEl = $state<HTMLDivElement | null>(null);
	let recalculateAnimationFrameId = 0;

	let layout = getLayout();

	const MIN_WIDTH_PX = 300;
	const MAX_DVW = 50;

	function getMaxWidthPxByDvw(): number {
		return Math.floor(getViewportWidth() * (MAX_DVW / 100));
	}

	function getAvailableSpacePx(): number {
		return getViewportWidth() - getThreadRefWidth() - getQuickBarWidth() - getSidebarWidth();
	}

	function getViewportWidth(): number {
		return typeof window !== 'undefined' && window.visualViewport
			? window.visualViewport.width
			: typeof document !== 'undefined'
				? document.documentElement.clientWidth
				: 1024;
	}

	function handleResizeStart(e: MouseEvent) {
		e.preventDefault();
		isResizing = true;

		const startX = e.clientX;
		const startPx = widthPx;

		function onMouseMove(e: MouseEvent) {
			const deltaX = startX - e.clientX;
			const maxPx = getMaxWidthPxByDvw();
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
		const maxPx = getMaxWidthPxByDvw();
		if (e.key === 'ArrowLeft') {
			e.preventDefault();
			widthPx = Math.min(maxPx, widthPx + step);
		} else if (e.key === 'ArrowRight') {
			e.preventDefault();
			widthPx = Math.max(MIN_WIDTH_PX, widthPx - step);
		}
	}

	function getThreadRefWidth(): number {
		const raw =
			maxThreadContentWidthSeen > 0
				? maxThreadContentWidthSeen
				: threadContentWidth > 0
					? threadContentWidth
					: 400;
		// Never assume the thread column is wider than the viewport (breaks space math after
		// desktop→mobile resize while maxThreadContentWidthSeen still holds the old peak).
		return Math.min(raw, getViewportWidth());
	}

	function getQuickBarWidth(): number {
		return quickBarAccessOpen ? 384 : 72;
	}

	function getSidebarWidth(): number {
		return layout.sidebarOpen ? 300 : 72;
	}

	function calculateInitialWidthPx(): number {
		const available = getAvailableSpacePx();
		const maxPx = getMaxWidthPxByDvw();
		return Math.max(MIN_WIDTH_PX, Math.min(maxPx, available));
	}

	$effect(() => {
		if (!open || !rootEl?.parentElement) return;
		const parent = rootEl.parentElement;
		const syncWidth = (w: number) => {
			if (w > 0) {
				widthPx = calculateInitialWidthPx();
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
			widthPx = calculateInitialWidthPx();
		});
	});

	$effect(() => {
		void quickBarAccessOpen;
		void layout.sidebarOpen;
		if (recalculateAnimationFrameId) cancelAnimationFrame(recalculateAnimationFrameId);
		recalculateAnimationFrameId = requestAnimationFrame(() => {
			widthPx = calculateInitialWidthPx();
		});
		return () => {
			if (recalculateAnimationFrameId) {
				cancelAnimationFrame(recalculateAnimationFrameId);
			}
		};
	});

	function getResourcePath(filename: string): string {
		if (filename.startsWith('workflow:///')) return filename;

		const homePrefix = '/home/nanobot/';
		const relpath = filename.startsWith(homePrefix) ? filename.slice(homePrefix.length) : filename;

		const workflowsPrefix = 'workflows/';
		const mdSuffix = '.md';
		if (relpath.startsWith(workflowsPrefix) && relpath.endsWith(mdSuffix)) {
			const withoutWorkflows = relpath.slice(workflowsPrefix.length);
			let withoutMd = withoutWorkflows.slice(0, -mdSuffix.length);
			// Handle directory-based workflows: workflows/<name>/SKILL.md → <name>
			withoutMd = withoutMd.replace(/\/SKILL$/, '');
			return `workflow:///${withoutMd}`;
		} else {
			return relpath.startsWith('file:///') ? relpath : `file:///${relpath}`;
		}
	}

	$effect(() => {
		// Reset state when filename changes
		resource = null;
		loading = true;
		error = null;

		let cleanup: (() => void) | undefined;

		const loadResource = async () => {
			if (!$nanobotChat?.api) {
				console.error('No chat API found');
				return;
			}
			try {
				const resourcePath = getResourcePath(filename);
				const result = await $nanobotChat.api.readResource(resourcePath);
				if (result.contents?.length) {
					resource = result.contents[0];
				}
				loading = false;
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
	let extension = $derived(
		filename.includes('.') ? filename.split('.').pop()?.toLowerCase() : undefined
	);
	let isMarkdown = $derived(mimeType.startsWith('text/markdown') || extension === 'md');
	let isPdf = $derived(mimeType === 'application/pdf');
	let isSvg = $derived(mimeType === 'image/svg+xml' || extension === 'svg');
	let isDocx = $derived(mimeType.includes('wordprocessingml') || extension === 'docx');
	const visible = $derived(mounted && open);
	let justOpened = $state(false);

	function resolveDownloadFilename(basename: string): string {
		if (basename.includes('.')) return basename;
		if (extension) {
			return `${basename}.${extension}`;
		}
		return basename;
	}

	let canDownload = $derived(
		!loading &&
			!error &&
			resource !== null &&
			(Boolean(resource.blob) || resource.text !== undefined)
	);

	function downloadResourceContents() {
		if (!resource || !canDownload) return;

		const downloadName = resolveDownloadFilename(name);
		let blob: Blob;

		if (resource.blob) {
			blob = formatBase64ToBlob(resource.blob, mimeType);
		} else {
			blob = new Blob([resource.text ?? ''], { type: mimeType });
		}

		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = downloadName;
		a.rel = 'noopener';
		a.click();
		URL.revokeObjectURL(url);
	}

	function getPanelDimensionsPx(): { width: number; minWidth: number; maxWidth: number } {
		if (!visible) {
			return { width: 0, minWidth: 0, maxWidth: 0 };
		}
		return {
			width: widthPx,
			minWidth: MIN_WIDTH_PX,
			maxWidth: getMaxWidthPxByDvw()
		};
	}

	const panelDimensionsPx = $derived(getPanelDimensionsPx());

	const ariaSliderValue = $derived.by(() => {
		const maxPx = getMaxWidthPxByDvw();
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
		visible ? 'opacity-100' : 'opacity-0',
		responsive.isMobile ? 'fixed top-0 right-0 z-60 h-[calc(100dvh-3.5rem)] w-full' : ''
	)}
	style={!responsive.isMobile
		? `width: ${panelDimensionsPx.width}px; min-width: ${panelDimensionsPx.minWidth}px; max-width: ${panelDimensionsPx.maxWidth}px;`
		: ''}
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
		<div class="border-base-300 flex items-center gap-2 border-b px-3 py-3 md:px-4 md:py-2">
			{#if responsive.isMobile && onClose}
				{@render closeButton()}
			{/if}
			<div class="flex grow items-center justify-center truncate md:justify-start">
				{#if loading}
					<span class="loading loading-spinner loading-xs"></span>
				{:else}
					<div class="flex items-center gap-1 truncate">
						<FileItem uri={filename} compact />
						<span class="truncate text-sm font-medium">{name}</span>
					</div>
				{/if}
			</div>
			<div class="flex shrink-0 items-center gap-1">
				{#if canDownload}
					<button
						type="button"
						class="btn btn-sm btn-square tooltip tooltip-left"
						onclick={downloadResourceContents}
						data-tip="Download file"
						aria-label="Download file"
					>
						<Download class="size-5 md:size-4" />
					</button>
				{/if}

				{#if !responsive.isMobile && onClose}
					{@render closeButton()}
				{/if}
			</div>
		</div>

		<div class={twMerge('flex-1 overflow-auto', isMarkdown ? 'p-4 pt-0' : '')}>
			{#if loading}
				<div class="flex h-full items-center justify-center">
					<span class="loading loading-spinner loading-md"></span>
				</div>
			{:else if error}
				<div class="alert alert-error">
					<span>Failed to load resource: {error}</span>
				</div>
			{:else if isDocx && resource?.blob}
				<OfficeDocumentPreview base64={resource.blob} {mimeType} />
			{:else if resource?.blob}
				<!-- Binary content - show as image if possible -->
				{#if mimeType.startsWith('image/') && isSafeImageMimeType(mimeType)}
					<div class="flex h-full w-full items-center justify-center">
						<img
							src="data:{mimeType};base64,{resource.blob}"
							alt={filename}
							class="h-auto max-w-full"
						/>
					</div>
				{:else if isPdf}
					<PDF class="h-full" base64={resource.blob} classes={{ iframe: 'h-full' }} />
				{:else}
					<div class="text-base-content/40 italic">This file could not be displayed.</div>
				{/if}
			{:else if isSvg && content}
				<!-- SVG as text (no blob) - display as image -->
				<div class="flex h-full w-full items-center justify-center p-4">
					<img
						src="data:image/svg+xml,{encodeURIComponent(content)}"
						alt={name}
						class="h-auto max-w-full"
					/>
				</div>
			{:else if isMarkdown}
				<MarkdownEditor value={content} readonly />
			{:else if content}
				<RawEditor
					disabled
					value={content}
					{filename}
					disablePreview
					class="h-full grow rounded-none border-0 bg-inherit shadow-none"
					classes={{
						input: 'bg-base-200 h-full max-h-full p-4 pb-8 grid'
					}}
				/>
			{:else}
				<div class="text-base-content/60 italic">The contents of this file are empty.</div>
			{/if}
		</div>
	</div>
</div>

{#snippet closeButton()}
	<button
		class="btn md:btn-sm btn-square md:tooltip md:tooltip-left"
		data-tip="Close"
		onclick={onClose}
	>
		<X class="size-5 md:size-4" />
	</button>
{/snippet}
