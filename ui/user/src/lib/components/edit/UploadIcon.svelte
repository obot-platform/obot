<script lang="ts">
	import { Upload } from 'lucide-svelte/icons';
	import { EditorService } from '$lib/services';
	import type { ImageResponse } from '$lib/services/editor/index.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import Error from '$lib/components/Error.svelte';

	let { onUpload, label = 'Upload Icon' } = $props();

	let fileList = $state<FileList>();
	let uploadInProgress = $state<Promise<ImageResponse>>();

	$effect(() => {
		if (!fileList?.length) return;

		onUpload(fileList[0].name);
		uploadInProgress = EditorService.uploadImage(fileList[0]);
		uploadInProgress
			.then((result) => {
				onUpload(result.imageUrl);
			})
			.catch((error) => {
				console.error('Failed to upload icon:', error);
			})
			.finally(() => {
				uploadInProgress = undefined;
			});
		fileList = undefined;
	});
</script>

<label class="icon-button flex cursor-pointer items-center justify-center gap-2 px-4 py-2">
	{#await uploadInProgress}
		<Loading class="h-5 w-5" />
	{:catch error}
		<Error {error} />
	{/await}
	{#if !uploadInProgress}
		<Upload class="h-5 w-5" />
		<span class="text-on-surface">{label}</span>
	{/if}
	<input
		type="file"
		accept="image/png,image/jpeg,image/webp"
		class="hidden"
		bind:files={fileList}
	/>
</label>
