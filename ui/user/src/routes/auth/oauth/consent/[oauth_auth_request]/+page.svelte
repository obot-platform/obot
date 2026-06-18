<script lang="ts">
	import { resolve } from '$app/paths';
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import type { OAuthConsent } from '$lib/services';
	import { ExternalLink, ShieldAlertIcon } from 'lucide-svelte';

	type Props = {
		data: {
			consent: OAuthConsent;
		};
	};

	let { data }: Props = $props();

	const consent = $derived(data.consent);
	const scopes = $derived(consent.scope?.split(' ').filter(Boolean) ?? []);
	const showMCPAuthNotice = $derived(consent.mcpAuthRequired || consent.userHasSecondLevelOAuthed);

	type DetailRow =
		| { label: string; type: 'text'; value: string; valueClass?: string }
		| { label: string; type: 'link'; value: string }
		| { label: string; type: 'scopes'; values: string[] };

	const details = $derived.by((): DetailRow[] => {
		const rows: DetailRow[] = [
			{
				label: 'Application',
				type: 'text',
				value: consent.clientName,
				valueClass: 'wrap-break-word font-medium'
			}
		];

		if (consent.clientURI) {
			rows.push({ label: 'Application URL', type: 'link', value: consent.clientURI });
		}

		rows.push({
			label: 'Redirect URL',
			type: 'text',
			value: consent.redirectURI,
			valueClass: 'break-all'
		});

		if (scopes.length) {
			rows.push({ label: 'Scopes', type: 'scopes', values: scopes });
		}

		if (consent.mcpAuthRequired || consent.userHasSecondLevelOAuthed) {
			rows.push({
				label: 'MCP server',
				type: 'text',
				value: consent.mcpServerName ?? '',
				valueClass: 'wrap-break-word'
			});
			rows.push({
				label: 'Third-party OAuth',
				type: 'text',
				value: consent.userHasSecondLevelOAuthed ? 'Already authorized' : 'Authorization required',
				valueClass: 'wrap-break-word'
			});

			if (consent.mcpServerURL) {
				rows.push({
					label: 'MCP URL',
					type: 'text',
					value: consent.mcpServerURL,
					valueClass: 'break-all'
				});
			}

			if (consent.thirdPartyAuthURL) {
				rows.push({
					label: 'OAuth URL',
					type: 'text',
					value: consent.thirdPartyAuthURL,
					valueClass: 'break-all'
				});
			}
		}

		if (consent.policyURI) {
			rows.push({ label: 'Privacy policy', type: 'link', value: consent.policyURI });
		}

		if (consent.tosURI) {
			rows.push({ label: 'Terms', type: 'link', value: consent.tosURI });
		}

		return rows;
	});
</script>

<svelte:head>
	<title>Authorize OAuth Access</title>
</svelte:head>

<div class="bg-base-200 dark:bg-base-100 flex min-h-screen items-center justify-center p-4">
	<main class="paper w-full max-w-lg overflow-hidden p-0">
		<BetaLogo class="self-center mt-6" />
		<h1 class="truncate text-xl font-semibold text-center">Authorize {consent.clientName}</h1>

		<section class="flex flex-col gap-5 p-4 pt-0">
			{#if showMCPAuthNotice}
				<div class="notification-info flex items-center gap-3 p-3">
					<ShieldAlertIcon class="size-5 shrink-0" />
					<p class="min-w-0 text-sm">
						{#if consent.mcpAuthRequired}
							<b class="font-semibold">{consent.mcpServerName || 'This MCP server'}</b> requires its own
							third-party OAuth authorization. You will be redirected to the third-party OAuth provider
							to complete the authorization.
						{:else if consent.userHasSecondLevelOAuthed}
							<b class="font-semibold">{consent.mcpServerName || 'This MCP server'}</b> requires its own
							third-party OAuth authorization, and you have already authorized it.
						{/if}
					</p>
				</div>
			{/if}

			<p class="text-sm">
				{consent.clientName} wants to authenticate with Obot for an MCP server connection. If you approve,
				Obot will redirect you back to the OAuth client that started this request.
			</p>

			<div>
				<details class="collapse collapse-arrow border border-base-300" name="more-details-content">
					<summary class="collapse-title text-muted-content text-xs font-medium"
						>See details</summary
					>

					<div class="collapse-content space-y-3 overflow-y-auto default-scrollbar-thin max-h-64">
						{#each details as detail (detail.label)}
							<div class="grid grid-cols-[9rem_minmax(0,1fr)] gap-3 text-xs max-sm:grid-cols-1">
								<div class="text-muted-content font-medium">{detail.label}</div>

								{#if detail.type === 'text'}
									<div class="min-w-0 {detail.valueClass ?? ''}">{detail.value}</div>
								{:else if detail.type === 'link'}
									<a
										class="link flex min-w-0 items-center gap-1 break-all"
										href={detail.value}
										rel="external noreferrer noopener"
									>
										<span class="truncate break-all">{detail.value}</span>
										<ExternalLink class="size-3 shrink-0" />
									</a>
								{:else}
									<div class="flex min-w-0 flex-wrap gap-2">
										{#each detail.values as scope, i (i)}
											<span class="badge badge-secondary badge-xs">{scope}</span>
										{/each}
									</div>
								{/if}
							</div>
						{/each}
					</div>
				</details>
			</div>
		</section>

		<footer
			class="border-base-300 bg-base-100 dark:bg-base-200 flex justify-end gap-3 border-t p-3 max-sm:flex-col-reverse"
		>
			<form method="POST" action={resolve(consent.cancelURL as `/${string}`)}>
				<button class="btn btn-text w-full" type="submit">Cancel</button>
			</form>
			<form method="POST" action={resolve(consent.continueURL as `/${string}`)}>
				<button class="btn btn-primary w-full" type="submit">Continue</button>
			</form>
		</footer>
	</main>
</div>
