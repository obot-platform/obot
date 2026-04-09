<script lang="ts">
	import Error from '$lib/components/Error.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { EditorService } from '$lib/services';
	import type { ImageResponse } from '$lib/services/editor/index.svelte';
	import { Upload } from 'lucide-svelte/icons';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onUpload: (imageUrl: string) => void;
		label?: string;
		variant?: 'icon' | 'preview';
	}
	let { onUpload, label = 'Upload Icon', variant = 'icon' }: Props = $props();

	let fileList = $state<FileList>();
	let uploadInProgress = $state<Promise<ImageResponse>>();
	let previewUrl = $state('');

	export function clearPreview() {
		previewUrl = '';
	}

	$effect(() => {
		if (!fileList?.length) return;

		onUpload(fileList[0].name);
		uploadInProgress = EditorService.uploadImage(fileList[0]);
		uploadInProgress
			.then((result) => {
				onUpload(result.imageUrl);
				if (variant === 'preview') {
					previewUrl = result.imageUrl;
				}
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

<label
	class={twMerge(
		'flex cursor-pointer items-center justify-center gap-2 px-4 py-2',
		variant === 'icon' ? 'icon-button' : 'border-surface3 min-h-72 border-2 border-dashed p-4'
	)}
>
	{#await uploadInProgress}
		<Loading class="h-5 w-5" />
	{:catch error}
		<Error {error} />
	{/await}

	{#if !uploadInProgress}
		{#if variant !== 'preview' || (variant === 'preview' && !previewUrl)}
			<Upload class="h-5 w-5" />
			<span class="text-on-surface">{label}</span>
		{:else if variant === 'preview' && previewUrl}
			{#key previewUrl}
				<img src={previewUrl} alt={label} class="max-h-72 object-contain" />
			{/key}
		{/if}
	{/if}
	<input
		type="file"
		accept="image/png,image/jpeg,image/webp,image/svg+xml"
		class="hidden"
		bind:files={fileList}
	/>
</label>
