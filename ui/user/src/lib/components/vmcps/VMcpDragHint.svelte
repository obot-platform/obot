<script lang="ts">
	import VMcpDragDropStage from '$lib/components/vmcps/VMcpDragDropStage.svelte';
	import { X } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
		titleId?: string;
		descriptionId?: string;
		onDismiss?: () => void;
	}

	let {
		class: klass,
		titleId = 'vmcp-drag-hint-title',
		descriptionId = 'vmcp-drag-hint-description',
		onDismiss
	}: Props = $props();
</script>

<div class={twMerge('w-60', klass)}>
	<div
		class="bg-base-100/90 dark:bg-base-300/90 border-base-300 dark:border-base-400 relative rounded-lg border p-3 shadow-lg backdrop-blur-sm"
	>
		<div
			class="bg-base-100 dark:bg-base-300 border-base-300 dark:border-base-400 absolute top-1/2 -left-1 size-2 -translate-y-1/2 rotate-45 border-b border-l"
			aria-hidden="true"
		></div>

		<div class="flex items-start justify-between gap-2">
			<p
				id={titleId}
				class="text-muted-content font-mono text-[0.625rem] tracking-[0.14em] uppercase"
			>
				Drag &amp; Drop
			</p>
			<button
				type="button"
				class="text-muted-content hover:text-base-content -mt-1 -mr-1 rounded-sm p-1 transition-colors"
				aria-label="Dismiss drag and drop tip"
				onclick={() => onDismiss?.()}
			>
				<X class="size-3" />
			</button>
		</div>

		<VMcpDragDropStage class="mt-2" />

		<p id={descriptionId} class="text-muted-content mt-2 text-xs font-light">
			Drag a server from the panel anywhere onto the canvas to build a vMCP.
		</p>
	</div>
</div>
