<script lang="ts">
	import { browser } from '$app/environment';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import CopyField from '../CopyField.svelte';
	import { Info, RefreshCw } from '@lucide/svelte';
	import { untrack } from 'svelte';

	let {
		url,
		onComplete,
		loading = false,
		serverName = '',
		purpose = 'connect',
		showTitle = true,
		showStatus = true,
		onExpiredChange
	}: {
		url: string;
		onComplete?: () => void | Promise<void>;
		loading?: boolean;
		serverName?: string;
		purpose?: 'connect' | 'tools';
		showTitle?: boolean;
		showStatus?: boolean;
		onExpiredChange?: (expired: boolean) => void;
	} = $props();
	let root = $state<HTMLDivElement>();
	let checking = $state(false);
	let checkFailed = $state(false);
	let expired = $state(false);
	let remainingSeconds = $state<number>();
	let countdown = $derived(
		remainingSeconds === undefined
			? ''
			: `${Math.floor(remainingSeconds / 60)}:${String(remainingSeconds % 60).padStart(2, '0')}`
	);
	$effect(() => onExpiredChange?.(expired && !loading && !checking));
	const id = $props.id();
	let loginURL = $derived(
		browser && url.startsWith('/') ? new URL(url, window.location.origin).href : url
	);
	let name = $derived.by(() => {
		try {
			return (
				new URL(loginURL).searchParams.get('serverName') || serverName || m.mcps_login_server()
			);
		} catch {
			return serverName || m.mcps_login_server();
		}
	});
	let command = $derived.by(() => {
		// Only the attempt endpoint belongs in the CLI command. Query parameters
		// (including the display name) and fragments are browser-only hints.
		if (!browser) return '';
		const endpoint = new URL(loginURL);
		// Double quotes work in Command Prompt, PowerShell, and POSIX shells.
		// Encode characters that those shells could expand inside double quotes.
		const argument = `${endpoint.origin}${endpoint.pathname}`.replace(
			/[\x21\x24\x60]/g,
			(char) => `%${char.charCodeAt(0).toString(16).toUpperCase()}`
		);
		return `obot mcp login "${argument}"`;
	});

	// Watching the existing attempt is read-only. Re-running the owning flow while
	// it is pending would generate new OAuth states and replace the copied command.
	$effect(() => {
		const attemptURL = loginURL;
		if (!root) return;
		expired = false;
		remainingSeconds = undefined;
		checkFailed = false;
		let deadline: number | undefined;
		const dialog = root.closest('dialog');
		let stopped = false;
		let settled = false;
		let timer: ReturnType<typeof setTimeout>;
		let controller: AbortController | undefined;
		const active = () => !stopped && (!dialog || dialog.open);
		const schedule = () => {
			clearTimeout(timer);
			if (active() && !settled && !expired) timer = setTimeout(checkAttempt, 2000);
		};
		async function checkAttempt() {
			if (!active() || expired || controller || settled) return;
			updateCountdown();
			if (expired) return;
			if (document.visibilityState !== 'visible' || loading || checking) {
				schedule();
				return;
			}
			const request = new AbortController();
			controller = request;
			try {
				const response = await fetch(attemptURL, { signal: request.signal, cache: 'no-store' });
				if (request.signal.aborted || !active() || document.visibilityState !== 'visible') return;
				updateCountdown();
				if (response.status === 410 || (response.status === 400 && !onComplete)) {
					expired = true;
					remainingSeconds = 0;
					checkFailed = false;
				} else if (response.status === 400 && onComplete) {
					// A consumed attempt may have completed or failed. The owner checks
					// authentication. Expiration never silently replaces the command.
					settled = await refresh();
				} else if (!response.ok) {
					checkFailed = true;
				} else {
					const metadata = await response.json();
					if (request.signal.aborted || !active()) return;
					if (typeof metadata.expiresIn === 'number' && Number.isFinite(metadata.expiresIn)) {
						deadline = performance.now() + Math.max(0, metadata.expiresIn) * 1000;
						updateCountdown();
					}
					checkFailed = false;
				}
			} catch {
				if (!request.signal.aborted && active()) checkFailed = true;
			} finally {
				if (controller === request) controller = undefined;
				schedule();
			}
		}
		function updateCountdown() {
			if (deadline === undefined) return;
			remainingSeconds = Math.max(0, Math.ceil((deadline - performance.now()) / 1000));
			// The local deadline only updates the display. The server must confirm
			// expiration: authentication may have completed while this tab was hidden.
		}
		const countdownTimer = setInterval(updateCountdown, 1000);
		const visibilityChanged = () => {
			updateCountdown();
			if (document.visibilityState === 'visible' && !controller && !expired) {
				clearTimeout(timer);
				if (!settled) void checkAttempt();
			}
		};
		const observer = new MutationObserver(() => {
			if (!active()) {
				clearTimeout(timer);
				controller?.abort();
			} else schedule();
		});
		if (dialog) observer.observe(dialog, { attributes: true, attributeFilter: ['open'] });
		document.addEventListener('visibilitychange', visibilityChanged);
		untrack(() => void checkAttempt());
		return () => {
			stopped = true;
			clearInterval(countdownTimer);
			clearTimeout(timer);
			controller?.abort();
			observer.disconnect();
			document.removeEventListener('visibilitychange', visibilityChanged);
		};
	});

	let refreshPromise: Promise<boolean> | undefined;
	function refresh(): Promise<boolean> {
		// Manual regeneration and completion polling must await the same result.
		if (refreshPromise) return refreshPromise;
		if (loading || !onComplete) return Promise.resolve(false);
		const complete = onComplete;
		checking = true;
		checkFailed = false;
		refreshPromise = Promise.resolve()
			.then(() => complete())
			.then(() => true)
			.catch(() => {
				checkFailed = true;
				return false;
			})
			.finally(() => {
				checking = false;
				refreshPromise = undefined;
			});
		return refreshPromise;
	}
</script>

<div bind:this={root} class="flex flex-col gap-3 text-sm w-full min-w-0">
	{#if showTitle || (showStatus && onComplete)}
		<div class="flex flex-wrap items-center justify-between gap-3">
			{#if showTitle}
				<h2 class="text-xl font-semibold break-words">
					{purpose === 'tools'
						? m.mcps_login_tools_title({ name })
						: m.mcps_login_named_title({ name })}
				</h2>
			{/if}
			{#if showStatus && onComplete}
				<span role="status" class="flex items-center gap-2 text-sm">
					{#if !expired || loading || checking}<Loading class="size-4 shrink-0" />{/if}
					{expired && !loading && !checking
						? m.mcps_login_expired_status()
						: m.mcps_oauth_composite_oauth_checking()}
				</span>
			{/if}
		</div>
	{/if}
	{#if purpose === 'tools'}
		<p>{m.mcps_login_tools_reason({ name })}</p>
	{/if}
	<div class="notification-info flex items-start gap-3 rounded-lg p-4">
		<Info class="size-5 shrink-0 mt-0.5" />
		<p>{m.mcps_login_reason({ name })}</p>
	</div>
	<ol class="flex flex-col gap-4 list-none">
		<li class="flex items-start gap-3">
			<span
				aria-hidden="true"
				class="notification-info flex size-7 shrink-0 items-center justify-center rounded-full"
				>1</span
			>
			<p>
				<a
					class="link"
					href="https://docs.obot.ai/installation/cli-setup"
					target="_blank"
					rel="noopener noreferrer">{m.mcps_login_install()}</a
				>
				{m.mcps_login_install_hint()}
			</p>
		</li>
		<li class="flex items-start gap-3">
			<span
				aria-hidden="true"
				class="notification-info flex size-7 shrink-0 items-center justify-center rounded-full"
				>2</span
			>
			<div class="flex min-w-0 flex-1 flex-col gap-3">
				<p>{m.mcps_login_instructions()}</p>
				{#if expired}
					<p role="alert" class="p-3">
						{m.mcps_login_expired()}
					</p>
				{:else}
					<CopyField {id} value={command} label={m.mcps_login_command()} variant="code" />
				{/if}
				<div class="flex items-center justify-between gap-2">
					{#if remainingSeconds !== undefined && !expired}
						<p class="text-base-content text-xs font-medium tabular-nums" role="timer">
							{m.mcps_login_expires_in({ time: countdown })}
						</p>
					{/if}
					{#if onComplete}
						<button
							type="button"
							class={expired ? 'btn btn-primary btn-sm shrink-0' : 'btn btn-ghost btn-sm shrink-0'}
							aria-label={m.mcps_login_refresh()}
							title={m.mcps_login_refresh()}
							disabled={loading || checking}
							onclick={refresh}
						>
							<RefreshCw class="size-4" /><span class="hidden sm:inline"
								>{m.mcps_login_new_command()}</span
							>
						</button>
					{/if}
				</div>
			</div>
		</li>
	</ol>
	{#if checkFailed}
		<p role="alert" class="text-error text-sm">{m.mcps_login_check_failed()}</p>
	{/if}
</div>
