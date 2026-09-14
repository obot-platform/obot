import { Group } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import {
	SkillsInstallGuide,
	McpCustomHostedGuide,
	McpCustomRemoteGuide,
	McpAccessPolicyCreateGuide,
	DevicesInstallSentryGuide,
	McpFiltersGuide
} from '.';

export function generateLessonItems() {
	const isAtLeastPoweruser = profile.current.groups.includes(Group.POWERUSER);
	const isAtLeastPowerUserPlus = profile.current.groups.includes(Group.POWERUSER_PLUS);
	return [
		...(isAtLeastPoweruser
			? [
					{
						label: McpCustomHostedGuide.title,
						description: McpCustomHostedGuide.description,
						guide: McpCustomHostedGuide
					},
					{
						label: McpCustomRemoteGuide.title,
						description: McpCustomRemoteGuide.description,
						guide: McpCustomRemoteGuide
					},
					...(isAtLeastPowerUserPlus
						? [
								{
									label: McpAccessPolicyCreateGuide.title,
									description: McpAccessPolicyCreateGuide.description,
									guide: McpAccessPolicyCreateGuide
								}
							]
						: []),
					...(profile.current.isAdmin?.()
						? [
								{
									label: McpFiltersGuide.title,
									description: McpFiltersGuide.description,
									guide: McpFiltersGuide
								},
								{
									label: DevicesInstallSentryGuide.title,
									description: DevicesInstallSentryGuide.description,
									guide: DevicesInstallSentryGuide
								}
							]
						: [])
				]
			: []),
		{
			label: SkillsInstallGuide.title,
			description: SkillsInstallGuide.description,
			guide: SkillsInstallGuide
		}
	];
}
