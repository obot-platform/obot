<script lang="ts">
	import type { EditorItem } from '$lib/services/editor/index.svelte';

	interface Props {
		file: EditorItem;
	}

	async function toDataURL(blob: Blob): Promise<string> {
		return new Promise<string>((resolve, reject) => {
			const reader = new FileReader();
			reader.onload = () => resolve(reader.result as string);
			reader.onerror = reject;
			reader.readAsDataURL(blob);
		});
	}

	let { file }: Props = $props();
	let src: Promise<string> | undefined = $derived.by(() => {
		if (file?.file?.blob) {
			return toDataURL(file.file.blob);
		}
	});
</script>

{#await src then src}
	<div class="p-5">
		<img class="rounded-3xl" {src} alt="AI generated, content unknown" />
	</div>
{/await}
