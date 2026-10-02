<script lang="ts">
	import { m } from '$lib/i18n';
	import { profile } from '$lib/stores';
	import { clearProductAnalyticsConsentDeferral } from '$lib/stores/productTelemetryConsent.svelte';

	let dialog: HTMLDialogElement;

	$effect(() => {
		if (profile.current.loaded === true && profile.current.expired === true) {
			clearProductAnalyticsConsentDeferral();
			dialog.showModal();
		}
	});

	function handleLogin() {
		window.location.href = `/?rd=${encodeURIComponent(window.location.pathname)}`;
	}
</script>

<dialog bind:this={dialog} class="dialog">
	<div class="dialog-container p-4">
		<div class="flex flex-col items-center gap-4">
			<h2 class="text-xl font-semibold">{m.session_expired_title()}</h2>
			<p class="text-center">{m.session_expired_message()}</p>
			<button onclick={handleLogin} class="btn btn-primary w-full">
				{m.session_log_in()}
			</button>
		</div>
	</div>
</dialog>
