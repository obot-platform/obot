<script lang="ts">
	import RFB from '@novnc/novnc/lib/rfb.js';
	import { Maximize2, Minimize2 } from 'lucide-svelte';
	import { onDestroy } from 'svelte';

	interface Props {
		browserBaseUrl?: string;
		visible?: boolean;
	}

	function withProtocol(url: string, websocket: boolean) {
		if (websocket) {
			if (url.startsWith('https://')) return `wss://${url.slice('https://'.length)}`;
			if (url.startsWith('http://')) return `ws://${url.slice('http://'.length)}`;
			return url;
		}

		if (url.startsWith('wss://')) return `https://${url.slice('wss://'.length)}`;
		if (url.startsWith('ws://')) return `http://${url.slice('ws://'.length)}`;
		return url;
	}

	function browserUrl(browserBaseUrl = '', pathname = '/browser', websocket = true) {
		if (typeof window === 'undefined') {
			return websocket ? 'ws://localhost/browser' : 'http://localhost/browser/resize';
		}

		const trimmedBaseUrl = browserBaseUrl.replace(/\/$/, '');
		const origin = window.location.origin.replace(/\/$/, '');
		const base = trimmedBaseUrl
			? /^(https?|wss?):\/\//.test(trimmedBaseUrl)
				? trimmedBaseUrl
				: `${origin}${trimmedBaseUrl.startsWith('/') ? '' : '/'}${trimmedBaseUrl}`
			: origin;
		return withProtocol(`${base}${pathname}`, websocket);
	}

	let { browserBaseUrl = '', visible = $bindable(true) }: Props = $props();

	let container = $state<HTMLDivElement | undefined>(undefined);
	let rfb = $state<RFB | null>(null);
	let connected = $state(false);
	let connecting = $state(false);
	let error = $state<string | null>(null);
	let isFullscreen = $state(false);
	let activeVNCUrl = $state<string | null>(null);
	let resizeTimer: ReturnType<typeof setTimeout> | null = null;
	let lastRequestedSize = $state<string | null>(null);
	let viewerActive = $state(false);

	function getVNCUrl() {
		return browserUrl(browserBaseUrl);
	}

	function getResizeUrl() {
		return browserUrl(browserBaseUrl, '/browser/resize', false);
	}

	async function connect() {
		const nextVNCUrl = getVNCUrl();
		if (!container || rfb || connecting) return;

		connecting = true;
		error = null;

		try {
			const nextRfb = new RFB(container, nextVNCUrl);
			rfb = nextRfb;
			activeVNCUrl = nextVNCUrl;

			nextRfb.addEventListener('connect', () => {
				if (rfb !== nextRfb) return;
				connected = true;
				connecting = false;
				error = null;
			});

			nextRfb.addEventListener('disconnect', () => {
				if (rfb === nextRfb) {
					rfb = null;
					activeVNCUrl = null;
				}
				connected = false;
				connecting = false;
			});

			nextRfb.addEventListener('credentialsrequired', () => {
				if (rfb !== nextRfb) return;
				connecting = false;
				error = 'Password required (but none configured)';
			});

			nextRfb.addEventListener('securityfailure', (e) => {
				if (rfb !== nextRfb) return;
				connecting = false;
				error = `Security failure: ${e.detail.status}`;
			});

			nextRfb.scaleViewport = true;
			nextRfb.resizeSession = true;
			nextRfb.dragViewport = false;
			nextRfb.clipViewport = false;
		} catch (err) {
			rfb = null;
			activeVNCUrl = null;
			connecting = false;
			console.error('VNC connection error:', err);
			error = err instanceof Error ? err.message : 'Connection failed';
		}
	}

	function disconnect() {
		if (rfb) {
			rfb.disconnect();
		}
		rfb = null;
		activeVNCUrl = null;
		connected = false;
		connecting = false;
	}

	async function syncRemoteClipboard(text: string) {
		if (!text || !rfb) return;
		rfb.focus();
		rfb.clipboardPasteFrom(text);
	}

	function sendRemotePasteShortcut() {
		if (!rfb) return;
		rfb.focus();
		rfb.sendKey(0xffe3, 'ControlLeft', true);
		rfb.sendKey(0x0076, 'KeyV', true);
		rfb.sendKey(0x0076, 'KeyV', false);
		rfb.sendKey(0xffe3, 'ControlLeft', false);
	}

	async function handleLocalPaste() {
		if (!viewerActive || !rfb || typeof navigator === 'undefined' || !navigator.clipboard) {
			return;
		}

		try {
			const text = await navigator.clipboard.readText();
			await syncRemoteClipboard(text);
			sendRemotePasteShortcut();
		} catch (err) {
			console.error('Browser clipboard read error:', err);
		}
	}

	function toggleFullscreen() {
		if (!container) return;

		if (!document.fullscreenElement) {
			container.requestFullscreen();
			isFullscreen = true;
		} else {
			document.exitFullscreen();
			isFullscreen = false;
		}
	}

	function queueResize(width: number, height: number) {
		const targetWidth = Math.max(640, Math.ceil(width * window.devicePixelRatio));
		const targetHeight = Math.max(480, Math.ceil(height * window.devicePixelRatio));
		const sizeKey = `${targetWidth}x${targetHeight}`;
		if (sizeKey === lastRequestedSize) return;

		if (resizeTimer) {
			clearTimeout(resizeTimer);
		}

		resizeTimer = setTimeout(async () => {
			try {
				const response = await fetch(getResizeUrl(), {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify({
						width: targetWidth,
						height: targetHeight
					})
				});

				if (!response.ok) {
					throw new Error(`resize failed: ${response.status}`);
				}

				lastRequestedSize = sizeKey;
			} catch (err) {
				console.error('Browser resize error:', err);
			}
		}, 150);
	}

	onDestroy(() => {
		if (resizeTimer) {
			clearTimeout(resizeTimer);
		}
		disconnect();
	});

	$effect(() => {
		if (!visible || !container || typeof ResizeObserver === 'undefined') {
			return;
		}

		const observer = new ResizeObserver((entries) => {
			const entry = entries[0];
			if (!entry) return;
			queueResize(entry.contentRect.width, entry.contentRect.height);
		});

		observer.observe(container);
		const rect = container.getBoundingClientRect();
		queueResize(rect.width, rect.height);

		return () => {
			observer.disconnect();
		};
	});

	$effect(() => {
		if (!visible || !container) {
			viewerActive = false;
			return;
		}

		const handlePointerDown = (event: PointerEvent) => {
			if (!container) return;
			viewerActive = container.contains(event.target as Node);
		};

		const handlePaste = async (event: ClipboardEvent) => {
			if (!viewerActive || !rfb) return;

			const text = event.clipboardData?.getData('text/plain') ?? '';
			if (!text) return;

			event.preventDefault();
			await syncRemoteClipboard(text);
		};

		const handleKeyDown = async (event: KeyboardEvent) => {
			const modifierPressed = event.metaKey || event.ctrlKey;
			if (!viewerActive || !modifierPressed || event.key.toLowerCase() !== 'v') {
				return;
			}

			event.preventDefault();
			await handleLocalPaste();
		};

		document.addEventListener('pointerdown', handlePointerDown, true);
		window.addEventListener('paste', handlePaste);
		window.addEventListener('keydown', handleKeyDown, true);

		return () => {
			document.removeEventListener('pointerdown', handlePointerDown, true);
			window.removeEventListener('paste', handlePaste);
			window.removeEventListener('keydown', handleKeyDown, true);
		};
	});

	$effect(() => {
		const desiredVisible = visible;
		const desiredUrl = getVNCUrl();
		const hasContainer = !!container;

		if (!desiredVisible || !hasContainer) {
			disconnect();
			return;
		}

		if (rfb && activeVNCUrl !== desiredUrl) {
			disconnect();
			return;
		}

		if (!rfb && !connecting) {
			void connect();
		}
	});
</script>

{#if visible}
	<div class="browser-viewer">
		<div class="viewer-header">
			<div class="viewer-status">
				<span
					class:error-dot={!!error}
					class:connecting-dot={!error && !connected}
					class:connected-dot={connected}
					class="status-dot"
				></span>
				<span class="status-label">
					{#if connected}
						Connected
					{:else if error}
						Connection issue
					{:else if connecting}
						Connecting
					{:else}
						Idle
					{/if}
				</span>
			</div>

			<div class="header-actions">
				<button
					class="btn btn-ghost btn-sm btn-square"
					onclick={toggleFullscreen}
					title="Toggle fullscreen"
				>
					{#if isFullscreen}
						<Minimize2 size={16} />
					{:else}
						<Maximize2 size={16} />
					{/if}
				</button>
			</div>
		</div>

		{#if error}
			<div class="error-message">
				<p>{error}</p>
				<button class="btn btn-primary btn-sm" onclick={connect}>Retry</button>
			</div>
		{/if}

		<div class="viewer-container" bind:this={container}></div>
	</div>
{/if}

<style>
	.browser-viewer {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow: hidden;
		background: hsl(var(--b1));
		border-left: 1px solid hsl(var(--b3) / 0.7);
	}

	.viewer-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.625rem 0.75rem;
		background: hsl(var(--b1) / 0.92);
		border-bottom: 1px solid hsl(var(--b3) / 0.7);
		backdrop-filter: blur(8px);
	}

	.viewer-status {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		min-width: 0;
	}

	.header-actions {
		display: flex;
		gap: 0.125rem;
	}

	.status-dot {
		width: 0.5rem;
		height: 0.5rem;
		border-radius: 9999px;
		flex-shrink: 0;
		background: hsl(var(--bc) / 0.35);
	}

	.connected-dot {
		background: #10b981;
		box-shadow: 0 0 0 3px rgb(16 185 129 / 0.12);
	}

	.connecting-dot {
		background: #f59e0b;
		box-shadow: 0 0 0 3px rgb(245 158 11 / 0.12);
	}

	.error-dot {
		background: #ef4444;
		box-shadow: 0 0 0 3px rgb(239 68 68 / 0.12);
	}

	.status-label {
		font-size: 0.8125rem;
		font-weight: 500;
		color: hsl(var(--bc) / 0.72);
		white-space: nowrap;
	}

	.viewer-container {
		flex: 1;
		position: relative;
		overflow: hidden;
		background: #000;
	}

	.error-message {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 1rem;
		padding: 2rem;
		color: #ef4444;
	}

	.error-message p {
		margin: 0;
	}
</style>
