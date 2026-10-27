<script lang="ts">
	import './layout.css';
	import favicon from '#lib/assets/favicon.svg';
	import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
	import { ModeWatcher } from 'mode-watcher';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { ApiError, setUnauthorizedHandler } from '#lib/api/client.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { Toaster } from '#lib/components/ui/sonner/index.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

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
<ModeWatcher disableHeadScriptInjection />
<Toaster richColors position="top-right" />

<QueryClientProvider client={queryClient}>
	{@render children()}
</QueryClientProvider>
