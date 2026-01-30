<script lang="ts">
	import Confirm from './Confirm.svelte';

	interface Props {
		show: boolean;
		msg?: string;
		username?: string;
		buttonText?: string;
		onsuccess: () => void;
		oncancel: () => void;
	}

	let { show = false, username = '', onsuccess, oncancel }: Props = $props();

	let dialog: HTMLDialogElement | undefined = $state();

	let username2 = $state('');

	$effect(() => {
		if (show) {
			dialog?.showModal();
			dialog?.focus();
			username2 = '';
		} else {
			dialog?.close();
		}
	});
</script>

<Confirm
	{show}
	title="Confirm Account Deletion"
	msg="Delete your account?"
	{onsuccess}
	{oncancel}
	disabled={username2 === '' || username2 !== username}
>
	{#snippet note()}
		<p class="text-on-background mb-4 text-sm font-normal">
			This will sign you out of all other devices and browsers, except for this one.
		</p>

		<p class="text-on-background mb-4 text-sm font-normal">
			To confirm, type <strong>{username}</strong> in the box below
		</p>

		<input
			type="text"
			bind:value={username2}
			oninput={(e) => (username2 = (e.target as HTMLInputElement).value)}
			class="focus:border-primary focus:ring-primary mt-1 block w-full rounded-3xl border border-gray-300 px-4 py-2 transition focus:ring-2 focus:outline-none"
		/>
	{/snippet}
</Confirm>
