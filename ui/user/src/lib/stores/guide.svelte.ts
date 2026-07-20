/**
 * Central store for user onboarding guides.
 */
import type { Guide, GuideStep } from '$lib/services/guides/types';

interface GuideStore {
	selectedGuide: Guide | undefined;
	previousGuide: Guide | undefined;
	activeSteps: GuideStep[];
	stream: GuideStep[];
	currentStep: number;
	revealed: { contentCount: number; showButton: boolean }[];
	showObotInPanel: boolean;
	showObotInGuide: boolean;
}

const store = $state<GuideStore>({
	selectedGuide: undefined,
	previousGuide: undefined,
	activeSteps: [],
	stream: [],
	currentStep: 0,
	revealed: [],
	showObotInPanel: true,
	showObotInGuide: true
});

export default store;
