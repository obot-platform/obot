const store = $state({
	isMobile: false
});

if (typeof window !== 'undefined') {
	const mediaQuery = window.matchMedia('(max-width: 640px)');
	store.isMobile = mediaQuery.matches;

	mediaQuery.addEventListener('change', (e) => {
		store.isMobile = e.matches;
		console.log(store.isMobile);
	});
}

export default store;
