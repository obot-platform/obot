<script lang="ts">
	import { formatBase64ToBlob } from '$lib/format';
	import { renderAsync } from 'docx-preview';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		base64: string;
		mimeType: string;
		class?: string;
	}

	let { base64, mimeType, class: klass }: Props = $props();

	let containerEl = $state<HTMLDivElement | null>(null);
	let renderError = $state<string | null>(null);

	$effect(() => {
		if (!base64 || !containerEl) return;

		const el = containerEl;
		const blob = formatBase64ToBlob(base64, mimeType);
		let active = true;

		el.innerHTML = '';
		renderError = null;

		(async () => {
			try {
				await renderAsync(blob, el, undefined, {
					className: 'docx',
					inWrapper: true,
					breakPages: true,
					renderHeaders: true,
					renderFooters: true,
					renderFootnotes: true,
					renderEndnotes: true,
					ignoreWidth: true,
					ignoreHeight: true,
					renderAltChunks: false
				});
				if (!active) return;
			} catch (e) {
				if (!active) return;
				el.innerHTML = '';
				renderError = e instanceof Error ? e.message : String(e);
			}
		})();

		return () => {
			active = false;
			el.innerHTML = '';
		};
	});
</script>

<div class={twMerge('flex h-full min-h-0 w-full flex-col', klass)}>
	{#if renderError}
		<div class="alert alert-error shrink-0 rounded-none border-x-0 border-t-0">
			<span>Could not preview document: {renderError}</span>
		</div>
	{/if}
	<div
		bind:this={containerEl}
		class="docx-preview-wrap text-base-content [&_.docx]:bg-base-100 [&_.docx]:text-base-content min-h-0 flex-1 overflow-auto [&_.docx]:shadow-sm"
		role="document"
		aria-label="Word document preview"
	></div>
</div>
