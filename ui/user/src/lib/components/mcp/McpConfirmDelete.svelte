<script lang="ts">
	import Confirm from '../Confirm.svelte';

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
		entity = 'server',
		entityPlural,
		additionalNote
	}: Props = $props();
	let plural = $derived(entityPlural ? entityPlural : entity + '(s)');
</script>

<Confirm
	{show}
	{onsuccess}
	{oncancel}
	{loading}
	msg={names.length === 1 ? `Delete ${names[0]}?` : `Delete selected ${plural}?`}
>
	{#snippet note()}
		{#if names.length > 1}
			<p class="text-sm font-light">
				The following {plural} will be permanently deleted:
			</p>
			<ul class="my-2 font-semibold">
				{#each names as name (name)}
					<li>{name}</li>
				{/each}
			</ul>
		{/if}

		<p class="mb-4 text-sm font-light">
			Are you sure you want to delete {names.length === 1 ? 'this ' + entity : plural}?
			{names.length === 1 ? 'It' : 'They'} will be permanently deleted and cannot be recovered.
		</p>

		{#if additionalNote}
			<p class="text-sm font-light">
				{additionalNote}
			</p>
		{/if}
	{/snippet}
</Confirm>
