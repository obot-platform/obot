<script lang="ts">
	import { LoaderCircle } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
	}

	let { class: klass = '' }: Props = $props();
</script>

<LoaderCircle class={twMerge('text-primary size-5 animate-spin', klass)} />
