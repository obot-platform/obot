<script lang="ts">
	import { CircleAlert, LoaderCircle, X } from 'lucide-svelte/icons';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		show: boolean;
		msg?: string;
		onsuccess: () => void;
		oncancel: () => void;
		loading?: boolean;
		note?: Snippet | string;
		msgContent?: Snippet;
		classes?: {
			confirm?: string;
			dialog?: string;
			icon?: string;
			iconContainer?: string;
		};
		title?: string;
		type?: 'delete' | 'info';
		disabled?: boolean;
	}

	let {
		show = false,
		msg = 'OK?',
		onsuccess,
		oncancel,
		loading,
		note = 'This action is permanent and cannot be undone. Are you sure you wish to continue?',
		msgContent,
		classes,
		title = 'Confirm Delete',
		type = 'delete',
		disabled
	}: Props = $props();

	let dialog = $state<HTMLDialogElement>();

	$effect(() => {
		if (show) {
			dialog?.showModal();
			dialog?.focus();
		} else {
			dialog?.close();
		}
	});
</script>

<dialog bind:this={dialog} class="dialog">
	<div class="dialog-container w-[calc(100dvw-2rem)] md:w-md">
		<div class="dialog-title p-4 pb-0">
			{title}
			<button type="button" onclick={oncancel}>
				<X class="size-5" />
			</button>
		</div>
		<div class="flex flex-col items-center justify-center gap-2 p-4 pt-0">
			{#if msgContent}
				{@render msgContent()}
			{:else}
				<div
					class={twMerge(
						'rounded-full p-2',
						type === 'delete' ? 'bg-red-500/10' : 'bg-primary/10',
						classes?.iconContainer
					)}
				>
					<CircleAlert
						class={twMerge(
							'size-8',
							type === 'delete' ? 'text-red-500' : 'text-primary',
							classes?.icon
						)}
					/>
				</div>
				<p class="text-center text-base font-medium">{msg}</p>
			{/if}

			<div class="mb-4 self-center text-center font-light">
				{#if typeof note === 'string'}
					<p>{note}</p>
				{:else if note}
					{@render note()}
				{/if}
			</div>

			<div
				class="flex w-full flex-col items-center justify-center gap-2 md:flex-row md:justify-end"
			>
				<button
					onclick={onsuccess}
					type="button"
					class={twMerge(
						'w-full justify-center p-3',
						type === 'delete' ? 'button-destructive' : 'button-primary',
						classes?.confirm
					)}
					disabled={loading || disabled}
				>
					{#if loading}
						<LoaderCircle class="size-4 animate-spin" />
					{:else}
						Yes, I'm sure
					{/if}
				</button>
				<button onclick={oncancel} type="button" class="button w-full justify-center">Cancel</button
				>
			</div>
		</div>
	</div>
	<form class="dialog-backdrop">
		<button type="button" onclick={oncancel}>close</button>
	</form>
</dialog>
