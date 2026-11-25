<script lang="ts">
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import RequiredMark from '$lib/components/RequiredMark.svelte';

	interface Props {
		label: string;
		help: string;
		required?: boolean;
		invalid?: boolean;
	}

	let { label, help, required, invalid }: Props = $props();
</script>

<span class="input-label flex items-center gap-1">
	<span>
		{label}
		{#if required}<RequiredMark {invalid} />{/if}
	</span>
	<InfoTooltip
		text={help}
		placement="right"
		class="ml-0.5 size-3.5"
		classes={{ icon: 'size-3.5' }}
	/>
</span>
