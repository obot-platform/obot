<script lang="ts">
	import { browser } from '$app/environment';
	import Logo from '$lib/components/Logo.svelte';
	import { LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
	import { CircleAlert } from '@lucide/svelte';

	// The form posts to the auth provider, which sets the session cookie and redirects to `rd`.
	// On failure it redirects back here with an `error` message to show.
	let params = $derived(browser ? new URL(window.location.href).searchParams : undefined);
	let rd = $derived(params?.get('rd') ?? '/');
	let error = $derived(params?.get('error'));
</script>

<svelte:head>
	<title>Obot | Sign In</title>
</svelte:head>

<div
	class="text-base-content dark:from-base-300 to-base-200 flex h-dvh w-full flex-col items-center justify-center bg-radial-[at_50%_50%] from-gray-50 dark:to-black"
>
	<form
		method="POST"
		action="/oauth2/start"
		class="dark:border-base-400 dark:bg-base-200 bg-base-100 flex w-sm flex-col gap-4 rounded-xl border border-transparent p-6 shadow-sm"
	>
		<Logo class="h-12 self-center" />
		<h1 class="text-center text-xl font-semibold">Sign in to Obot</h1>

		{#if error}
			<div class="notification-error flex items-center gap-2">
				<CircleAlert class="text-error size-5 shrink-0" />
				<p class="text-sm font-light">{error}</p>
			</div>
		{/if}

		<input type="hidden" name="rd" value={rd} />

		<label class="flex flex-col gap-1 text-sm font-light" for="local-auth-email">
			Email
			<input
				id="local-auth-email"
				class="text-input-filled"
				type="email"
				name="email"
				autocomplete="username"
				required
			/>
		</label>

		<label class="flex flex-col gap-1 text-sm font-light" for="local-auth-password">
			Password
			<input
				id="local-auth-password"
				class="text-input-filled"
				type="password"
				name="password"
				autocomplete="current-password"
				minlength={LOCAL_AUTH_MIN_PASSWORD_LENGTH}
				required
			/>
		</label>

		<button class="btn btn-primary w-full" type="submit">Sign in</button>

		<p class="text-muted-content text-center text-xs font-light">
			Don't have an account? Ask an administrator to create one for you.
		</p>
	</form>
</div>
