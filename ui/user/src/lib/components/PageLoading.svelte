<script lang="ts">
	import { AlertCircle, LoaderCircle, X } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { fade } from 'svelte/transition';
	import { onDestroy } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		show: boolean;
		text?: string;
		longLoadMessage?: string;
		longLoadDuration?: number;
		isProgressBar?: boolean;
		progress?: number;
		error?: string;
		errorPreContent?: Snippet;
		errorPostContent?: Snippet;
		errorClasses?: {
			root?: string;
		};
		onClose?: () => void;
	}

	let {
		show,
		text,
		longLoadMessage,
		longLoadDuration = 30000,
		progress,
		isProgressBar,
		error,
		errorClasses,
		errorPreContent,
		errorPostContent,
		onClose
	}: Props = $props();
	let isLongLoad = $state(false);
	let timeout = $state<ReturnType<typeof setTimeout>>();
	let displayedProgress = $state<number>(0);

	$effect(() => {
		if (!progress) {
			displayedProgress = 0;
		} else if (progress < displayedProgress) {
			displayedProgress = progress ?? 0;
		} else {
			const intervalRate = (progress - displayedProgress) / 100;
			const interval = setInterval(() => {
				displayedProgress = Math.min(displayedProgress + 1, progress ?? 0);
			}, intervalRate * 10);

			return () => clearInterval(interval);
		}
	});

	onDestroy(() => {
		if (timeout) {
			clearTimeout(timeout);
		}
	});

	$effect(() => {
		if (show) {
			if (!timeout) {
				timeout = setTimeout(() => {
					isLongLoad = true;
				}, longLoadDuration);
			}
		} else {
			isLongLoad = false;
			if (timeout) {
				clearTimeout(timeout);
				timeout = undefined;
			}
		}
	});
</script>

{#if show}
	<div
		in:fade|global={{ duration: 200 }}
		class="fixed top-0 left-0 z-100 flex h-svh w-svw items-center justify-center bg-black/90"
	>
		{#if error}
			<div
				class={twMerge(
					'dark:bg-surface2 dark:border-surface3 bg-background relative flex w-full flex-col items-center gap-4 rounded-lg p-4 dark:border',
					errorClasses?.root
				)}
			>
				<button class="icon-button absolute top-2 right-2 self-end" onclick={() => onClose?.()}
					><X class="size-5" /></button
				>

				{#if errorPreContent}
					{@render errorPreContent()}
				{:else}
					<h4 class="text-xl font-semibold">An Error Occurred</h4>
				{/if}

				<div class="notification-error flex w-full items-center gap-2">
					<AlertCircle class="size-6 text-red-500" />
					<p class="flex flex-col text-sm font-light">
						<span class="font-semibold">Error Details:</span>
						<span class="break-all">
							{error}
						</span>
					</p>
				</div>

				{#if errorPostContent}
					{@render errorPostContent()}
				{/if}
			</div>
		{:else if isProgressBar}
			<div
				class="flex w-full max-w-(--breakpoint-md) flex-col items-center justify-center gap-4 px-8"
			>
				<div class="text-4xl font-extralight text-white">
					{Math.round(displayedProgress ?? 0)}%
				</div>

				<div class="bg-surface3 h-3 w-full overflow-hidden rounded-full">
					<div
						class={twMerge('bg-primary h-full rounded-full transition-all duration-500 ease-out')}
						style="width: {progress ?? 0}%"
					></div>
				</div>

				<div class="flex w-md flex-col justify-center gap-2 text-center">
					{#if isLongLoad && longLoadMessage}
						<p in:fade class="text-md font-light text-white">
							{longLoadMessage}
						</p>
					{:else if text}
						<p class="text-md font-light text-white">{text}</p>
					{/if}
				</div>
			</div>
		{:else}
			<div
				class="dark:bg-surface2 dark:border-surface3 bg-background flex flex-col items-center rounded-xl px-4 py-2 shadow-sm dark:border"
			>
				<div class="flex items-center gap-2">
					<LoaderCircle class="size-8 animate-spin " />
					<p class="text-xl font-semibold">{text ?? 'Loading...'}</p>
				</div>
				{#if isLongLoad && longLoadMessage}
					<p in:fade class="text-md text-on-surface1 mt-4 font-light">
						{longLoadMessage}
					</p>
				{/if}
			</div>
		{/if}
	</div>
{/if}
