<script lang="ts">
	import '../app.css';
	import { darkMode } from '$lib/stores';
	import Notifications from '$lib/components/Notifications.svelte';
	import ReLoginDialog from '$lib/components/ReLoginDialog.svelte';
	import SuccessNotifications from '$lib/components/SuccessNotifications.svelte';
	import { KV } from '$lib/kv';

	interface Props {
		children?: import('svelte').Snippet;
	}

	let { children }: Props = $props();

	$effect(() => {
		if (typeof document === 'undefined') {
			return;
		}

		const html = document.querySelector('html');
		if (darkMode.isDark) {
			html?.classList.add('dark');
		} else {
			html?.classList.remove('dark');
		}
		html?.classList.remove('hidden');
	});

	let kv: KV;

	$effect.pre(() => {
		if (kv) {
			kv.share();
		} else {
			kv = new KV().share();
		}
	});
</script>

{@render children?.()}

<svelte:head>
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="" />
	<link
		href="https://fonts.googleapis.com/css2?family=Poppins:ital,wght@0,100;0,200;0,300;0,400;0,500;0,600;0,700;0,800;0,900;1,100;1,200;1,300;1,400;1,500;1,600;1,700;1,800;1,900&display=swap"
		rel="stylesheet"
	/>
	{#if darkMode.isDark}
		<link
			rel="stylesheet"
			href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.10.0/styles/github-dark.min.css"
		/>
	{:else}
		<link
			rel="stylesheet"
			href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.10.0/styles/github.min.css"
		/>
	{/if}
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="" />
	<link
		href="https://fonts.googleapis.com/css2?family=Manrope:wght@200..800&display=swap"
		rel="stylesheet"
	/>
</svelte:head>

<Notifications />
<SuccessNotifications />
<ReLoginDialog />
