<script lang="ts">
	/**
	 * This is the standard responsive dialog component that shows a header w/ an X for desktop,
	 * then, on mobile, a header and separator with a chevron for the return button. It takes up
	 * the whole screen on mobile and a customizable max width on desktop. (default is 2xl)
	 */
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import { responsive } from '$lib/stores';
	import { ChevronRight, X } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
		classes?: {
			header?: string;
			content?: string;
			title?: string;
			closeBtn?: string;
		};
		onClickOutside?: () => void;
		onClose?: () => void;
		onOpen?: () => void;
		titleContent?: Snippet;
		title?: string;
		children: Snippet;
		animate?: 'slide' | 'fade' | null;
		hideClose?: boolean;
		disableClickOutside?: boolean;
	}

	let {
		onClickOutside,
		onClose,
		onOpen,
		titleContent,
		title,
		children,
		class: klass,
		classes,
		animate,
		hideClose,
		disableClickOutside
	}: Props = $props();
	let dialog = $state<HTMLDialogElement>();

	export function open() {
		onOpen?.();
		dialog?.showModal();
	}

	export function close() {
		// Just close the dialog - onClose will be called via the native onclose event
		dialog?.close();
	}
</script>

<dialog
	bind:this={dialog}
	class="dialog"
	use:dialogAnimation={{ type: animate }}
	onclose={() => {
		// Handle native dialog close (e.g., Escape key)
		onClose?.();
	}}
>
	<div
		class={twMerge(
			'dialog-container w-full max-w-2xl font-normal',
			responsive.isMobile && 'mobile',
			klass,
			'p-0'
		)}
	>
		<div
			class={twMerge(
				'flex h-full w-full flex-col',
				!responsive.isMobile && 'p-4',
				classes?.content ?? 'max-h-dvh min-h-fit'
			)}
		>
			{#if titleContent || title}
				<div class="flex flex-col gap-4">
					<h3 class={twMerge('dialog-title', responsive.isMobile && 'mobile', classes?.header)}>
						<span class={twMerge('flex items-center gap-2', classes?.title ?? '')}>
							{#if titleContent}
								{@render titleContent()}
							{:else if title}
								{title}
							{/if}
						</span>
						{#if !hideClose}
							<button
								class={twMerge(
									'icon-button dialog-close-btn',
									responsive.isMobile && 'mobile',
									classes?.closeBtn
								)}
								onclick={(e) => {
									e.preventDefault();
									close();
								}}
							>
								{#if responsive.isMobile && animate === 'slide'}
									<ChevronRight class="size-6" />
								{:else}
									<X class="size-5" />
								{/if}
							</button>
						{/if}
					</h3>
				</div>
			{/if}
			{@render children()}
		</div>
	</div>
	<form class="dialog-backdrop">
		<button
			type="button"
			onclick={() => {
				if (disableClickOutside) return;
				if (onClickOutside) {
					onClickOutside();
				} else {
					close();
				}
			}}
		>
			close
		</button>
	</form>
</dialog>
