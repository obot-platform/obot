<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import type { AuthProvider } from '$lib/services';

	interface Props {
		isLocalSetup: boolean;
		localUserEmail?: string;
		explicitOwners?: string[];
		provider?: AuthProvider;
		tempLoginUrl: string;
		onManageLocalUsers?: () => void;
		showTitle?: boolean;
	}

	const {
		isLocalSetup,
		localUserEmail,
		explicitOwners = [],
		provider,
		tempLoginUrl,
		onManageLocalUsers,
		showTitle = true
	}: Props = $props();
</script>

{#if showTitle}
	<h3 class="mb-4 text-lg font-semibold">Next Step: Owner Setup</h3>
{/if}

<div class="flex flex-col gap-2">
	{#if isLocalSetup}
		<p>
			{#if localUserEmail}
				Finish setting up Obot by signing in as <b>{localUserEmail}</b>.
			{:else}
				Finish setting up Obot by signing in with one of your local accounts.
			{/if}
		</p>
		<p>This account then becomes the <b>owner</b> of this Obot installation.</p>
	{:else if explicitOwners.length > 0}
		<p>You'll need to continue setup with an owner account.</p>
		<p>The following user(s) have been explicitly assigned the Owner role:</p>
		<ul class="list-disc px-8">
			{#each explicitOwners as owner (owner)}
				<li>{owner}</li>
			{/each}
		</ul>
		<p>
			Log in to the system as one of the explicit owners -- you'll be redirected after
			authenticating.
		</p>
		<p>
			Or log into a different account with your configured auth provider. After authentication,
			you'll be asked to confirm the owner before proceeding.
		</p>
	{:else}
		<p>
			You'll need to set up an initial owner for the system. Login with your configured auth
			provider to continue.
		</p>
	{/if}

	<div class="my-4 flex flex-col gap-2">
		{#if tempLoginUrl}
			<a class="btn btn-secondary w-full" href={tempLoginUrl} rel="external">
				{#if provider?.icon}
					<img
						class="bg-base-100 h-6 w-6 rounded-full p-1 dark:bg-gray-600"
						src={provider.icon}
						alt={provider.name}
					/>
				{/if}
				<span class="text-center text-sm font-light">
					{#if isLocalSetup && localUserEmail}
						Sign in as {localUserEmail}
					{:else}
						Continue with {provider?.name}
					{/if}
				</span>
			</a>
		{:else}
			<div class="w-full justify-center items-center flex">
				<Loading class="size-4" />
			</div>
		{/if}
		{#if isLocalSetup && onManageLocalUsers}
			<p class="text-muted-content text-center text-xs font-light">
				Want to change your owner account?
				<button type="button" class="text-link underline" onclick={onManageLocalUsers}>
					Click here
				</button>
			</p>
		{/if}
	</div>
</div>
