<script lang="ts">
	import { port } from './utils';

	let { class: klass = '', as = 'div', target, children, ...restProps } = $props();
</script>

<svelte:element this={as} {@attach port(target)} class={klass} {...restProps}>
	{@render children?.()}
</svelte:element>
