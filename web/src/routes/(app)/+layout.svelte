<script lang="ts">
	import { onMount } from 'svelte';
	import { useQueryClient } from '@tanstack/svelte-query';
	import * as Sidebar from '#lib/components/ui/sidebar/index.js';
	import { Separator } from '#lib/components/ui/separator/index.js';
	import AppSidebar from '#lib/components/app-sidebar.svelte';
	import { cacheDeployment, deploymentKeys } from '#lib/api/deployments.js';
	import { projectKeys } from '#lib/api/projects.js';
	import { ruleKeys } from '#lib/api/rules.js';
	import type { Deployment } from '#lib/api/types.gen.js';
	import { realtime } from '#lib/realtime.svelte.js';
	import { cn } from '#lib/utils.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
	const queryClient = useQueryClient();

	// One realtime connection for the signed-in app; events keep query caches fresh.
	onMount(() => {
		realtime.connect();
		const unsubscribe = realtime.subscribe('events', (m) => {
			const pid = m.project_id;
			if (m.type === 'deployment.updated') {
				const d = m.data as Deployment;
				cacheDeployment(queryClient, d);
				queryClient.invalidateQueries({ queryKey: deploymentKeys.list(d.project_id) });
				queryClient.invalidateQueries({ queryKey: projectKeys.all, exact: true });
			} else if (m.type === 'webhook.received' && pid) {
				queryClient.invalidateQueries({ queryKey: ruleKeys.events(pid) });
			}
		});
		return () => {
			unsubscribe();
			realtime.disconnect();
		};
	});

	// After a reconnect (not the first connect) events may have been missed: refetch.
	let connectedBefore = false;
	$effect(() => {
		if (realtime.status !== 'open') return;
		if (connectedBefore) queryClient.invalidateQueries();
		connectedBefore = true;
	});

	const live = $derived(
		{
			open: { label: 'Live', dot: 'bg-emerald-500' },
			connecting: { label: 'Connecting…', dot: 'bg-amber-500 animate-pulse' },
			closed: { label: 'Reconnecting…', dot: 'bg-amber-500 animate-pulse' },
			idle: { label: 'Offline', dot: 'bg-muted-foreground' }
		}[realtime.status]
	);
</script>

<Sidebar.Provider>
	<AppSidebar />
	<Sidebar.Inset class="min-w-0">
		<header class="flex h-14 shrink-0 items-center gap-2 border-b px-4">
			<Sidebar.Trigger class="-ml-1" />
			<Separator orientation="vertical" class="mr-2 data-[orientation=vertical]:h-4" />
			<span
				class="ml-auto flex items-center gap-1.5 text-xs text-muted-foreground"
				title="Realtime connection"
				data-testid="realtime-status"
			>
				<span class={cn('size-2 rounded-full', live.dot)}></span>
				{live.label}
			</span>
		</header>
		<main class="flex min-w-0 flex-1 flex-col gap-6 p-4 md:p-6">
			{@render children()}
		</main>
	</Sidebar.Inset>
</Sidebar.Provider>
