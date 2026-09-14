import { getExpandAdvancedPaneAction } from '../actions';
import { SIDEBAR_AI_RESOURCES_COLLAPSE, SIDEBAR_SKILLS_LINK } from '../mcp/constants';
import type { GuideHighlight, GuideListener, GuideStep } from '../types';

const highlightSkillsLink: GuideHighlight = {
	selector: {
		id: SIDEBAR_SKILLS_LINK
	},
	title: 'Skills',
	description: 'Click here to view the skills you have access to.'
};

const listenSkillsLink: GuideListener = {
	id: SIDEBAR_SKILLS_LINK,
	action: {
		success: true
	}
};

export const steps: GuideStep[] = [
	{
		content: [
			'To get started, view the skills you have access to. Go to the Skills page under AI Resources in the left sidebar.'
		],
		action: [
			{
				elementExists: SIDEBAR_SKILLS_LINK,
				highlight: highlightSkillsLink,
				listener: listenSkillsLink
			},
			getExpandAdvancedPaneAction({
				elementMissing: SIDEBAR_SKILLS_LINK,
				highlight: highlightSkillsLink,
				listener: listenSkillsLink,
				parentID: SIDEBAR_AI_RESOURCES_COLLAPSE,
				title: 'Expand AI Resources',
				description: 'Expand AI Resources to access Skills.'
			})
		]
	},
	{
		content: ["For the purpose of this guide, let's install a skill."],
		action: {
			highlight: {
				selector: {
					beginsWith: ['install-skill-btn-container']
				},
				title: 'Install Skill',
				description: 'Click here to begin installing the skill.',
				side: 'left',
				align: 'end'
			},
			listener: {
				beginsWith: ['install-skill-btn-container'],
				action: {
					success: true
				}
			}
		}
	},
	{
		content: ['To install the skill, follow the instructions on the install dialog.'],
		action: {
			highlight: {
				selector: {
					id: 'download-skill-container'
				},
				title: 'Download the Zip File',
				description: "To install the skill, you'll first need to download the zip file."
			},
			listener: {
				id: 'download-skill-container',
				skipClickTargetOnNext: true,
				action: {
					highlight: {
						selector: {
							id: 'install-skill-os-selector'
						},
						title: 'Select Your Operating System',
						description:
							'Select your operating system to see the appropriate CLI commands for installing the skill.'
					},
					listener: {
						id: 'install-skill-os-selector',
						skipClickTargetOnNext: true,
						action: {
							highlight: {
								selector: {
									id: 'unzip-skill-commands-container'
								},
								title: 'Copy & Paste the Unzip Command',
								description:
									'After installing, run the appropriate command for your operating system to unzip it to the appropriate directory.'
							},
							listener: {
								id: 'unzip-skill-commands-container',
								action: {
									highlight: {
										selector: {
											id: 'install-skill-dialog-content'
										},
										title: 'Try it Out!',
										description: 'Try using the appropriate CLI command here to install the skill!'
									},
									next: {
										action: {
											success: true,
											elementExists: 'install-skill-dialog',
											closeExistingElement: true
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
];

export default {
	steps,
	title: 'Discover & Install Skills',
	description: 'View the skills you have access to and install them.',
	id: 'skills-install-guide'
};
