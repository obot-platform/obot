<script lang="ts">
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import type { Message } from '$lib/services';
	import { Pencil } from '$lib/icons';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';

	interface Props {
		msg: Message;
	}

	let { msg }: Props = $props();
</script>

{#if !msg.icon}
	<!-- Nothing -->
{:else if msg.icon === 'Pencil'}
	<Pencil class="h-8 w-8" />
{:else if msg.icon === 'Assistant'}
	<AssistantIcon />
{:else if msg.icon === 'Profile'}
	<ProfileIcon />
{:else}
	<img class="h-8 w-8 rounded-md bg-gray-100 p-1 text-blue" src={msg.icon} alt="message icon" />
{/if}
