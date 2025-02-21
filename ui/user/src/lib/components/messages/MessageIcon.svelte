<script lang="ts">
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import type { Message, Project } from '$lib/services';
	import { Pencil } from 'lucide-svelte/icons';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { AlertCircle } from 'lucide-svelte';

	interface Props {
		msg: Message;
		project?: Project;
	}

	let { msg, project }: Props = $props();
</script>

{#if !msg.icon}
	<!-- Nothing -->
{:else if msg.icon === 'Pencil'}
	<Pencil class="h-8 w-8" />
{:else if msg.icon === 'Assistant'}
	<AssistantIcon {project} />
{:else if msg.icon === 'Profile'}
	<ProfileIcon />
{:else if msg.icon === 'Error'}
	<AlertCircle class="h-8 w-8 text-red-500" />
{:else}
	<img class="h-8 w-8 rounded-md bg-gray-100 p-1 text-blue" src={msg.icon} alt="message icon" />
{/if}
