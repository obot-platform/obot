<script lang="ts">
	import { formatBase64ToBlobUrl } from '$lib/format';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		base64: string;
		class?: string;
		classes?: {
			iframe?: string;
		};
	}

	let { base64, class: klass, classes }: Props = $props();

	let pdfBlobUrl = $state<string | null>(null);

	$effect(() => {
		if (!base64) return;
		const url = formatBase64ToBlobUrl(base64, 'application/pdf');
		pdfBlobUrl = url;
		return () => {
			URL.revokeObjectURL(url);
		};
	});
</script>

{#if pdfBlobUrl}
	<div class={twMerge('w-full', klass)}>
		<iframe src={pdfBlobUrl} class={twMerge('w-full', classes?.iframe)} title="PDF Viewer"></iframe>
	</div>
{/if}
