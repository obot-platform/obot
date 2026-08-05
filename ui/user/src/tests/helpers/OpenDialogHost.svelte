<script lang="ts" generics="TProps extends Record<string, unknown>">
	import type { Component } from 'svelte';
	import { onMount } from 'svelte';

	interface Openable {
		open: () => void;
	}

	interface Props {
		dialog: Component<TProps, Openable>;
		dialogProps: TProps;
	}

	let { dialog: Dialog, dialogProps }: Props = $props();
	let instance = $state<Openable>();

	onMount(() => {
		instance?.open();
	});
</script>

<Dialog bind:this={instance} {...dialogProps} />
