<script lang="ts">
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';

	interface Props {
		label: string;
		help: string;
	}

	let { label, help }: Props = $props();
</script>

<span class="input-label flex items-center gap-1">
	<span>{label}</span>
	<InfoTooltip
		text={help}
		placement="right"
		class="ml-0.5 size-3.5"
		classes={{ icon: 'size-3.5' }}
	/>
</span>
