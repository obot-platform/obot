<script lang="ts">
	import { port } from './utils';

	let { class: klass = '', as = 'div', component, target, children, ...restProps } = $props();
</script>

{#if target && component}
	<component {@attach port(target)} class={klass} {...restProps}>
		{@render children?.()}
	</component>
{:else if target}
	<svelte:element this={as} {@attach port(target)} class={klass} {...restProps}>
		{@render children?.()}
	</svelte:element>
{/if}
