<script lang="ts">
	import { m } from '$lib/i18n';

	interface Props {
		name: string;
		disabled?: boolean;
		/** Marks the subject as an agent with a badge. */
		agent?: boolean;
		class?: string;
	}

	let { name, disabled = false, agent = false, class: klass }: Props = $props();
</script>

<!-- The name of a user, group or agent, dimmed and marked when it is a disabled user. -->
<span class={['flex min-w-0 items-center gap-2', disabled && 'opacity-50', klass]}>
	<span class="truncate">{name}</span>
	{#if agent}
		<span class="pill-secondary shrink-0">{m.core_col_agent()}</span>
	{/if}
	{#if disabled}
		<span class="pill-secondary shrink-0">{m.core_status_disabled()}</span>
	{/if}
</span>
