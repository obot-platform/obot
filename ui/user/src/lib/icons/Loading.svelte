<script lang="ts">
	import { twMerge } from 'tailwind-merge';
	import { LoaderCircle } from 'lucide-svelte';

	interface Props {
		class?: string;
	}

	let { class: klass = '' }: Props = $props();
</script>

<LoaderCircle class={twMerge('text-blue h-5 w-5 animate-spin', klass)} />
