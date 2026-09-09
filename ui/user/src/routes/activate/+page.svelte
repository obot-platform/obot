<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Logo from '$lib/components/Logo.svelte';
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService } from '$lib/services';
	import { profile } from '$lib/stores';
	import { CircleAlert } from '@lucide/svelte';
	import { onMount } from 'svelte';

	let error = $state<string>();
	let requiresActivation = $state(false);

	async function activate() {
		error = undefined;
		requiresActivation = false;
		const fragment = window.location.hash.slice(1);
		// Not URLSearchParams: form decoding turns a literal '+' into a space, which silently
		// corrupts an operator-supplied token generated with a standard Base64 alphabet.
		const token = /(?:^|&)token=([^&]*)/.exec(fragment)?.[1];
		let setupToken: string | undefined;
		try {
			setupToken = token ? decodeURIComponent(token) : undefined;
		} catch {
			// A malformed percent escape is a broken link, not a usable token. Fall through to the
			// same incomplete-link error rather than throwing before the fragment is cleared.
			setupToken = undefined;
		}
		// Remove the token before any navigation, analytics, or copied URL can retain it. Fragments
		// are not sent in HTTP requests in the first place.
		window.history.replaceState(null, '', window.location.pathname + window.location.search);

		if (!fragment) {
			if (profile.current.requirePasswordChange) {
				await goto(resolve('/change-password'), { invalidateAll: true });
				return;
			}
			requiresActivation = true;
			return;
		}
		if (!setupToken) {
			error = 'This setup link is incomplete.';
			return;
		}

		try {
			await UserService.activateInitialLocalAuthOwner(setupToken);
			await goto(resolve(`/change-password?rd=${encodeURIComponent('/admin/auth-providers')}`), {
				invalidateAll: true
			});
		} catch (err) {
			error = err instanceof Error ? parseErrorContent(err).message : 'The setup link is invalid.';
		}
	}

	onMount(() => {
		activate();
		// Changing only the fragment is a same-document navigation, so nothing remounts.
		window.addEventListener('hashchange', activate);
		return () => window.removeEventListener('hashchange', activate);
	});
</script>

<svelte:head>
	<title>Obot | Activate Owner Account</title>
</svelte:head>

<div
	class="text-base-content dark:from-base-300 to-base-200 flex h-dvh w-full flex-col items-center justify-center bg-radial-[at_50%_50%] from-gray-50 dark:to-black"
>
	<div
		class="dark:border-base-400 dark:bg-base-200 bg-base-100 flex w-sm flex-col items-center gap-4 rounded-xl border border-transparent p-6 shadow-sm"
	>
		<Logo class="h-12" />
		<h1 class="text-center text-xl font-semibold">Activate your Obot account</h1>
		{#if error}
			<div class="notification-error flex w-full items-center gap-2" role="alert">
				<CircleAlert class="text-error size-5 shrink-0" />
				<p class="text-sm font-light">{error}</p>
			</div>
			<p class="text-muted-content text-center text-sm font-light">
				Ask the person who provisioned this environment to reissue the owner setup link.
			</p>
		{:else if requiresActivation}
			<p class="text-muted-content text-center text-sm font-light">
				Obot requires activation. Open the setup link from your provisioning email to continue.
			</p>
		{:else}
			<Loading class="size-6" />
			<p class="text-muted-content text-center text-sm font-light">
				Verifying your secure setup link…
			</p>
		{/if}
	</div>
</div>
