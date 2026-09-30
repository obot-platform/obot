<script lang="ts">
	import { resolve } from '$app/paths';
	import Logo from '$lib/components/Logo.svelte';
</script>

<svelte:head>
	<title>Obot | Okta SCIM Provisioning</title>
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
		<h1 class="text-center text-xl font-semibold">This app is for provisioning only</h1>
		<p class="text-muted-content text-center text-sm font-light">
			Okta uses this app to provision your Obot account through SCIM. It does not sign you in. Sign
			in to Obot through your organization's single sign-on instead.
		</p>
		<a class="btn btn-primary w-full" href={resolve('/')}>Go to Obot sign-in</a>
	</div>
</div>
