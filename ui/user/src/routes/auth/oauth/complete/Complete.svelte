<script lang="ts">
	import Logo from '$lib/components/Logo.svelte';
	import { m } from '$lib/i18n';
	import { onMount } from 'svelte';

	type Props = {
		redirectURL: string;
	};

	let { redirectURL }: Props = $props();
	const REDIRECT_DELAY_SECONDS = 3;

	let redirecting = $state(false);
	let secondsRemaining = $state(REDIRECT_DELAY_SECONDS);

	function redirectNow() {
		if (!redirectURL) return;

		redirecting = true;
		window.location.href = redirectURL;
	}

	onMount(() => {
		if (!redirectURL) return;

		secondsRemaining = REDIRECT_DELAY_SECONDS;

		const interval = window.setInterval(() => {
			secondsRemaining = Math.max(0, secondsRemaining - 1);
		}, 1000);
		const timeout = window.setTimeout(redirectNow, REDIRECT_DELAY_SECONDS * 1000);

		return () => {
			window.clearInterval(interval);
			window.clearTimeout(timeout);
		};
	});
</script>

<svelte:head>
	<title>{m.auth_oauth_complete_title()}</title>
</svelte:head>

<main
	id="main-content"
	class="bg-base-200 dark:bg-base-100 flex min-h-screen items-center justify-center p-6"
>
	<section class="text-center">
		<Logo class="mx-auto mb-4 size-56" />
		<h1 class="text-base-content mb-4 text-5xl font-bold">
			{m.auth_oauth_complete_title()}
		</h1>

		<p class="text-muted-content text-base">
			{#if !redirectURL}
				{m.auth_oauth_complete_close()}
			{:else if redirecting}
				{m.auth_oauth_complete_redirecting()}
			{:else}
				{secondsRemaining === 1
					? m.auth_oauth_complete_redirect_in_one({ seconds: secondsRemaining })
					: m.auth_oauth_complete_redirect_in_other({ seconds: secondsRemaining })}
				<button class="link" type="button" onclick={redirectNow}
					>{m.auth_oauth_complete_click_here()}</button
				>
				{m.auth_oauth_complete_redirect_now()}
			{/if}
		</p>
	</section>
</main>
