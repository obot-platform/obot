<script lang="ts">
	import { ChatService, type Project, type ProjectCredential } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { Plus, X } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
	import { fade } from 'svelte/transition';
	import CredentialAuth from '$lib/components/edit/CredentialAuth.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';

	interface Props {
		project: Project;
		local?: boolean;
	}

	const projectTools = getProjectTools();
	let { project, local }: Props = $props();
	let { ref, tooltip, toggle } = popover();
	let credentials = $state<ProjectCredential[]>();
	let credentialsAvailable = $derived.by(() => {
		return credentials?.filter((cred) => {
			return projectTools.tools.find((tool) => {
				return tool.enabled && cred.toolID === tool.id && !cred.exists;
			});
		});
	});
	let credentialsExists = $derived.by(() => {
		return credentials?.filter((cred) => {
			return projectTools.tools.find((tool) => {
				return tool.enabled && cred.toolID === tool.id && cred.exists;
			});
		});
	});
	let authDialog: ReturnType<typeof CredentialAuth>;
	let credToAuth = $state<string>('');

	export async function reload() {
		if (local) {
			credentials = (await ChatService.listProjectLocalCredentials(project.assistantID, project.id))
				.items;
		} else {
			credentials = (await ChatService.listProjectCredentials(project.assistantID, project.id))
				.items;
		}
	}

	async function removeCred(cred: ProjectCredential) {
		if (local) {
			await ChatService.deleteProjectLocalCredential(project.assistantID, project.id, cred.toolID);
		} else {
			await ChatService.deleteProjectCredential(project.assistantID, project.id, cred.toolID);
		}
		return reload();
	}

	async function addCred(cred: ProjectCredential) {
		credToAuth = cred.toolID;
		toggle();
		authDialog?.show();
	}
</script>

{#snippet credentialList(creds: ProjectCredential[], remove: boolean)}
	<div class="flex grow flex-col gap-2">
		{#each creds ?? [] as cred}
			{#key cred.toolID}
				<div
					class="bg-surface3 flex min-w-[200px] items-center justify-between gap-2 rounded-3xl px-4 py-2"
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
		{#if !creds || creds.length === 0}
			<span class="text-gray place-self-center self-center pt-6 pb-4 text-sm font-light"
				>No credentials found.</span
			>
		{/if}
	</div>
{/snippet}

{#snippet body()}
	<div class="flex grow flex-col gap-2">
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
			use:tooltip={{ disablePortal: true }}
			class="default-dialog scrollbar-thin hidden max-h-[500px] overflow-y-auto p-5"
		>
			{@render credentialList(credentialsAvailable ?? [], false)}
		</div>
		<CredentialAuth
			bind:this={authDialog}
			toolID={credToAuth}
			{project}
			onClose={() => reload()}
			{local}
		/>
	</div>
{/snippet}

{#if local}
	{@render body()}
{:else}
	<CollapsePane header="Credentials" onOpen={() => reload()}>
		{@render body()}
	</CollapsePane>
{/if}
