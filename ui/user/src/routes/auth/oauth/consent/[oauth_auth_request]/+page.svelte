<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type OAuthConsent } from '$lib/services';
	import { ChevronDown, ExternalLink, Server, ShieldCheck } from 'lucide-svelte';

	type Props = {
		data: {
			consent: OAuthConsent;
		};
	};

	let { data }: Props = $props();

	let submitting = $state(false);
	let showDetails = $state(false);
	let error = $state('');

	const consent = $derived(data.consent);
	const scopes = $derived(consent.scope?.split(' ').filter(Boolean) ?? []);

	function navigateTo(url: string) {
		window.location.href = url;
	}

	async function submit(action: 'approve' | 'deny') {
		if (submitting) return;

		submitting = true;
		error = '';
		try {
			const response = await UserService.submitOAuthConsent(consent.authRequestID, {
				action,
				csrfToken: consent.csrfToken
			});
			navigateTo(response.redirectURL);
		} catch (err) {
			const parsed = parseErrorContent(err);
			error = parsed.message;
		}
	}
</script>

<svelte:head>
	<title>Authorize OAuth Access</title>
</svelte:head>

<div class="colors-background flex min-h-screen items-center justify-center p-4">
	<main class="popover w-full max-w-xl overflow-hidden p-0">
		<header class="border-base-300 border-b p-6">
			<div class="mb-4 flex items-center gap-3">
				<div class="bg-base-200 flex size-11 shrink-0 items-center justify-center rounded-md">
					<ShieldCheck class="size-6" />
				</div>
				<div class="min-w-0">
					<h1 class="truncate text-2xl font-semibold">Authorize {consent.clientName}</h1>
					<p class="text-muted-foreground mt-1 text-sm">
						{consent.clientName} wants to authenticate with Obot for an MCP server connection.
					</p>
				</div>
			</div>
		</header>

		<section class="flex flex-col gap-5 p-6">
			{#if consent.mcpAuthRequired}
				<div class="notification-info flex items-start gap-3">
					<Server class="mt-0.5 size-5 shrink-0" />
					<p class="min-w-0 text-sm">
						The MCP server {consent.mcpServerName || 'this MCP server'} requires its own third-party OAuth
						authorization. After you continue, Obot will redirect you to authorize that MCP server, then
						return you to the requesting application.
					</p>
				</div>
			{:else if consent.userHasSecondLevelOAuthed}
				<div class="notification-info flex items-start gap-3">
					<Server class="mt-0.5 size-5 shrink-0" />
					<p class="min-w-0 text-sm">
						The MCP server {consent.mcpServerName || 'this MCP server'} requires its own third-party OAuth
						authorization, and you have already authorized it. After you continue, Obot will return you
						to the requesting application.
					</p>
				</div>
			{/if}

			<p class="text-sm">
				After authorization, Obot will redirect you back to the OAuth client that started this
				request.
			</p>

			<div>
				<button
					class="btn btn-text gap-2 px-0"
					type="button"
					aria-expanded={showDetails}
					onclick={() => (showDetails = !showDetails)}
				>
					<ChevronDown class={`size-4 transition-transform ${showDetails ? 'rotate-180' : ''}`} />
					{showDetails ? 'Hide details' : 'See details'}
				</button>

				{#if showDetails}
					<div class="border-base-300 bg-base-100 mt-2 overflow-hidden rounded-lg border">
						<div
							class="grid grid-cols-[9rem_minmax(0,1fr)] gap-3 border-b p-4 text-sm max-sm:grid-cols-1"
						>
							<div class="text-muted-foreground">Application</div>
							<div class="min-w-0 break-words font-medium">{consent.clientName}</div>
						</div>

						{#if consent.clientURI}
							<div
								class="grid grid-cols-[9rem_minmax(0,1fr)] gap-3 border-b p-4 text-sm max-sm:grid-cols-1"
							>
								<div class="text-muted-foreground">Application URL</div>
								<a
									class="link flex min-w-0 items-center gap-1 break-all"
									href={consent.clientURI}
									rel="noreferrer noopener"
								>
									<span>{consent.clientURI}</span>
									<ExternalLink class="size-3 shrink-0" />
								</a>
							</div>
						{/if}

						<div
							class="grid grid-cols-[9rem_minmax(0,1fr)] gap-3 border-b p-4 text-sm max-sm:grid-cols-1"
						>
							<div class="text-muted-foreground">Redirect URL</div>
							<div class="min-w-0 break-all">{consent.redirectURI}</div>
						</div>

						{#if scopes.length}
							<div
								class="grid grid-cols-[9rem_minmax(0,1fr)] gap-3 border-b p-4 text-sm max-sm:grid-cols-1"
							>
								<div class="text-muted-foreground">Scopes</div>
								<div class="flex min-w-0 flex-wrap gap-2">
									{#each scopes as scope}
										<span class="bg-base-200 rounded px-2 py-1 text-xs">{scope}</span>
									{/each}
								</div>
							</div>
						{/if}

						{#if consent.mcpAuthRequired || consent.userHasSecondLevelOAuthed}
							<div
								class="grid grid-cols-[9rem_minmax(0,1fr)] gap-3 border-b p-4 text-sm max-sm:grid-cols-1"
							>
								<div class="text-muted-foreground">MCP server</div>
								<div class="min-w-0 break-words">{consent.mcpServerName}</div>
							</div>

							<div
								class="grid grid-cols-[9rem_minmax(0,1fr)] gap-3 border-b p-4 text-sm max-sm:grid-cols-1"
							>
								<div class="text-muted-foreground">Third-party OAuth</div>
								<div class="min-w-0 break-words">
									{consent.userHasSecondLevelOAuthed
										? 'Already authorized'
										: 'Authorization required'}
								</div>
							</div>

							{#if consent.mcpServerURL}
								<div
									class="grid grid-cols-[9rem_minmax(0,1fr)] gap-3 border-b p-4 text-sm max-sm:grid-cols-1"
								>
									<div class="text-muted-foreground">MCP URL</div>
									<div class="min-w-0 break-all">{consent.mcpServerURL}</div>
								</div>
							{/if}

							{#if consent.thirdPartyAuthURL}
								<div
									class="grid grid-cols-[9rem_minmax(0,1fr)] gap-3 border-b p-4 text-sm max-sm:grid-cols-1"
								>
									<div class="text-muted-foreground">OAuth URL</div>
									<div class="min-w-0 break-all">{consent.thirdPartyAuthURL}</div>
								</div>
							{/if}
						{/if}

						{#if consent.policyURI}
							<div
								class="grid grid-cols-[9rem_minmax(0,1fr)] gap-3 border-b p-4 text-sm max-sm:grid-cols-1"
							>
								<div class="text-muted-foreground">Privacy policy</div>
								<a
									class="link flex min-w-0 items-center gap-1 break-all"
									href={consent.policyURI}
									rel="noreferrer noopener"
								>
									<span>{consent.policyURI}</span>
									<ExternalLink class="size-3 shrink-0" />
								</a>
							</div>
						{/if}

						{#if consent.tosURI}
							<div class="grid grid-cols-[9rem_minmax(0,1fr)] gap-3 p-4 text-sm max-sm:grid-cols-1">
								<div class="text-muted-foreground">Terms</div>
								<a
									class="link flex min-w-0 items-center gap-1 break-all"
									href={consent.tosURI}
									rel="noreferrer noopener"
								>
									<span>{consent.tosURI}</span>
									<ExternalLink class="size-3 shrink-0" />
								</a>
							</div>
						{/if}
					</div>
				{/if}
			</div>

			{#if error}
				<div class="notification-error text-sm">{error}</div>
			{/if}
		</section>

		<footer
			class="border-base-300 bg-base-100 flex justify-end gap-3 border-t p-6 max-sm:flex-col-reverse"
		>
			<button class="btn btn-text" disabled={submitting} onclick={() => submit('deny')}
				>Cancel</button
			>
			<button class="btn btn-primary" disabled={submitting} onclick={() => submit('approve')}>
				{#if submitting}
					<Loading class="size-4" />
				{:else}
					Continue
				{/if}
			</button>
		</footer>
	</main>
</div>
