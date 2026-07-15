<script module lang="ts">
	export type VirtualListViewportProps<T> = {
		class?: string;
		header?: Snippet;
		children: Snippet<
			[
				{
					items: { index: number; data: T }[];
				}
			]
		>;
	};
</script>

<script lang="ts" generics="T">
	import { getVirtualPageContext, type VirtualPageContext } from './context';
	import { type Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	const context: VirtualPageContext<T> | undefined = getVirtualPageContext();

	const top = $derived(context?.top ?? 0);
	const bottom = $derived(context?.bottom ?? 0);
	const rows = $derived(context?.rows ?? []);

	if (!context) {
		throw new Error('VirtualPageTable must be used within a VirtualPageRoot');
	}

	let { class: klass = '', children, header, ...restProps }: VirtualListViewportProps<T> = $props();
</script>

<table class={twMerge('h-min w-full', klass)} {...restProps}>
	{@render header?.()}

	<tbody bind:this={context.elements.content}>
		<!-- Top spacer row -->
		{#if top > 0}
			<tr aria-hidden="true" class="pointer-events-none">
				<td colspan="100" style="height: {top}px; padding: 0; border: none; line-height: 0;"></td>
			</tr>
		{/if}

		{@render children?.({ items: rows })}

		<!-- Bottom spacer row -->
		{#if bottom > 0}
			<tr aria-hidden="true" class="pointer-events-none">
				<td colspan="100" style="height: {bottom}px; padding: 0; border: none; line-height: 0;"
				></td>
			</tr>
		{/if}
	</tbody>
</table>
