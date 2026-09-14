import { MDM_DEVICES_CONFIGURATION_FIELD_IDS } from '$lib/constants';
import { getExpandAdvancedPaneAction } from '../actions';
import { SIDEBAR_OPERATIONS_COLLAPSE } from '../mcp/constants';
import type { GuideAction, GuideStep } from '../types';

function getInventoryTabAction(
	tabId: string,
	title: string,
	description: string,
	next: GuideAction | GuideAction[]
): GuideAction {
	return {
		highlight: {
			selector: { id: tabId },
			side: 'left',
			title,
			description
		},
		listener: {
			id: tabId,
			skipClickTargetOnNext: true,
			action: next
		}
	};
}

function getEnforcementEventsAction(): GuideAction[] {
	const highlight = {
		selector: { id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enforcementEventsLink },
		side: 'right' as const,
		title: 'Enforcement Events',
		description:
			'When enforcement is enabled and tool calls are made, any actions Obot Sentry takes against them will be recorded and viewable here.'
	};
	const listener = {
		id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enforcementEventsLink,
		skipClickTargetOnNext: true,
		action: { success: true }
	};

	return [
		{
			elementExists: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enforcementEventsLink,
			highlight,
			listener
		},
		getExpandAdvancedPaneAction({
			elementMissing: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enforcementEventsLink,
			highlight,
			listener,
			parentID: SIDEBAR_OPERATIONS_COLLAPSE,
			title: 'Expand Operations',
			description: 'Expand Operations to access Enforcement Events.'
		})
	];
}

const highlightInventoryLink = {
	selector: {
		id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesLink
	},
	title: 'Inventory',
	description: 'This is where you can manage devices and install Obot Sentry.'
};

const listenInventoryLink = {
	id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesLink,
	action: {
		success: true
	}
};

export const steps: GuideStep[] = [
	{
		content: [
			'In order to discover shadow AI and enforce policies for unmanaged MCP servers, you will need to install Obot Sentry on your devices.',
			'**What is Obot Sentry?** Obot Sentry is a lightweight program designed to be used by MDMs for device scanning and agent hook configuration. You can learn more about it [here](https://github.com/obot-platform/obot-sentry).',
			"To get set up, let's head to the Inventory page under Operations."
		],
		action: [
			{
				elementExists: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesLink,
				highlight: highlightInventoryLink,
				listener: listenInventoryLink
			},
			getExpandAdvancedPaneAction({
				elementMissing: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesLink,
				highlight: highlightInventoryLink,
				listener: listenInventoryLink,
				parentID: SIDEBAR_OPERATIONS_COLLAPSE,
				title: 'Expand Operations',
				description: 'Expand Operations to access Inventory.'
			})
		]
	},
	{
		content: ['The "Configuration" tab contains all the information needed to set up Obot Sentry.'],
		action: [
			{
				routeContains: '/inventory',
				elementExists: MDM_DEVICES_CONFIGURATION_FIELD_IDS.getStartedButton,
				highlight: {
					selector: {
						id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.getStartedButton
					},
					side: 'left' as const,
					title: 'Configure Managed Devices',
					description: 'Begin creating the initial managed-device configuration here.'
				},
				listener: {
					id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.getStartedButton,
					action: {
						success: true
					}
				}
			},
			{
				routeContains: '/inventory',
				elementMissing: MDM_DEVICES_CONFIGURATION_FIELD_IDS.getStartedButton,
				elementExists: MDM_DEVICES_CONFIGURATION_FIELD_IDS.configurationTab,
				highlight: {
					selector: {
						id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.configurationTab
					},
					side: 'right' as const,
					title: 'Open Configuration',
					description: 'Select the Configuration tab to install Obot Sentry.'
				},
				listener: {
					id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.configurationTab,
					action: {
						success: true
					}
				}
			}
		]
	},
	{
		content: ["We'll take you through the installation steps."],
		action: {
			highlight: {
				selector: {
					id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetup
				},
				side: 'top',
				title: 'Installation & Setup',
				description: 'Follow the steps here to install and setup Obot Sentry.',
				noDescendantInteraction: true
			},
			listener: {
				id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetup,
				skipClickTargetOnNext: true,
				action: {
					highlight: {
						selector: {
							id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-1`
						},
						side: 'top',
						title: 'Generate Enrollment Key',
						description:
							"First things first, you'll need to generate an enrollment key. Make sure to save the key when you create it! Its full value will only be shown once during creation."
					},
					listener: {
						id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-1`,
						skipClickTargetOnNext: true,

						action: {
							highlight: {
								selector: {
									id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-2`
								},
								side: 'top',
								title: 'Installation Method',
								description:
									'Select the appropriate installation method for your devices here. Manual installation requires you to manually install the Obot Sentry agent on each device. Microsoft Intune installation allows you to deploy the agent to multiple devices across your organization.'
							},
							listener: {
								id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-2`,
								skipClickTargetOnNext: true,
								action: {
									highlight: {
										selector: {
											id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-3`
										},
										side: 'top',
										title: 'Operating System',
										description:
											'Make sure to select the proper operating system for your devices here.'
									},
									listener: {
										id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-3`,
										skipClickTargetOnNext: true,
										action: {
											success: true
										}
									}
								}
							}
						}
					}
				}
			}
		}
	},
	{
		content: [
			"For more detailed instructions, here's a video showing how to deploy Obot Sentry with Microsoft Intune.",
			{
				videoUrl: 'https://youtu.be/NwuQlU5WpK0',
				title: 'Installing Obot Sentry w/ Intune'
			}
		],
		action: {
			highlight: {
				selector: {
					id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-5`
				},
				side: 'top',
				title: 'OS/Type Specific Instructions',
				description:
					"Depending on the installation type and OS selected, the instructions here will be different. Let's expand it for you."
			},
			listener: {
				id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-5`,
				action: {
					highlight: {
						selector: {
							id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentKeysSection
						},
						side: 'top',
						title: 'Enrollment Keys',
						description:
							"Once you've set up an enrollment key, its information will be displayed here. Generally, you'll only need a single enrollment key to register devices with, but if you need to rotate keys, you can create new enrollment keys from here.",
						noDescendantInteraction: true
					},
					listener: {
						id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentKeysSection,
						skipClickTargetOnNext: true,
						action: {
							highlight: {
								selector: {
									id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.toolCallEnforcementSection
								},
								side: 'top',
								title: 'Tool Call Enforcement',
								description:
									'Here you can control which tool calls can run on your enrolled devices when enforcement is enabled. Make sure to save your changes for them to take effect.',
								experimental: true
							},
							listener: {
								id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.toolCallEnforcementSection,
								action: {
									success: true
								}
							}
						}
					}
				}
			}
		}
	},
	{
		content: [
			'Once Obot Sentry has been installed and configured on your devices, you can view the results across the Inventory tabs — Overview, Devices, Device Clients, Device MCP Servers, and Device Skills — and review enforcement actions under Operations.'
		],
		action: getInventoryTabAction(
			MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesTabOverview,
			'Overview',
			'View an overall summary of scans sent through Obot Sentry over a given time period.',
			getInventoryTabAction(
				MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesTabDevices,
				'Devices',
				'View results for an individual device, including the most recent scan and historical ones.',
				getInventoryTabAction(
					MDM_DEVICES_CONFIGURATION_FIELD_IDS.inventoryTabDeviceMcpServers,
					'Device MCP Servers',
					'Browse MCP servers discovered across your enrolled devices.',
					getEnforcementEventsAction()
				)
			)
		)
	}
];

export default {
	steps,
	title: 'Discover Shadow AI & Enforce Policies for Unmanaged MCP Servers',
	description: 'Install Obot Sentry on devices to inventory, audit, and enforce.',
	id: 'devices-install-sentry-guide'
};
