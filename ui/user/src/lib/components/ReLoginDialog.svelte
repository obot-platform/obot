<script lang="ts">
	import { profile } from '$lib/stores';

	let dialog: HTMLDialogElement;

	$effect(() => {
		if (profile.current.loaded === true && profile.current.expired === true) {
			dialog.showModal();
		}
	});

	function handleLogin() {
		window.location.href = `/?rd=${encodeURIComponent(window.location.pathname)}`;
	}
</script>

<dialog
	bind:this={dialog}
	class="bg-background dark:bg-surface2 rounded-lg p-6 shadow-lg"
	onclose={() => {
		// Prevent closing by clicking outside
		dialog.showModal();
	}}
>
	<div class="flex flex-col items-center gap-4">
		<h2 class="text-xl font-semibold">Session Expired</h2>
		<p class="text-on-surface1 text-center">
			Your session has expired. Please log in again to continue.
		</p>
		<button onclick={handleLogin} class="button-primary w-full"> Log In </button>
	</div>
</dialog>
