<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { CircleAlert, LoaderCircle, X } from 'lucide-svelte/icons';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		show: boolean;
		msg?: string;
		onsuccess: () => void;
		oncancel: () => void;
		loading?: boolean;
		note?: Snippet;
		title?: Snippet;
		classes?: {
			confirm?: string;
			dialog?: string;
		};
	}

	let {
		show = false,
		msg = 'OK?',
		onsuccess,
		oncancel,
		loading,
		note,
		title,
		classes
	}: Props = $props();

	let dialog: HTMLDialogElement | undefined = $state();

	$effect(() => {
		if (show) {
			dialog?.showModal();
			dialog?.focus();
		} else {
			dialog?.close();
		}
	});
</script>

<dialog
	bind:this={dialog}
	use:clickOutside={() => oncancel()}
	class={twMerge('dark:bg-surface1 bg-background max-h-full w-full max-w-md', classes?.dialog)}
>
	<div class="relative">
		<button type="button" onclick={oncancel} class="icon-button absolute end-2.5 top-3 ms-auto">
			<X class="h-5 w-5" />
			<span class="sr-only">Close modal</span>
		</button>
		<div class="p-4 text-center md:p-8">
			{#if title}
				{@render title()}
			{:else}
				<CircleAlert class="text-on-background mx-auto mb-4 h-12 w-12" />
				<h3 class="text-on-background mb-5 text-lg font-normal break-words">{msg}</h3>
			{/if}
			{#if note}
				{@render note()}
			{/if}
			<div class="flex items-center justify-center gap-2">
				<button
					onclick={onsuccess}
					type="button"
					class={twMerge(
						'inline-flex min-h-10 items-center rounded-3xl bg-red-600 px-5 py-2.5 text-center text-sm font-medium text-white hover:bg-red-800',
						classes?.confirm
					)}
					disabled={loading}
				>
					{#if loading}
						<LoaderCircle class="size-4 animate-spin" />
					{:else}
						Yes, I'm sure
					{/if}
				</button>
				<button onclick={oncancel} type="button" class="button ms-3">No, cancel</button>
			</div>
		</div>
	</div>
</dialog>
