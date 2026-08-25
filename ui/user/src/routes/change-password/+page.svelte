<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Logo from '$lib/components/Logo.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService } from '$lib/services';
	import { CircleAlert } from '@lucide/svelte';

	let password = $state('');
	let confirmation = $state('');
	let saving = $state(false);
	let error = $state<string>();

	function redirectTarget() {
		const rd = page.url.searchParams.get('rd') ?? '/';
		return rd.startsWith('/') && !rd.startsWith('//') ? rd : '/';
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		error = undefined;
		if (password !== confirmation) {
			error = 'The passwords do not match.';
			return;
		}

		saving = true;
		try {
			await UserService.changeLocalAuthPassword(password);
			// A full navigation refreshes all auth-dependent application state. The target has been
			// constrained to a same-origin absolute path above.
			window.location.assign(redirectTarget());
		} catch (err) {
			error =
				err instanceof Error ? parseErrorContent(err).message : 'Failed to set your password.';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Obot | Set Your Password</title>
</svelte:head>

<div
	class="text-base-content dark:from-base-300 to-base-200 flex h-dvh w-full flex-col items-center justify-center bg-radial-[at_50%_50%] from-gray-50 dark:to-black"
>
	<form
		onsubmit={submit}
		class="dark:border-base-400 dark:bg-base-200 bg-base-100 flex w-sm flex-col gap-4 rounded-xl border border-transparent p-6 shadow-sm"
	>
		<Logo class="h-12 self-center" />
		<h1 class="text-center text-xl font-semibold">Set your password</h1>
		<p class="text-muted-content text-center text-sm font-light">
			Choose a new password before continuing to Obot.
		</p>

		{#if error}
			<div class="notification-error flex items-center gap-2">
				<CircleAlert class="text-error size-5 shrink-0" />
				<p class="text-sm font-light">{error}</p>
			</div>
		{/if}

		<label class="flex flex-col gap-1 text-sm font-light" for="new-password">
			New password
			<SensitiveInput
				name="new-password"
				bind:value={password}
				class="text-input-filled"
				autocomplete="new-password"
				minlength={LOCAL_AUTH_MIN_PASSWORD_LENGTH}
				required
				data1pIgnore={false}
			/>
			<span class="text-muted-content pt-0.5 text-xs">
				At least {LOCAL_AUTH_MIN_PASSWORD_LENGTH} characters.
			</span>
		</label>

		<label class="flex flex-col gap-1 text-sm font-light" for="confirm-password">
			Confirm password
			<SensitiveInput
				name="confirm-password"
				bind:value={confirmation}
				class="text-input-filled"
				autocomplete="new-password"
				minlength={LOCAL_AUTH_MIN_PASSWORD_LENGTH}
				required
				data1pIgnore={false}
			/>
		</label>

		<button class="btn btn-primary w-full" type="submit" disabled={saving}>
			{#if saving}<Loading class="size-4" />{:else}Set password and continue{/if}
		</button>

		<a class="text-link text-center text-xs font-light" href={resolve('/oauth2/sign_out?rd=/')}>
			Finish later
		</a>
	</form>
</div>
