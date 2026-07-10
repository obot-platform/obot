import type { GuideHighlight, GuideListener, GuideStep } from '../types';

const SIDEBAR_LINK_INSTALL_CLI = 'sidebar-link-install-cli';

const highlightInstallCliLink: GuideHighlight = {
	selector: {
		id: SIDEBAR_LINK_INSTALL_CLI
	},
	title: 'Install CLI',
	description: 'Click here to view the install CLI page.'
};

const listenInstallCliLink: GuideListener = {
	id: SIDEBAR_LINK_INSTALL_CLI,
	action: {
		success: true
	}
};

export const steps: GuideStep[] = [
	{
		content: ["To begin, let's head to the Install CLI page!"],
		button: {
			text: 'Where is the Install CLI page?',
			action: [
				{
					elementExists: 'back-to-app-btn',
					highlight: {
						selector: {
							id: 'back-to-app-btn'
						},
						title: 'Return to App',
						description: 'Click here to return to the app.'
					},
					listener: {
						id: 'back-to-app-btn',
						action: {
							highlight: highlightInstallCliLink,
							listener: listenInstallCliLink
						}
					}
				},
				{
					highlight: highlightInstallCliLink,
					listener: listenInstallCliLink
				}
			]
		}
	},
	{
		content: [
			'Great! From this page, you can view the instructions to install the CLI or take a look at the commands available to you.'
		],
		button: {
			text: 'Where is the CLI Commands section?',
			action: {
				highlight: {
					selector: {
						id: 'obot-cli-installation'
					},
					title: 'Install CLI here',
					description:
						'Here are the instructions to install the CLI! Go ahead and copy the commands or select the installer.',
					side: 'left',
					align: 'start'
				},
				listener: {
					beginsWith: [
						'obot-cli-homebrew-install',
						'obot-cli-setup-command',
						'obot-cli-windows-installer'
					],
					action: {
						success: true
					}
				}
			}
		}
	},
	{
		content: ['Once you have installed the CLI, run obot setup to get started!'],
		button: {
			text: "I've installed the CLI, what next?",
			action: {
				success: true
			}
		}
	},
	{
		content: [
			'Run obot skills search to begin seeing what skills you have access to!',
			"Obot may prompt you to authenticate once you've run the command.",
			'There are a default set of skills that come with Obot. Try installing the Algorithmic Art using your AI client or from the terminal via:',
			'obot skills install --destination <your-ai-client-skills-directory> <skill-id>'
		],
		button: {
			text: "I've installed the skill, what is next?",
			action: {
				success: true
			}
		}
	},
	{
		content: [
			'Great! Use your AI client to call the skill. Or go ahead and install other skills you now have access to!',
			"And that's it! You've completed the install Obot/Obot Sentry CLI guide."
		]
	}
];

export default {
	steps,
	title: 'Install Obot/Obot Sentry CLI',
	id: 'cli-install-guide'
};
