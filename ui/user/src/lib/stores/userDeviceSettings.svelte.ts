import { browser } from '$app/environment';

const store = $state({
	timeFormat: getTimeFormat(),
	developerMode: getDeveloperMode(),
	setTimeFormat,
	setDeveloperMode,
	initialize
});

function initialize() {
	if (!browser) {
		return;
	}
	store.timeFormat = getTimeFormat();
	store.developerMode = getDeveloperMode();
}

function setTimeFormat(timeFormat: '12h' | '24h') {
	if (browser) {
		localStorage.setItem('timeFormat', timeFormat);
	}
	store.timeFormat = timeFormat;
}

function setDeveloperMode(developerMode: boolean) {
	if (browser) {
		localStorage.setItem('developerMode', developerMode.toString());
	}
	store.developerMode = developerMode;
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

function getDeveloperMode(): boolean {
	if (!browser) {
		return false;
	}
	const developerMode = localStorage.getItem('developerMode') ?? 'false';
	return developerMode === 'true';
}

export default store;
