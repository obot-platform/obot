import { m } from '$lib/i18n';
import { getExpandAdvancedPaneAction } from '../actions';
import { SIDEBAR_AI_RESOURCES_COLLAPSE, SIDEBAR_SKILLS_LINK } from '../mcp/constants';
import type { GuideHighlight, GuideListener, GuideStep } from '../types';

const highlightSkillsLink: GuideHighlight = {
	selector: {
		id: SIDEBAR_SKILLS_LINK
	},
	title: m.skills_title(),
	description: m.skills_guides_click_here_to_view_the_skills()
};

const listenSkillsLink: GuideListener = {
	id: SIDEBAR_SKILLS_LINK,
	action: {
		success: true
	}
};

export const steps: GuideStep[] = [
	{
		content: [m.skills_guides_to_get_started_view_the_skills()],
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
				title: m.core_guide_expand_ai_resources(),
				description: m.skills_guides_expand_ai_resources_to_access_skills()
			})
		]
	},
	{
		content: [m.skills_guides_for_the_purpose_of_this_guide()],
		action: {
			highlight: {
				selector: {
					beginsWith: ['install-skill-btn-container']
				},
				title: m.skills_guides_install_skill(),
				description: m.skills_guides_click_here_to_begin_installing_the(),
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
		content: [m.skills_guides_to_install_the_skill_follow_the()],
		action: {
			highlight: {
				selector: {
					id: 'download-skill-container'
				},
				title: m.skills_guides_download_the_zip_file(),
				description: m.skills_guides_to_install_the_skill_you_ll()
			},
			listener: {
				id: 'download-skill-container',
				skipClickTargetOnNext: true,
				action: {
					highlight: {
						selector: {
							id: 'install-skill-os-selector'
						},
						title: m.skills_guides_select_your_operating_system(),
						description: m.skills_guides_select_your_operating_system_to_see()
					},
					listener: {
						id: 'install-skill-os-selector',
						skipClickTargetOnNext: true,
						action: {
							highlight: {
								selector: {
									id: 'unzip-skill-commands-container'
								},
								title: m.skills_guides_copy_paste_the_unzip_command(),
								description: m.skills_guides_after_installing_run_the_appropriate_command()
							},
							listener: {
								id: 'unzip-skill-commands-container',
								action: {
									highlight: {
										selector: {
											id: 'install-skill-dialog-content'
										},
										title: m.skills_guides_try_it_out(),
										description: m.skills_guides_try_using_the_appropriate_cli_command()
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
	title: m.skills_guides_discover_install_skills(),
	description: m.skills_guides_view_the_skills_you_have_access(),
	id: 'skills-install-guide'
};
