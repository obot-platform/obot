<script lang="ts">
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import { m } from '$lib/i18n';
	import { CheckCheck } from '@lucide/svelte';
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

		<div class="flex flex-col items-center justify-center pt-10 pb-12">
			<span class="oauth-complete-check">
				<CheckCheck class="size-18 text-success" />
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
	.oauth-complete-check :global(path) {
		stroke-dasharray: var(--check-length);
		stroke-dashoffset: calc(var(--check-length) * -1);
		animation: oauth-complete-check-draw 0.42s cubic-bezier(0.65, 0, 0.35, 1) forwards;
	}

	.oauth-complete-check :global(path:nth-child(1)) {
		--check-length: 22.63;
	}

	.oauth-complete-check :global(path:nth-child(2)) {
		--check-length: 12.73;
		animation-delay: 0.24s;
	}

	@keyframes oauth-complete-check-draw {
		to {
			stroke-dashoffset: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.oauth-complete-check :global(path) {
			animation: none;
			stroke-dashoffset: 0;
		}
	}
</style>
