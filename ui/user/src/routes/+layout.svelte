<script lang="ts">
	import { browser } from '$app/environment';
	import { page } from '$app/state';
	import Notifications from '$lib/components/Notifications.svelte';
	import ReLoginDialog from '$lib/components/ReLoginDialog.svelte';
	import SuccessNotifications from '$lib/components/SuccessNotifications.svelte';
	import { applyDocumentLocale } from '$lib/i18n';
	import {
		darkMode,
		profile,
		appPreferences,
		version,
		mcpServersAndEntries,
		mcpTunnelConnections,
		vmcpInstances,
		userDeviceSettings,
		appNotification
	} from '$lib/stores';
	import '../app.css';
	import type { PageData } from './$types';
	import { apply, isSupported } from '@oddbird/popover-polyfill/fn';
	import 'devicon/devicon.min.css';
	import { onMount, untrack } from 'svelte';

	interface Props {
		children?: import('svelte').Snippet;
		data: PageData;
	}

	// native popover api polyfill
	if (!isSupported()) {
		apply();
	}

	let { children, data }: Props = $props();

	onMount(() => {
		applyDocumentLocale();
		document.documentElement.toggleAttribute('hydrated', true);
	});

	untrack(() => {
		if (data.appPreferences) {
			appPreferences.initialize(data.appPreferences);
		}

		if (data.profile) {
			profile.initialize(data.profile);
		}

		if (data.version) {
			version.initialize(data.version);
		}

		if (data.appNotification) {
			appNotification.initialize(data.appNotification);
		}

		if (browser) {
			userDeviceSettings.initialize();
		}
	});

	$effect(() => {
		if (typeof document === 'undefined') {
			return;
		}

		const html = document.querySelector('html');
		if (darkMode.isDark) {
			html?.classList.add('dark');
			html?.setAttribute('data-theme', 'nanobotdark');
		} else {
			html?.classList.remove('dark');
			html?.setAttribute('data-theme', 'nanobotlight');
		}

		// Hide the initial loader
		const loader = document.getElementById('initial-loader');
		loader?.classList.add('loaded');
	});

	$effect(() => {
		const pathname = page.url.pathname;
		const view = page.url.searchParams.get('view');
		const isPageRequiresMcpServers =
			pathname.startsWith('/mcp-servers') ||
			pathname.startsWith('/vmcps') ||
			pathname.startsWith('/market');
		const forceRefresh =
			(pathname === '/mcp-servers' || pathname === '/vmcps') &&
			(!view || ['connectors', 'servers', 'vmcps'].includes(view));
		const scope = profile.current.hasAdminAccess?.() ? 'admin' : 'user';
		// A restricted session is walled off from every one of these endpoints, so prefetching the
		// catalog only produces a wall of 403s (and races to create the same identity).
		if (
			profile.current.loaded &&
			!profile.current.requirePasswordChange &&
			isPageRequiresMcpServers
		) {
			untrack(() => mcpServersAndEntries.initialize({ forceRefresh, scope }));
		}
	});

	$effect(() => {
		const pathname = page.url.pathname;
		const view = page.url.searchParams.get('view');
		const isSingleMcpServerView =
			pathname.startsWith('/mcp-servers/c/') || pathname.startsWith('/mcp-servers/s/');
		const isValidMcpServersView =
			pathname === '/mcp-servers' &&
			(!view || ['servers', 'deployments', 'tunnels'].includes(view));
		const isValidVmcpsView =
			pathname === '/vmcps' && (!view || ['connectors', 'deployments', 'vmcps'].includes(view));
		const usesMcpTunnelStatus = isSingleMcpServerView || isValidMcpServersView || isValidVmcpsView;

		if (profile.current.loaded && usesMcpTunnelStatus) {
			return mcpTunnelConnections.startPolling();
		}
	});

	$effect(() => {
		const usesVmcpsInstances =
			page.url.pathname === '/vmcps' ||
			page.url.pathname.startsWith('/vmcps/') ||
			page.url.pathname === '/market';
		if (profile.current.loaded && usesVmcpsInstances) {
			return vmcpInstances.startWatching();
		}
	});
</script>

{@render children?.()}

<svelte:head>
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="" />
	<link
		href="https://fonts.googleapis.com/css2?family=Poppins:ital,wght@0,100;0,200;0,300;0,400;0,500;0,600;0,700;0,800;0,900;1,100;1,200;1,300;1,400;1,500;1,600;1,700;1,800;1,900&display=swap"
		rel="stylesheet"
	/>
	{#if darkMode.isDark}
		<link
			rel="stylesheet"
			href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.10.0/styles/github-dark.min.css"
		/>
	{:else}
		<link
			rel="stylesheet"
			href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.10.0/styles/github.min.css"
		/>
	{/if}
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="" />
	<link
		href="https://fonts.googleapis.com/css2?family=Manrope:wght@200..800&display=swap"
		rel="stylesheet"
	/>
</svelte:head>

<Notifications />
<SuccessNotifications />
<ReLoginDialog />
