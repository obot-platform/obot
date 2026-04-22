<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import { Group } from '$lib/services';
	import { darkMode, profile, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import { isSameDay } from 'date-fns';
	import { driver, type DriveStep, type Driver, type PopoverDOM } from 'driver.js';
	import 'driver.js/dist/driver.css';
	import { noop } from 'es-toolkit';
	import { mount, unmount } from 'svelte';

	const TOUR_COMPLETED_KEY = 'tour-completed';
	let hasCompletedTour = $state(false);
	let hasAdminAccess = $derived(profile.current?.hasAdminAccess?.() ?? false);
	let isAtLeastPowerUser = $derived(profile.current?.groups.includes(Group.POWERUSER));
	let isNewUser = $derived(
		profile.current?.created && isSameDay(new Date(profile.current?.created), new Date())
	);

	const adminConfig = $derived($adminConfigStore);
	const isConfigured = $derived(
		hasAdminAccess
			? adminConfig.modelProviderConfigured &&
					(version.current.authEnabled ? adminConfig.authProviderConfigured : true)
			: true
	);
	const tourEnabled = $derived(page.url.searchParams.get('enableTour') === 'true');

	afterNavigate(() => {
		hasCompletedTour = localStorage.getItem(TOUR_COMPLETED_KEY) === 'true';
	});

	const correctRoute = $derived(
		['/admin/dashboard', '/admin/mcp-servers', '/mcp-servers'].includes(page.url.pathname)
	);

	const POPOVER_CLASS = 'w-2xl! max-w-2xl! min-w-2xl!';
	const IMG_WRAP = 'flex items-center justify-center my-2 w-full overflow-hidden rounded-md';

	function prependTourImage(popover: PopoverDOM, dataMark: string, src: string, alt: string) {
		if (popover.wrapper.querySelector(`[${dataMark}]`)) {
			return;
		}
		const wrapper = document.createElement('div');
		wrapper.className = IMG_WRAP;
		wrapper.setAttribute(dataMark, '');
		const img = document.createElement('img');
		img.src = src;
		img.alt = alt;
		img.className = 'h-[445px] w-full object-contain object-top';
		wrapper.appendChild(img);
		popover.wrapper.prepend(wrapper);
	}

	function tourImageRender(dataMark: string, src: string, alt: string) {
		return (popover: PopoverDOM) => prependTourImage(popover, dataMark, src, alt);
	}

	const tourImg = {
		configure: {
			mark: 'data-tour-mcpservers-configure',
			src: '/admin/assets/tour/mcpservers_1.webp'
		},
		audit: {
			mark: 'data-tour-mcpservers-audit',
			src: '/admin/assets/tour/mcpservers_2.webp'
		},
		registries: {
			mark: 'data-tour-mcpregistries',
			src: '/admin/assets/tour/mcpregistries.webp'
		},
		skills: {
			mark: 'data-tour-skills',
			src: '/admin/assets/tour/skills.webp'
		},
		agent: {
			mark: 'data-tour-agent',
			src: '/admin/assets/tour/agent.webp'
		},
		connect: {
			mark: 'data-tour-mcpservers-connect',
			src: '/admin/assets/tour/connect_to_server.webp'
		}
	} as const;

	$effect(() => {
		let logoMount: Record<string, unknown> | undefined;
		if (!isConfigured || !correctRoute || hasCompletedTour || !tourEnabled) return;

		const introStep = {
			popover: {
				showButtons: ['next', 'close'],
				title: '',
				description: isNewUser
					? "Looks like it's your first time here! Let's walk you through what Obot has to offer."
					: "Welcome back! Let's walk you through what Obot has to offer.",
				side: 'top',
				align: 'center',
				popoverClass: 'text-sm w-xs! max-w-xs! min-w-xs!',
				onPopoverRender: (popover: PopoverDOM) => {
					if (popover.wrapper.querySelector('[data-tour-logo]')) {
						return;
					}
					const container = document.createElement('div');
					container.className = 'flex items-center justify-center pb-2 mb-2';
					container.setAttribute('data-tour-logo', '');
					popover.wrapper.prepend(container);
					logoMount = mount(BetaLogo, {
						target: container,
						props: { class: 'justify-center' }
					});
				}
			}
		};

		const stepMcpRegistries = {
			element: '#mcp-registries',
			popover: {
				popoverClass: POPOVER_CLASS,
				title: 'Control MCP Server Accessibility',
				description:
					'Set up access control rules to determine who can access each MCP server. You can allow access to specific users or groups, or even everyone.',
				side: 'bottom' as const,
				align: 'end' as const,
				onPopoverRender: tourImageRender(
					tourImg.registries.mark,
					tourImg.registries.src,
					'Control MCP Server Accessibility'
				)
			}
		};

		const stepAgent = {
			element: '#launch-agent-chat',
			popover: {
				popoverClass: `${POPOVER_CLASS} tour-skills-arrow-center`,
				title: 'Use Built-In Agents to Accomplish Tasks',
				description:
					'The agents will be set up to access the MCP servers & skills based on your access policies. Automate processes, build workflows, and more.',
				side: 'right' as const,
				align: 'center' as const,
				onPopoverRender: tourImageRender(
					tourImg.agent.mark,
					tourImg.agent.src,
					'Use Built-in Agents to Accomplish Tasks'
				)
			}
		};

		const adminSteps = [
			{
				element: '#mcp-servers',
				popover: {
					popoverClass: POPOVER_CLASS,
					title: 'Deploy & Connect Your MCP Servers',
					description:
						'Set up your MCP servers to start using them through the gateway. You can set up against an existing remote server, have a instance created per user, or start a shared server that is accessible to everyone who has access.',
					side: 'bottom' as const,
					align: 'end' as const,
					onPopoverRender: tourImageRender(
						tourImg.configure.mark,
						tourImg.configure.src,
						'MCP servers configuration'
					)
				}
			},
			{
				element: '#mcp-servers',
				popover: {
					popoverClass: POPOVER_CLASS,
					title: 'Get Visibility Into Your MCP Servers',
					description:
						'On top of seeing all MCP servers in one place, you can also see the status of each server, logs in real time, and more.',
					side: 'bottom' as const,
					align: 'end' as const,
					onPopoverRender: tourImageRender(
						tourImg.audit.mark,
						tourImg.audit.src,
						'Get Visibility Into Your MCP Servers'
					)
				}
			},
			stepMcpRegistries,
			{
				element: '#skills',
				popover: {
					popoverClass: `${POPOVER_CLASS} tour-skills-arrow-center`,
					title: 'Manage & Control Skills',
					description:
						'In addition to MCP servers, you can also manage and control skills. Import your skills through a Source URL & set up access policies to determine who can access each skill.',
					side: 'right' as const,
					align: 'center' as const,
					onPopoverRender: tourImageRender(
						tourImg.skills.mark,
						tourImg.skills.src,
						'Manage & Control Skills'
					)
				}
			},
			stepAgent
		];

		const defaultUserSteps = [
			...(isAtLeastPowerUser
				? [
						{
							element: '#add-mcp-server-button',
							popover: {
								popoverClass: POPOVER_CLASS,
								title: 'Deploy A New MCP Server',
								description:
									'Click the "Add MCP Server" button to deploy a new MCP server. You can choose to deploy a new server, whether shared or single instance per user, or connect to an existing remote server.',
								side: 'bottom' as const,
								align: 'end' as const,
								onPopoverRender: tourImageRender(
									tourImg.configure.mark,
									tourImg.configure.src,
									'Deploy A New MCP Server'
								)
							}
						}
					]
				: []),
			{
				element: 'table tr:nth-child(2)',
				popover: {
					popoverClass: POPOVER_CLASS,
					title: 'Connect To Available MCP Servers',
					description:
						'Click "Connect To Server" to get set up & start using them in your preferred client or IDE.',
					side: 'bottom' as const,
					align: 'end' as const,
					onPopoverRender: tourImageRender(
						tourImg.connect.mark,
						tourImg.connect.src,
						'Connect To Available MCP Servers'
					)
				}
			},
			...(isAtLeastPowerUser
				? [
						{
							element: 'table tr:nth-child(2)',
							popover: {
								popoverClass: POPOVER_CLASS,
								title: 'Get Visibility Into Your MCP Servers',
								description:
									"For MCP servers you've created, you can see the status of each server, logs in real time, and more.",
								side: 'bottom' as const,
								align: 'end' as const,
								onPopoverRender: tourImageRender(
									tourImg.audit.mark,
									tourImg.audit.src,
									'Get Visibility Into Your MCP Servers'
								)
							}
						},
						stepMcpRegistries
					]
				: []),
			stepAgent
		];

		function markTourCompleted() {
			localStorage.setItem(TOUR_COMPLETED_KEY, 'true');
		}

		const tour = driver({
			showProgress: false,
			overlayClickBehavior: noop,
			overlayColor: darkMode.isDark ? 'rgba(0, 0, 0, 1)' : 'rgba(0, 0, 0, 0.35)',
			steps: [introStep, ...(hasAdminAccess ? adminSteps : defaultUserSteps)] as DriveStep[],
			onNextClick: (_element: Element | undefined, _step: DriveStep, ctx: { driver: Driver }) => {
				const { driver: d } = ctx;
				if (!d.hasNextStep()) {
					markTourCompleted();
					tour.destroy();
				} else {
					d.moveNext();
				}
			},
			onCloseClick: () => {
				markTourCompleted();
				tour.destroy();
			}
		});
		tour.drive();

		return () => {
			if (logoMount) {
				unmount(logoMount);
			}
			tour.destroy();
		};
	});
</script>

<style lang="postcss">
	:global {
		.driver-popover {
			box-shadow: var(--shadow-md);
			border: 1px solid transparent;
			background-color: var(--color-background);
			color: var(--color-black);
			border-radius: var(--radius-md);

			.dark & {
				border: 1px solid var(--surface3);
				background-color: var(--surface2);
				color: var(--color-white);
			}
		}
		.driver-popover-description {
			font-size: var(--text-sm);
			font-family: var(--default-font-family);
		}

		.driver-popover-footer button {
			display: flex;
			align-items: center;
			gap: 0.25rem;
			font-size: 0.875rem;
			line-height: 1;
			text-shadow: none;
			font-family: var(--default-font-family);
			font-size: var(--text-xs);

			border-radius: 1.5rem;
			padding: 0.5rem 1.25rem;
			background-color: var(--surface3);
			border-width: 0;
			transition-property: color, background-color;
			transition-duration: 200ms;

			.dark & {
				color: var(--color-white);
			}

			&:hover {
				background-color: color-mix(in oklab, var(--surface3) 90%, var(--color-black));
				border-color: color-mix(in oklab, var(--surface3) 90%, var(--color-black));
			}

			.dark &:hover {
				background-color: color-mix(in oklab, var(--surface3) 90%, var(--color-white));
			}
		}

		.driver-popover-footer button.driver-popover-next-btn {
			background-color: var(--color-primary);
			color: var(--color-white);

			&:hover {
				background-color: color-mix(in oklab, var(--color-primary) 90%, var(--color-black));
				border-color: color-mix(in oklab, var(--color-primary) 90%, var(--color-black));
			}

			.dark &:hover {
				background-color: color-mix(in oklab, var(--color-primary) 90%, var(--color-white));
			}
		}

		.driver-popover-arrow {
			border: 5px solid var(--color-background);
			.dark & {
				border: 5px solid var(--surface3);
			}
		}

		.driver-popover-close-btn {
			color: color-mix(in oklab, var(--color-on-background) 75%, var(--color-white));
			&:hover {
				color: var(--color-on-background);
			}

			.dark & {
				color: color-mix(in oklab, var(--color-on-background) 75%, var(--color-black));
				&:hover {
					color: var(--color-on-background);
				}
			}
		}

		.driver-popover-arrow.driver-popover-arrow-side-right {
			border-left-color: transparent;
			border-bottom-color: transparent;
			border-top-color: transparent;
		}

		.driver-popover.tour-skills-arrow-center
			.driver-popover-arrow.driver-popover-arrow-side-left.driver-popover-arrow-align-end,
		.driver-popover.tour-skills-arrow-center
			.driver-popover-arrow.driver-popover-arrow-side-right.driver-popover-arrow-align-end {
			top: 50%;
			margin-top: -5px;
			bottom: auto;
		}

		.driver-popover.tour-skills-arrow-center
			.driver-popover-arrow.driver-popover-arrow-side-top.driver-popover-arrow-align-end,
		.driver-popover.tour-skills-arrow-center
			.driver-popover-arrow.driver-popover-arrow-side-bottom.driver-popover-arrow-align-end {
			left: 50%;
			margin-left: -5px;
			right: auto;
		}
	}
</style>
