<script lang="ts">
	import { twMerge } from 'tailwind-merge';
	import { onMount } from 'svelte';

	interface Props {
		class?: string;
		includeFrom?: boolean;
	}

	let { class: klass, includeFrom }: Props = $props();
	let href = $state('/catalog');

	onMount(() => {
		if (includeFrom && window?.location?.pathname) {
			href = `/catalog?from=${encodeURIComponent(window.location.pathname)}`;
		}
	});
</script>

<a {href}>
	<img src="/user/images/obot-icon-blue.svg" class={twMerge('mx-2 h-8', klass)} alt="Obot icon" />
</a>
