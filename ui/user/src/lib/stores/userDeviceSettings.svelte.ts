import { browser } from '$app/environment';
import { AiClient } from '$lib/services/user/constants';

const store = $state({
	timeFormat: getTimeFormat(),
	setTimeFormat,
	initialize,
	aiClientPreference: getAiClientPreference(),
	setAiClientPreference
});

function initialize() {
	if (!browser) {
		return;
	}
	store.timeFormat = getTimeFormat();
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
			localStorage.removeItem('aiClientPreference');
		} else {
			localStorage.setItem('aiClientPreference', values.join(','));
		}
	}
	store.aiClientPreference = values.length ? values : getAiClientPreference();
}

function getAiClientPreference(): AiClient[] | undefined {
	const fallback: AiClient[] = [AiClient.Cursor, AiClient.Claude, AiClient.Codex, AiClient.VSCode];
	if (!browser) {
		return fallback;
	}

	const raw = localStorage.getItem('aiClientPreference');
	if (!raw) return fallback;

	const valid = raw
		.split(',')
		.map((s) => s.trim())
		.filter((s): s is AiClient => (Object.values(AiClient) as string[]).includes(s));
	return valid.length ? valid : fallback;
}

export default store;
