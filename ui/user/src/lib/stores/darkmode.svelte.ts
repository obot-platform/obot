const store = $state({
	isDark: getIsDark(),
	setDark
});

function setDark(darkMode: boolean) {
	const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
	if (darkMode && prefersDark) {
		localStorage.removeItem('theme');
	} else if (darkMode) {
		localStorage.setItem('theme', 'dark');
	} else if (!prefersDark) {
		localStorage.removeItem('theme');
	} else {
		localStorage.setItem('theme', 'light');
	}
	store.isDark = getIsDark();
}

function getIsDark(): boolean {
	if (typeof window === 'undefined') {
		return false;
	}
	const theme = localStorage.getItem('theme') ?? 'system';
	if (theme === 'dark') {
		return true;
	} else if (theme === 'light') {
		return false;
	}
	const mm = window.matchMedia('(prefers-color-scheme: dark)');
	return mm.matches;
}

if (typeof window !== 'undefined') {
	const mm = window.matchMedia('(prefers-color-scheme: dark)');
	mm.addEventListener('change', () => {
		store.isDark = getIsDark();
	});
}

export default store;
