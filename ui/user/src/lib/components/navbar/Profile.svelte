<script lang="ts">
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import { profile } from '$lib/stores';
	import { ChatService, type CredentialList } from '$lib/services';
	import { Trash } from 'lucide-svelte/icons';
	import Menu from '$lib/components/navbar/Menu.svelte';

	let credentials: CredentialList | undefined = $state();

	async function deleteCred(name: string) {
		await ChatService.deleteCredential(name);
		await load();
	}

	async function load() {
		credentials = await ChatService.listCredentials();
	}
</script>

<Menu title={profile.current.getDisplayName?.() || 'Anonymous'} onLoad={load}>
	{#snippet icon()}
		<ProfileIcon />
	{/snippet}
	{#snippet body()}
		<div class="py-2">
			{#if credentials && credentials?.items.length > 0}
				<span class="mb-2">Credentials</span>
				{#each credentials.items as cred}
					{#if !cred.toolName.startsWith('tl1')}
						<div class="flex justify-between">
							<div class="flex items-center gap-2">
								<img alt={cred.toolName} src={cred.icon} class="h-5 w-5" />
								<span>{cred.toolName}</span>
							</div>
							<button>
								<Trash
									class="h-5 w-5 text-gray"
									onclick={() => {
										deleteCred(cred.toolName);
									}}
								/>
							</button>
						</div>
					{/if}
				{/each}
			{:else}
				<span>No credentials</span>
			{/if}
		</div>
		<div class="flex flex-col gap-2 py-2">
			{#if profile.current.role === 1}
				<a href="/admin/" rel="external" role="menuitem">Settings</a>
			{/if}
			{#if profile.current.email}
				<a href="/oauth2/sign_out?rd=/" rel="external" role="menuitem">Sign out</a>
			{/if}
		</div>
	{/snippet}
</Menu>
