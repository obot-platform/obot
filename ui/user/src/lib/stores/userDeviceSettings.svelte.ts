import { browser } from '$app/environment';
import { AI_CLIENT_PREFERENCE_KEY, OBOT_GUIDE_KEYS } from '$lib/constants';
import { AiClient } from '$lib/services/user/constants';

const store = $state({
	timeFormat: getTimeFormat(),
	setTimeFormat,
	initialize,
	aiClientPreference: getAiClientPreference(),
	setAiClientPreference,
	showAllGuides: getShowAllGuides(),
	setShowAllGuides
});

function initialize() {
	if (!browser) {
		return;
	}
	store.timeFormat = getTimeFormat();
	store.showAllGuides = getShowAllGuides();
	store.aiClientPreference = getAiClientPreference();
}

function setTimeFormat(timeFormat: '12h' | '24h') {
	if (browser) {
		localStorage.setItem('timeFormat', timeFormat);
	}
	store.timeFormat = timeFormat;
}

function getTimeFormat(): '12h' | '24h' {
	if (!browser) {
		return '12h';
	}
	const timeFormat = localStorage.getItem('timeFormat') ?? '12h';
	if (timeFormat === '24h') {
		return '24h';
	}
	return '12h';
}

function setAiClientPreference(aiClient: AiClient | AiClient[]) {
	const values = Array.isArray(aiClient) ? aiClient : [aiClient];
	if (browser) {
		if (values.length === 0) {
			localStorage.removeItem(AI_CLIENT_PREFERENCE_KEY);
		} else {
			localStorage.setItem(AI_CLIENT_PREFERENCE_KEY, values.join(','));
		}
	}
	store.aiClientPreference = values.length ? values : getAiClientPreference();
}

function getAiClientPreference(): AiClient[] {
	if (!browser) {
		return [];
	}

	const raw = localStorage.getItem(AI_CLIENT_PREFERENCE_KEY);
	if (raw === null) return [];
	if (raw.trim() === '') return [];

	const valid = raw
		.split(',')
		.map((s) => s.trim())
		.filter((s): s is AiClient => (Object.values(AiClient) as string[]).includes(s));
	return valid.length ? valid : [];
}

function getShowAllGuides(): boolean {
	if (!browser) {
		return false;
	}
	const showAllGuides = localStorage.getItem(OBOT_GUIDE_KEYS.SHOW_ALL_GUIDES);
	return showAllGuides ? showAllGuides === 'true' : true;
}

function setShowAllGuides(showAllGuides: boolean) {
	if (browser) {
		localStorage.setItem(OBOT_GUIDE_KEYS.SHOW_ALL_GUIDES, showAllGuides.toString());
	}
	store.showAllGuides = showAllGuides;
}

export default store;
