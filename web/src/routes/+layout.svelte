<script lang="ts">
	import './layout.css';
	import favicon from '#lib/assets/favicon.svg';
	import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
	import { onMount } from 'svelte';
	import { ModeWatcher, setMode } from 'mode-watcher';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { ApiError, setUnauthorizedHandler } from '#lib/api/client.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { Toaster } from '#lib/components/ui/sonner/index.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	// Dark is the default look. ModeWatcher's defaultMode needs its head script (blocked
	// by our CSP) and it stores "system" on load, so apply dark once per browser here;
	// a theme picked later in the user menu is kept.
	// The loading screen in app.html stays until the first page is ready. One
	// that never showed (a fast load) goes at once; a visible one fades out.
	onMount(() => {
		const splash = document.getElementById('splash');
		if (!splash) return;
		const opacity = Number(getComputedStyle(splash).opacity);
		if (opacity < 0.05 || matchMedia('(prefers-reduced-motion: reduce)').matches) {
			splash.remove();
			return;
		}
		splash
			.animate([{ opacity }, { opacity: 0 }], {
				duration: 180,
				easing: 'ease-out',
				fill: 'forwards'
			})
			.finished.finally(() => splash.remove());
	});

	onMount(() => {
		try {
			if (localStorage.getItem('degoploy.theme-default')) return;
			localStorage.setItem('degoploy.theme-default', 'dark');
			setMode('dark');
		} catch {
			// Storage unavailable: the system preference applies.
		}
	});

	const queryClient = new QueryClient({
		defaultOptions: {
			queries: {
				staleTime: 30_000,
				refetchOnWindowFocus: false,
				// Auth/permission errors will not fix themselves on retry.
				retry: (count, err) =>
					!(err instanceof ApiError && err.status >= 400 && err.status < 500) && count < 2
			}
		}
	});

	setUnauthorizedHandler(() => {
		if (!session.me) return;
		session.clear();
		queryClient.clear();
		goto(resolve(`/login?next=${encodeURIComponent(page.url.pathname + page.url.search)}`));
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>degoploy</title>
</svelte:head>

<!-- Head script injection is disabled: it would be an inline script blocked by our CSP. -->
<ModeWatcher defaultMode="dark" disableHeadScriptInjection />
<Toaster richColors position="top-right" />

<QueryClientProvider client={queryClient}>
	{@render children()}
</QueryClientProvider>
