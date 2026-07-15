<script lang="ts">
	import Logo from '$lib/components/Logo.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import {
		AiClient,
		COMMAND_SUPPORTED_AI_CLIENTS,
		COMMON_AI_CLIENTS,
		MAGIC_LINK_SUPPORTED_AI_CLIENTS
	} from '$lib/services/user/constants';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onSelect: (clients: AiClient[]) => void;
	}

	let { onSelect }: Props = $props();
	let selectedClient = $state<AiClient[]>([]);
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();

	const clients = $derived.by(() => {
		const clientsMap = new Map(COMMON_AI_CLIENTS.map((client) => [client.id, client]));
		return [...MAGIC_LINK_SUPPORTED_AI_CLIENTS, ...COMMAND_SUPPORTED_AI_CLIENTS]
			.sort((a, b) => a.localeCompare(b))
			.map((clientId) => ({
				...(clientsMap.get(clientId) ?? { id: clientId, icon: '', iconDark: '', alt: '' })
			}));
	});

	export function open() {
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}
</script>

<ResponsiveDialog bind:this={dialog} hideClose disableClickOutside class="w-sm">
	<div class="flex flex-col w-full items-center justify-center">
		<Logo class="size-18" />
		<h2 class="text-2xl font-semibold mt-0.5">Get Started!</h2>
	</div>

	<p class="w-fit self-center text-center mt-2 mb-4">What is your preferred AI client?</p>

	<div class="flex flex-col gap-2">
		{#each clients as client (client.id)}
			<label
				class={twMerge(
					'cursor-pointer flex items-center justify-between gap-4',
					selectedClient.includes(client.id)
						? 'btn btn-primary rounded-md!'
						: 'btn rounded-md! px-5!'
				)}
			>
				<div class="flex items-center gap-2">
					<img
						src={client?.iconDark ?? client?.icon}
						alt={client?.alt}
						class="size-4 dark:block hidden"
					/>
					<img src={client?.icon} alt={client?.alt} class="size-4 block dark:hidden" />
					<span>{client?.alt}</span>
				</div>
				<div class="flex items-center gap-2">
					<input
						id={`preferred-client-${client.id}`}
						type="checkbox"
						class="checkbox text-primary-content border-0 bg-transparent"
						checked={selectedClient.includes(client.id)}
						onchange={(e) => {
							e.preventDefault();
							selectedClient = selectedClient.includes(client.id)
								? selectedClient.filter((id) => id !== client.id)
								: [...selectedClient, client.id];
						}}
					/>
				</div>
			</label>
		{/each}
	</div>

	<p class="text-xs font-light text-muted-content my-4">
		You can change your preferred AI client at a later time. Your initial selection will help us
		tailor your onboarding experience!
	</p>

	<button
		class="btn btn-primary flex justify-center text-center"
		onclick={() => {
			onSelect(
				selectedClient.length > 0
					? selectedClient
					: [...MAGIC_LINK_SUPPORTED_AI_CLIENTS, ...COMMAND_SUPPORTED_AI_CLIENTS]
			);
			dialog?.close();
		}}
	>
		Next
	</button>
</ResponsiveDialog>
