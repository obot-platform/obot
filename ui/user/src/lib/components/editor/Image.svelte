<script lang="ts">
	import type { EditorItem } from '$lib/stores/editor.svelte';

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
		if (file?.blob) {
			return toDataURL(file.blob);
		}
	});
</script>

{#await src then src}
	<div class="p-5">
		<img class="rounded-3xl" {src} alt="AI generated, content unknown" />
	</div>
{/await}
