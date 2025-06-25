<script lang="ts">
	/**
	 * This is the standard responsive dialog component that shows a header w/ an X for desktop,
	 * then, on mobile, a header and separator with a chevron for the return button. It takes up
	 * the whole screen on mobile and a customizable max width on desktop. (default is 2xl)
	 */
	import { clickOutside } from '$lib/actions/clickoutside';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import { responsive } from '$lib/stores';
	import { ChevronRight, X } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
		classes?: {
			header?: string;
		};
		onClose?: () => void;
		onOpen?: () => void;
		titleContent?: Snippet;
		title?: string;
		children: Snippet;
		animate?: 'slide' | 'fade';
	}

	let {
		onClose,
		onOpen,
		titleContent,
		title,
		children,
		class: klass,
		classes,
		animate
	}: Props = $props();
	let dialog = $state<HTMLDialogElement>();

	export function open() {
		onOpen?.();
		dialog?.showModal();
	}

	export function close() {
		onClose?.();
		dialog?.close();
	}
</script>

<dialog
	bind:this={dialog}
	class={twMerge('w-full max-w-2xl font-normal', !responsive.isMobile && 'p-4', klass)}
	class:mobile-screen-dialog={responsive.isMobile}
	use:clickOutside={() => close()}
	use:dialogAnimation={{ type: animate }}
>
	<div class="flex h-full w-full flex-col">
		<div class="mb-4 flex flex-col gap-4">
			<h3
				class={twMerge('default-dialog-title', classes?.header)}
				class:default-dialog-mobile-title={responsive.isMobile}
			>
				<span class="flex items-center gap-2">
					{#if titleContent}
						{@render titleContent()}
					{:else if title}
						{title}
					{/if}
				</span>
				<button
					class:mobile-header-button={responsive.isMobile}
					onclick={(e) => {
						e.preventDefault();
						close();
					}}
					class="icon-button"
				>
					{#if responsive.isMobile}
						<ChevronRight class="size-6" />
					{:else}
						<X class="size-5" />
					{/if}
				</button>
			</h3>
		</div>
		{@render children()}
	</div>
</dialog>
