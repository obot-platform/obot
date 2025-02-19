<script lang="ts">
	import { ChatService, type Project, type ProjectCredential } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { Plus, X } from 'lucide-svelte/icons';
	import { tools } from '$lib/stores';
	import { popover } from '$lib/actions';
	import { fade } from 'svelte/transition';
	import CredentialAuth from '$lib/components/edit/CredentialAuth.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let { ref, tooltip, toggle } = popover();
	let credentials = $state<ProjectCredential[]>();
	let credentialsAvailable = $derived.by(() => {
		return credentials?.filter((cred) => {
			return tools.items.find((tool) => {
				return tool.enabled && cred.toolID === tool.id && !cred.exists;
			});
		});
	});
	let credentialsExists = $derived.by(() => {
		return credentials?.filter((cred) => {
			return tools.items.find((tool) => {
				return tool.enabled && cred.toolID === tool.id && cred.exists;
			});
		});
	});
	let authDialog: ReturnType<typeof CredentialAuth>;
	let credToAuth = $state<string>('');

	async function reload() {
		credentials = (await ChatService.listProjectCredentials(project.id)).items;
	}

	async function removeCred(cred: ProjectCredential) {
		await ChatService.deleteProjectCredential(cred.toolID);
		return reload();
	}

	async function addCred(cred: ProjectCredential) {
		credToAuth = cred.toolID;
		toggle();
		authDialog?.show();
	}
</script>

{#snippet credentialList(creds: ProjectCredential[], remove: boolean)}
	<ul class="flex flex-col gap-2">
		{#each creds ?? [] as cred}
			{#key cred.toolID}
				<div
					class="flex min-w-[200px] items-center justify-between gap-2 rounded-3xl bg-surface2 px-5 py-4"
				>
					<div class="flex flex-col gap-1">
						<div class="flex items-center gap-2">
							{#if cred.icon}
								<img
									src={cred.icon}
									class="h-6 rounded-md bg-white p-1"
									alt="credential {cred.toolName} icon"
								/>
							{/if}
							<span class="text-sm font-medium">{cred.toolName}</span>
						</div>
					</div>
					{#if remove}
						<button class="icon-button" onclick={() => removeCred(cred)}>
							<X class="h-5 w-5" />
						</button>
					{:else}
						<button class="icon-button" onclick={() => addCred(cred)}>
							<Plus class="h-5 w-5" />
						</button>
					{/if}
				</div>
			{/key}
		{/each}
	</ul>
{/snippet}

<CollapsePane header="Credentials" onOpen={() => reload()}>
	<div class="flex flex-col gap-2">
		{#if credentialsExists}
			{@render credentialList(credentialsExists, true)}
		{/if}
		{#if credentialsExists && credentialsAvailable}
			{#if credentialsExists.length === 0 && credentialsAvailable.length === 0}
				<span class="text-sm">No tools require credentials.</span>
			{/if}
		{/if}
		{#if credentialsAvailable && credentialsAvailable.length > 0}
			<div class="self-end" in:fade>
				<button use:ref class="button flex items-center gap-1" onclick={() => toggle()}>
					<Plus class="h-4 w-4" />
					<span class="text-sm">Credential</span>
				</button>
			</div>
		{/if}
		<div
			use:tooltip
			class="z-20 hidden max-h-[500px] overflow-y-auto rounded-3xl bg-background p-3"
		>
			{@render credentialList(credentialsAvailable ?? [], false)}
		</div>
		<CredentialAuth bind:this={authDialog} toolID={credToAuth} onClose={() => reload()} />
	</div>
</CollapsePane>
