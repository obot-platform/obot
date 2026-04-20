<script lang="ts">
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import { profile } from '$lib/stores';
	// import { isSameDay } from 'date-fns';
	import { driver } from 'driver.js';
	import 'driver.js/dist/driver.css';
	import { mount, unmount } from 'svelte';

	let hasAdminAccess = $derived(profile.current?.hasAdminAccess?.() ?? false);
	// let isNewUser = $derived(
	// 	profile.current?.created && isSameDay(new Date(profile.current?.created), new Date())
	// );

	$effect(() => {
		let logoMount: Record<string, unknown> | undefined;

		// if (isNewUser) {
		// 	tour.drive();
		// }
		const tour = driver({
			showProgress: false,
			steps: hasAdminAccess
				? [
						{
							popover: {
								title: '',
								description:
									"Looks like it's your first time here! Let's walk you through what Obot has to offer.",
								side: 'top',
								align: 'center',
								popoverClass: 'text-sm',
								onPopoverRender: (popover) => {
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
						},
						{
							element: '#mcp-servers',
							popover: {
								popoverClass: 'w-xl! max-w-xl! min-w-xl!',
								title: 'Deploy & Connect Your MCP Servers',
								description:
									'Set up your MCP servers to start using them through the gateway. You can set up against an existing remote server, have a instance created per user, or start a shared server that is accessible to everyone who has access.',
								side: 'bottom',
								align: 'end',
								onPopoverRender: (popover) => {
									if (popover.wrapper.querySelector('[data-tour-mcpservers-configure]')) {
										return;
									}
									const wrapper = document.createElement('div');
									wrapper.className =
										'flex items-center justify-center my-2 w-full overflow-hidden rounded-md';
									wrapper.setAttribute('data-tour-mcpservers-configure', '');
									const img = document.createElement('img');
									img.src = '/admin/assets/tour/mcpservers_1.webp';
									img.alt = 'MCP servers configuration';
									img.className = 'h-96 w-full object-contain object-top';
									wrapper.appendChild(img);
									popover.wrapper.prepend(wrapper);
								}
							}
						},
						{
							element: '#mcp-servers',
							popover: {
								popoverClass: 'w-xl! max-w-xl! min-w-xl!',
								title: 'Get Visibility Into Your MCP Servers',
								description:
									'On top of seeing all MCP servers in one place, you can also see the status of each server, logs in real time, and more.',
								side: 'bottom',
								align: 'end',
								onPopoverRender: (popover) => {
									if (popover.wrapper.querySelector('[data-tour-mcpservers-audit]')) {
										return;
									}
									const wrapper = document.createElement('div');
									wrapper.className =
										'flex items-center justify-center my-2 w-full overflow-hidden rounded-md';
									wrapper.setAttribute('data-tour-mcpservers-audit', '');
									const img = document.createElement('img');
									img.src = '/admin/assets/tour/mcpservers_2.webp';
									img.alt = 'Get Visibility Into Your MCP Servers';
									img.className = 'h-96 w-full object-contain object-top';
									wrapper.appendChild(img);
									popover.wrapper.prepend(wrapper);
								}
							}
						},
						{
							element: '#mcp-registries',
							popover: {
								popoverClass: 'w-xl! max-w-xl! min-w-xl!',
								title: 'Control MCP Server Accessibility',
								description:
									'Set up access control rules to determine who can access each MCP server. You can allow access to specific users or groups, or even everyone.',
								side: 'bottom',
								align: 'end',
								onPopoverRender: (popover) => {
									if (popover.wrapper.querySelector('[data-tour-mcpregistries]')) {
										return;
									}
									const wrapper = document.createElement('div');
									wrapper.className =
										'flex items-center justify-center my-2 w-full overflow-hidden rounded-md';
									wrapper.setAttribute('data-tour-mcpregistries', '');
									const img = document.createElement('img');
									img.src = '/admin/assets/tour/mcpregistries.webp';
									img.alt = 'Control MCP Server Accessibility';
									img.className = 'h-96 w-full object-contain object-top';
									wrapper.appendChild(img);
									popover.wrapper.prepend(wrapper);
								}
							}
						},
						{
							element: '#skills',
							popover: {
								popoverClass: 'w-xl! max-w-xl! min-w-xl! tour-skills-arrow-center',
								title: 'Manage & Control Skills',
								description:
									'In addition to MCP servers, you can also manage and control skills. Import your skills through a Source URL & set up access policies to determine who can access each skill.',
								side: 'right',
								align: 'center',
								onPopoverRender: (popover) => {
									if (popover.wrapper.querySelector('[data-tour-skills]')) {
										return;
									}
									const wrapper = document.createElement('div');
									wrapper.className =
										'flex items-center justify-center my-2 w-full overflow-hidden rounded-md';
									wrapper.setAttribute('data-tour-skills', '');
									const img = document.createElement('img');
									img.src = '/admin/assets/tour/skills.webp';
									img.alt = 'Manage & Control Skills';
									img.className = 'h-96 w-full object-contain object-top';
									wrapper.appendChild(img);
									popover.wrapper.prepend(wrapper);
								}
							}
						},
						{
							element: '#launch-agent-chat',
							popover: {
								popoverClass: 'w-xl! max-w-xl! min-w-xl! tour-skills-arrow-center',
								title: 'Use Built-In Agents to Accomplish Tasks',
								description:
									'The agents will be set up to access the MCP servers & skills based on your access policies. Automate processes, build workflows, and more.',
								side: 'right',
								align: 'center',
								onPopoverRender: (popover) => {
									if (popover.wrapper.querySelector('[data-tour-agent]')) {
										return;
									}
									const wrapper = document.createElement('div');
									wrapper.className =
										'flex items-center justify-center my-2 w-full overflow-hidden rounded-md';
									wrapper.setAttribute('data-tour-agent', '');
									const img = document.createElement('img');
									img.src = '/admin/assets/tour/agent.webp';
									img.alt = 'Use Built-in Agents to Accomplish Tasks';
									img.className = 'h-96 w-full object-contain object-top';
									wrapper.appendChild(img);
									popover.wrapper.prepend(wrapper);
								}
							}
						}
					]
				: []
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

			&:hover {
				background-color: color-mix(in oklab, var(--surface3) 90%, var(--color-black));
				border-color: color-mix(in oklab, var(--surface3) 90%, var(--color-black));
			}

			.dark &:hover {
				background-color: color-mix(in oklab, var(--surface3) 90%, var(--color-white));
			}
		}

		.driver-popover-footer .driver-popover-next-btn {
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
