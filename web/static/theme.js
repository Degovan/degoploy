// Applies the saved light or dark mode before the first paint, so a reload
// does not flash the light theme while the app loads. It mirrors
// mode-watcher's setInitialMode with dark as the default (see the root
// layout); it is a file because the CSP blocks inline scripts.
(function () {
	var mode = 'dark';
	try {
		mode = localStorage.getItem('mode-watcher-mode') || 'dark';
	} catch {
		// Storage unavailable: keep the default.
	}
	var light =
		mode === 'light' ||
		(mode === 'system' && window.matchMedia('(prefers-color-scheme: light)').matches);
	var root = document.documentElement;
	root.classList.toggle('dark', !light);
	root.style.colorScheme = light ? 'light' : 'dark';
})();
