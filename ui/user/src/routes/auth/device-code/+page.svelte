<script lang="ts">
	import Logo from '$lib/components/Logo.svelte';
	import { getHttpStatusCode } from '$lib/errors';
	import { UserService } from '$lib/services';
	import { CircleAlert, CircleCheck, Clock, LoaderCircle } from '@lucide/svelte';

	type FormState = 'ready' | 'submitting' | 'invalid' | 'rate-limited' | 'error' | 'success';

	const deviceCodeAlphabet = /^[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{8}$/;

	let code = $state('');
	let formState = $state<FormState>('ready');
	let errorMessage = $state('');

	function formatDeviceCode(value: string): string {
		const compact = value.toUpperCase().replace(/[\s-]/g, '').slice(0, 8);
		return compact.length > 4 ? `${compact.slice(0, 4)}-${compact.slice(4)}` : compact;
	}

	function handleInput(event: Event) {
		code = formatDeviceCode((event.currentTarget as HTMLInputElement).value);
		if (formState !== 'submitting' && formState !== 'success') {
			formState = 'ready';
			errorMessage = '';
		}
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (formState === 'submitting' || formState === 'success') return;

		const normalizedCode = code.replaceAll('-', '');
		if (!deviceCodeAlphabet.test(normalizedCode)) {
			formState = 'invalid';
			errorMessage = 'That code is invalid or expired. Check the code and try again.';
			return;
		}

		formState = 'submitting';
		errorMessage = '';
		try {
			const response = await UserService.verifyDeviceCode(code);
			if (!response.authorized) {
				throw new Error('Device code verification did not complete');
			}
			code = '';
			formState = 'success';
		} catch (error) {
			const status = getHttpStatusCode(error);
			if (status === 400) {
				formState = 'invalid';
				errorMessage = 'That code is invalid or expired. Check the code and try again.';
			} else if (status === 429) {
				formState = 'rate-limited';
				errorMessage = 'Too many attempts. Wait a moment, then try again.';
			} else if (status === 401) {
				formState = 'error';
				errorMessage = 'Your session expired. Sign in again, then retry this code.';
			} else {
				formState = 'error';
				errorMessage = 'We could not verify the code. Check your connection and try again.';
			}
		}
	}
</script>

<svelte:head>
	<title>Obot | Connect Device</title>
</svelte:head>

<div
	class="text-base-content dark:from-base-300 to-base-200 flex min-h-dvh w-full items-center justify-center bg-radial-[at_50%_50%] from-gray-50 px-4 py-8 dark:to-black"
>
	<section
		class="dark:border-base-400 dark:bg-base-200 bg-base-100 flex w-full max-w-sm flex-col gap-4 rounded-xl border border-transparent p-6 shadow-sm"
	>
		<Logo class="h-12 self-center" />

		{#if formState === 'success'}
			<div class="flex flex-col items-center gap-3 py-3 text-center" aria-live="polite">
				<div class="bg-success/10 flex size-12 items-center justify-center rounded-full">
					<CircleCheck class="text-success size-7" />
				</div>
				<h1 class="text-xl font-semibold">Connected</h1>
				<p class="text-muted-content text-sm font-light">
					Authentication is complete. You can close this window and return to your terminal.
				</p>
			</div>
		{:else}
			<div class="flex flex-col gap-1 text-center">
				<h1 class="text-xl font-semibold">Connect your device</h1>
				<p class="text-muted-content text-sm font-light">
					Enter the code shown by <span class="font-medium">obot login</span> in your terminal.
				</p>
			</div>

			{#if errorMessage}
				<div
					class:notification-info={formState === 'rate-limited'}
					class:notification-error={formState !== 'rate-limited'}
					class="flex items-center gap-2"
					role="alert"
				>
					{#if formState === 'rate-limited'}
						<Clock class="text-primary size-5 shrink-0" />
					{:else}
						<CircleAlert class="text-error size-5 shrink-0" />
					{/if}
					<p class="text-sm font-light">{errorMessage}</p>
				</div>
			{/if}

			<form class="flex flex-col gap-4" onsubmit={submit}>
				<label class="flex flex-col gap-1 text-sm font-light" for="device-code">
					Device code
					<input
						id="device-code"
						class="text-input-filled text-center font-mono text-lg tracking-[0.18em] uppercase"
						type="text"
						name="device-code"
						value={code}
						oninput={handleInput}
						autocomplete="one-time-code"
						autocapitalize="characters"
						spellcheck="false"
						inputmode="text"
						maxlength="9"
						placeholder="XXXX-XXXX"
						disabled={formState === 'submitting'}
						required
					/>
				</label>

				<button class="btn btn-primary w-full" type="submit" disabled={formState === 'submitting'}>
					{#if formState === 'submitting'}
						<LoaderCircle class="size-4 animate-spin" />
						Verifying...
					{:else}
						Continue
					{/if}
				</button>
			</form>
		{/if}
	</section>
</div>
