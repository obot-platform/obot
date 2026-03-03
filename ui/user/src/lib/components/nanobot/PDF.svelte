<script lang="ts">
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

	function base64ToBlobUrl(base64: string, mime: string): string {
		const binary = atob(base64);
		const bytes = new Uint8Array(binary.length);
		for (let i = 0; i < binary.length; i++) {
			bytes[i] = binary.charCodeAt(i);
		}
		return URL.createObjectURL(new Blob([bytes], { type: mime }));
	}

	$effect(() => {
		if (!base64) return;
		const url = base64ToBlobUrl(base64, 'application/pdf');
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
