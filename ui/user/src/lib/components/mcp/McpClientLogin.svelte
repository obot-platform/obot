<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { UserService } from '$lib/services';
	import { isMcpLoginURL } from '$lib/services/user/mcp';
	import McpLogin from './McpLogin.svelte';
	import { onMount } from 'svelte';

	let { authRequestID, onComplete }: { authRequestID: string; onComplete: () => void } = $props();
	let authURL = $state('');
	let loading = $state(false);
	let error = $state('');
	let controller: AbortController;

	async function check() {
		if (loading) return;
		loading = true;
		error = '';
		try {
			const result = await UserService.checkOAuthLocalLogin(authRequestID, {
				signal: controller.signal
			});
			if (controller.signal.aborted) return;
			authURL = result.authURL;
			if (!authURL) onComplete();
		} catch (err) {
			if (!controller.signal.aborted) error = parseErrorContent(err).message;
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		controller = new AbortController();
		void check();
		return () => controller.abort();
	});
</script>

<div class="colors-background flex min-h-screen items-center justify-center p-4">
	<div class="popover w-full max-w-lg p-6 flex flex-col gap-4">
		<h1 class="text-2xl font-semibold">{m.mcps_login_title()}</h1>
		{#if error}
			<p role="alert" class="notification-error">{error}</p>
			<button type="button" class="btn btn-secondary" disabled={loading} onclick={check}
				>{m.mcps_retry()}</button
			>
		{:else if isMcpLoginURL(authURL)}
			<McpLogin url={authURL} {loading} onComplete={check} />
		{:else if authURL}
			<a class="btn btn-primary" href={authURL} target="_blank" rel="external noopener noreferrer"
				>{m.mcps_oauth_authenticate()}</a
			>
			<button type="button" class="btn btn-secondary" disabled={loading} onclick={check}
				>{m.mcps_login_continue()}</button
			>
		{:else}
			<p role="status">{m.mcps_login_validating()}</p>
		{/if}
	</div>
</div>
