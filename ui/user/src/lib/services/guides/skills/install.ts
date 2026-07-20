import type { GuideHighlight, GuideListener, GuideStep } from '../types';

const SIDEBAR_SKILLS_LINK = 'sidebar-link-mcp-skills';

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
			'To get started, view the skills you have access to. Go to the Skills page located in the left sidebar.'
		],
		action: [
			{
				elementExists: 'back-to-app-btn',
				highlight: {
					selector: {
						id: 'back-to-app-btn'
					},
					title: 'Return to User Consumption View',
					description: 'Click here to return to the user consumption view.'
				},
				listener: {
					id: 'back-to-app-btn',
					action: {
						highlight: highlightSkillsLink,
						listener: listenSkillsLink
					}
				}
			},
			{
				highlight: highlightSkillsLink,
				listener: listenSkillsLink
			}
		]
	},
	{
		content: ['Have you installed the Obot CLI?'],
		buttons: [
			{
				text: 'Yes, I have installed the Obot CLI',
				steps: [
					{
						content: [
							'Great! You can install skills using the Obot CLI.',
							'Use the following to list skills you have access to:',
							{ text: 'obot skills list', type: 'code' },
							"Let's use the Algorithmic Art skill as an example. Run the following command to install it:",
							{ text: 'obot skills install sk1-default-skills-algorithmic-art', type: 'code' },
							'Once the command completes, you can use the Algorithmic Art skill in your client.'
						]
					}
				]
			},
			{
				text: 'No, I have not installed the Obot CLI',
				steps: [
					{
						content: ['Download the Algorithmic Art skill to continue.'],
						action: {
							highlight: {
								selector: {
									beginsWith: ['download-skill-sk1-default-skills-algorithmic-art']
								},
								title: 'Download Algorithmic Art',
								description: 'Click here to download the Algorithmic Art skill.',
								side: 'left'
							},
							listener: {
								beginsWith: ['download-skill-sk1-default-skills-algorithmic-art'],
								skipClickOnNext: true,
								action: {
									success: true
								}
							}
						}
					},
					{
						content: [
							'After downloading the skill archive, extract it into the skills directory used by your client:',
							'Claude:',
							{ text: 'unzip ~/[path]/algorithmic-art.zip -d ~/.claude/skills', type: 'code' },
							'Cursor:',
							{ text: 'unzip ~/[path]/algorithmic-art.zip -d ~/.cursor/skills', type: 'code' },
							'Codex:',
							{ text: 'unzip ~/[path]/algorithmic-art.zip -d ~/.codex/skills', type: 'code' },
							'Global/Other:',
							{ text: 'unzip ~/[path]/algorithmic-art.zip -d ~/.agents/skills', type: 'code' },
							'Or if you are using Claude Desktop, install the skill by dragging & dropping into "Upload a Skill"',
							{
								imageUrl: '/user/images/guides/claude-upload-skill.webp',
								alt: 'Upload a Skill in Claude Desktop'
							},
							'1. Click the + in chat box',
							'2. Go to Skills > Manage Skills > Add > Upload a Skill',
							'3. Drag the downloaded zip file in this dialog that appears'
						]
					}
				]
			}
		]
	}
];

export default {
	steps,
	title: 'Discover & Install Skills',
	description: 'View the skills you have access to and install them.',
	id: 'skills-install-guide'
};
