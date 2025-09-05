<script module lang="ts">
	import type { Component, ComponentProps, Snippet } from 'svelte';

	export type RenderProps<E = string, C extends Component = Component> = {
		class?: string;
		as?: E;
		component?: C;
		children: Snippet;
	} & ComponentProps<C>;
</script>

<script lang="ts">
	let { as = 'div', component, children, ...restProps }: RenderProps<string, Component> = $props();

	const Shell = component;
</script>

{#if component}
	<Shell {as} {...restProps}>
		{@render children?.()}
	</Shell>
{:else}
	<svelte:element this={as} {...restProps}>
		{@render children?.()}
	</svelte:element>
{/if}
