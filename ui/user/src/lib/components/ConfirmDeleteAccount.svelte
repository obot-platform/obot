<script lang="ts">
	import { m } from '$lib/i18n';
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
	title={m.account_delete_title()}
	msg={m.account_delete_msg()}
	{onsuccess}
	{oncancel}
	disabled={username2 === '' || username2 !== username}
>
	{#snippet note()}
		<p class="text-base-content mb-4 text-sm font-normal">
			{m.account_delete_note()}
		</p>

		<p class="text-base-content mb-4 text-sm font-normal">
			{m.account_delete_type_to_confirm_prefix()} <strong>{username}</strong>
			{m.account_delete_type_to_confirm_suffix()}
		</p>

		<input
			type="text"
			bind:value={username2}
			oninput={(e) => (username2 = (e.target as HTMLInputElement).value)}
			class="focus:border-primary focus:ring-primary mt-1 block w-full rounded-3xl border border-gray-300 px-4 py-2 transition focus:ring-2 focus:outline-none"
		/>
	{/snippet}
</Confirm>
