<script lang="ts">
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
	}

	let { class: klass }: Props = $props();
</script>

<a href="/">
	<img src="/user/images/obot-icon-blue.svg" class={twMerge('mx-2 h-8', klass)} alt="Obot icon" />
</a>
