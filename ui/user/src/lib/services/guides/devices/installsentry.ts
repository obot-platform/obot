import { MDM_DEVICES_CONFIGURATION_FIELD_IDS } from '$lib/constants';
import { getExpandAdvancedPaneAction } from '../actions';
import type { GuideStep } from '../types';

const highlightDevicesLink = {
	selector: {
		id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesLink
	},
	title: 'Devices',
	description: 'Click here to manage devices and install Obot Sentry.'
};

const listenDevicesLink = {
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
			'To get set up, head to the Devices page.'
		],
		action: [
			{
				elementExists: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesLink,
				highlight: highlightDevicesLink,
				listener: listenDevicesLink
			},
			getExpandAdvancedPaneAction({
				elementMissing: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesLink,
				highlight: highlightDevicesLink,
				listener: listenDevicesLink,
				parentID: 'sidebar-collapse-device-management',
				title: 'Expand Device Management',
				description: 'Device Management is collapsed. Click here to expand it.'
			})
		]
	},
	{
		content: ['Go to or set up the Obot Sentry configuration.'],
		action: [
			{
				routeContains: '/admin/devices',
				elementExists: MDM_DEVICES_CONFIGURATION_FIELD_IDS.getStartedButton,
				highlight: {
					selector: {
						id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.getStartedButton
					},
					side: 'left' as const,
					title: 'Configure Managed Devices',
					description: 'Click Get Started to create the initial managed-device configuration.'
				},
				listener: {
					id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.getStartedButton,
					action: {
						success: true
					}
				}
			},
			{
				routeContains: '/admin/devices',
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
		content: ['The Configuration tab contains the Install Obot Sentry workflow.'],
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
									'Select the appropriate installation method for your devices. Manual installation requires you to manually install the Obot Sentry agent on each device. Microsoft Intune installation allows you to deploy the agent to your devices using Microsoft Intune.'
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
										description: 'Make sure to select the proper operating system for your devices.'
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
					'Depending on the installation type and OS selected, the instructions here will be different. Click here to go ahead and expand it.'
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
					}
				}
			}
		}
	},
	{
		content: ['Once scans have been sent through Obot Sentry, you can view the results here.'],
		action: {
			highlight: {
				selector: {
					id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesTabOverview
				},
				title: 'See Overall Results',
				side: 'left',
				description:
					'View an overall summary of all the scans sent through Obot Sentry over a given time period.'
			},
			listener: {
				id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesTabOverview,
				skipClickTargetOnNext: true,
				action: {
					highlight: {
						selector: {
							id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesTabDevices
						},
						side: 'left',
						title: 'See Individual Device Scans',
						description:
							'Go here to view results of an individual device. See the more recent scan or view historical ones.'
					},
					listener: {
						id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesTabDevices,
						skipClickTargetOnNext: true,
						action: {
							success: true
						}
					}
				}
			}
		}
	}
];

export default {
	steps,
	title: 'Discover Shadow AI & Enforce Policies for Unmanaged MCP Servers',
	description: 'Install Obot Sentry on devices to inventory, audit, and enforce.',
	id: 'devices-install-sentry-guide'
};
