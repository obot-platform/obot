<script lang="ts">
	interface Props {
		skillsAccess?: boolean;
	}

	let { skillsAccess = false }: Props = $props();
</script>

<div>
	{#if skillsAccess}
		All Skills
	{:else}
		-
	{/if}
</div>
