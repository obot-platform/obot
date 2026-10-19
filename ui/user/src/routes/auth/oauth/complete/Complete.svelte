<script lang="ts">
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import { m } from '$lib/i18n';
	import { CircleCheckBig } from '@lucide/svelte';
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
	class="flex flex-col gap-3 min-h-screen items-center justify-center p-6 dark:from-base-300 bg-radial-[at_50%_50%] from-gray-50 dark:to-black"
>
	<section class="paper pt-4 md:w-md w-sm items-center justify-center border-base-400 gap-0">
		<BetaLogo />

		<div class="flex flex-col items-center justify-center pt-6 pb-6">
			<span class="oauth-complete-circle mb-2">
				<CircleCheckBig class="size-18 text-success" />
			</span>

			<h1 class="text-base-content text-2xl font-semibold">
				{m.auth_oauth_complete_title()}
			</h1>

			<p class="text-muted-content text-base font-light">
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
		</div>
	</section>
</main>

<style>
	/* The arc is authored clockwise from the right side up to the top gap, so a
	   negative offset draws one loop from the top, counterclockwise. */
	.oauth-complete-circle :global(path:nth-child(1)) {
		--len: 54.37;
		stroke-dasharray: var(--len);
		stroke-dashoffset: -54.37;
		animation: oauth-complete-circle-draw 0.7s cubic-bezier(0.65, 0, 0.35, 1) forwards;
	}

	.oauth-complete-circle :global(path:nth-child(2)) {
		--check-length: 18.38;
		--check-gap: 22.38;
		stroke-dasharray: 0 var(--check-gap);
		stroke-dashoffset: 2;
		animation: oauth-complete-check-draw 0.42s cubic-bezier(0.65, 0, 0.35, 1) 0.7s forwards;
	}

	@keyframes oauth-complete-circle-draw {
		to {
			stroke-dashoffset: 0;
		}
	}

	@keyframes oauth-complete-check-draw {
		from {
			stroke-dasharray: 0 var(--check-gap);
			stroke-dashoffset: 2;
		}

		to {
			stroke-dasharray: var(--check-length) var(--check-length);
			stroke-dashoffset: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.oauth-complete-circle :global(path:nth-child(1)),
		.oauth-complete-circle :global(path:nth-child(2)) {
			animation: none;
			stroke-dasharray: none;
			stroke-dashoffset: 0;
		}
	}
</style>
