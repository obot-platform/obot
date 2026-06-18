<script lang="ts">
	import Logo from '$lib/components/Logo.svelte';
	import { onMount } from 'svelte';

	type Props = {
		redirectURL: string;
	};

	let { redirectURL }: Props = $props();
	let redirecting = $state(false);

	function redirectNow() {
		if (!redirectURL) return;

		redirecting = true;
		window.location.href = redirectURL;
	}

	onMount(() => {
		if (!redirectURL) return;

		const timeout = window.setTimeout(redirectNow, 3000);
		return () => window.clearTimeout(timeout);
	});
</script>

<svelte:head>
	<title>Authentication Complete</title>
</svelte:head>

<main
	id="main-content"
	class="bg-base-200 dark:bg-base-100 flex min-h-screen items-center justify-center p-6"
>
	<section class="text-center">
		<Logo class="mx-auto mb-4 size-56" />
		<h1 class="text-base-content mb-4 text-5xl font-bold">Authentication Complete</h1>

		<p class="text-muted-content text-base">
			{#if redirecting || !redirectURL}
				You can now close this window.
			{:else}
				You will be redirected in 3 seconds,
				<button class="link" type="button" onclick={redirectNow}>click here</button>
				to redirect now.
			{/if}
		</p>
	</section>
</main>
