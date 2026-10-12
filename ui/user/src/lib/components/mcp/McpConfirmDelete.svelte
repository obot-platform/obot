<script lang="ts">
	import { m } from '$lib/i18n';
	import Confirm from '../Confirm.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		names: string[];
		show: boolean;
		onsuccess: () => void;
		oncancel: () => void;
		loading?: boolean;
		entity?: string;
		entityPlural?: string;
		additionalNote?: string;
	}

	let {
		show,
		onsuccess,
		oncancel,
		loading,
		names,
		entity = m.mcps_servers_entity_server(),
		entityPlural,
		additionalNote
	}: Props = $props();
	let plural = $derived(
		entityPlural ? entityPlural : m.mcps_servers_entity_plural_fallback({ entity })
	);
</script>

<Confirm
	{show}
	{onsuccess}
	{oncancel}
	{loading}
	msg={names.length === 1
		? m.mcps_delete_named({ name: names[0] })
		: m.mcps_servers_confirm_delete_msg_other({ plural })}
	classes={{ body: 'p-0', actions: 'p-4 pt-0' }}
>
	{#snippet note()}
		{#if names.length > 1}
			<p class="px-4 text-sm font-light">
				{m.mcps_servers_confirm_delete_following({ plural })}
			</p>
			<ul class="my-2 max-h-[50vh] w-full overflow-y-auto font-semibold">
				{#each names as name, i (i)}
					<li>{name}</li>
				{/each}
			</ul>
		{/if}

		<p class={twMerge('px-4 text-sm font-light', additionalNote && 'mb-4')}>
			{names.length === 1
				? m.mcps_servers_confirm_delete_note_one({ entity })
				: m.mcps_servers_confirm_delete_note_other({ plural })}
		</p>

		{#if additionalNote}
			<p class="text-sm font-light">
				{additionalNote}
			</p>
		{/if}
	{/snippet}
</Confirm>
