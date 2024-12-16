<script lang="ts">
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import { profile, currentAssistant } from '$lib/stores';
	import { ChatService, type CredentialList } from '$lib/services';
	import { Trash } from '$lib/icons';
	import Menu from '$lib/components/navbar/Menu.svelte';

	let credentials: CredentialList | undefined = $state();

	async function deleteCred(name: string) {
		await ChatService.deleteCredential($currentAssistant.id, name);
		await load();
	}

	async function load() {
		credentials = await ChatService.listCredentials($currentAssistant.id);
	}
</script>

<Menu title={$profile.email || 'Anonymous'} onLoad={load}>
	{#snippet icon()}
		<ProfileIcon />
	{/snippet}
	{#snippet body()}
		<div class="py-2">
			{#if credentials && credentials?.items.length > 0}
				<span class="mb-2">Credentials</span>
				{#each credentials.items as cred}
					<div class="flex justify-between">
						<span>{cred.name}</span>
						<button>
							<Trash
								class="h-5 w-5 text-gray"
								onclick={() => {
									deleteCred(cred.name);
								}}
							/>
						</button>
					</div>
				{/each}
			{:else}
				<span>No credentials</span>
			{/if}
		</div>
		<div class="flex flex-col gap-2 py-2">
			{#if $profile.role === 1}
				<a href="/admin/" rel="external" role="menuitem">Settings</a>
			{/if}
			{#if $profile.email}
				<a href="/oauth2/sign_out?rd=/" rel="external" role="menuitem">Sign out</a>
			{/if}
		</div>
	{/snippet}
</Menu>
