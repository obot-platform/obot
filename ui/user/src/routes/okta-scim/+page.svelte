<script lang="ts">
	import { resolve } from '$app/paths';
	import Logo from '$lib/components/Logo.svelte';
	import { m } from '$lib/i18n';
</script>

<svelte:head>
	<title>{m.identity_access_scim_okta_title()}</title>
</svelte:head>

<!-- The login page of the Okta app that provisions users and groups through SCIM. Okta requires one, but the app is
never used to sign in, so anyone who opens it is sent to Obot's own sign-in. -->
<div
	class="text-base-content dark:from-base-300 to-base-200 flex h-dvh w-full flex-col items-center justify-center bg-radial-[at_50%_50%] from-gray-50 dark:to-black md:p-0 p-4"
>
	<div
		class="dark:border-base-400 dark:bg-base-200 bg-base-100 flex w-full md:w-sm flex-col gap-4 rounded-xl border border-transparent p-6 shadow-sm"
	>
		<Logo class="h-12 self-center" />
		<h1 class="text-center text-xl font-semibold">{m.identity_access_scim_okta_heading()}</h1>
		<p class="text-muted-content text-center text-sm font-light">
			{m.identity_access_scim_okta_body()}
		</p>
		<a class="btn btn-primary w-full" href={resolve('/')}>{m.identity_access_scim_okta_signin()}</a>
	</div>
</div>
