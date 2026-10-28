<script lang="ts">
	import { untrack } from 'svelte';
	import { useQueryClient } from '@tanstack/svelte-query';
	import AppHeader from '#lib/components/app-header.svelte';
	import PasswordChangeGate from '#lib/components/password-change-gate.svelte';
	import { cacheDeployment, deploymentKeys, isActive } from '#lib/api/deployments.js';
	import { projectKeys } from '#lib/api/projects.js';
	import { ruleKeys } from '#lib/api/rules.js';
	import type { Deployment } from '#lib/api/types.gen.js';
	import { realtime } from '#lib/realtime.svelte.js';
	import { session } from '#lib/stores/session.svelte.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
	const queryClient = useQueryClient();

	// After an admin reset with "require a new password", the API refuses
	// everything else, so the app waits behind the gate.
	const mustChange = $derived(!!session.user?.must_change_password);

	// One realtime connection for the signed-in app; events keep query caches fresh.
	function connectRealtime() {
		realtime.connect();
		const unsubscribe = realtime.subscribe('events', (m) => {
			const pid = m.project_id;
			if (m.type === 'deployment.updated') {
				const d = m.data as Deployment;
				cacheDeployment(queryClient, d);
				queryClient.invalidateQueries({ queryKey: deploymentKeys.list(d.project_id) });
				// Covers the recent list and the active (queue) list.
				queryClient.invalidateQueries({ queryKey: deploymentKeys.recent });
				queryClient.invalidateQueries({ queryKey: projectKeys.all, exact: true });
				// Health changes once a deployment finishes.
				if (!isActive(d)) {
					queryClient.invalidateQueries({
						queryKey: projectKeys.detail(d.project_id),
						exact: true
					});
				}
			} else if (m.type === 'webhook.received' && pid) {
				queryClient.invalidateQueries({ queryKey: ruleKeys.events(pid) });
			}
		});
		return () => {
			unsubscribe();
			realtime.disconnect();
		};
	}
	$effect(() => {
		if (mustChange) return;
		return untrack(connectRealtime);
	});

	// After a reconnect (not the first connect) events may have been missed: refetch.
	let connectedBefore = false;
	$effect(() => {
		if (realtime.status !== 'open') return;
		if (connectedBefore) queryClient.invalidateQueries();
		connectedBefore = true;
	});
</script>

{#if mustChange}
	<PasswordChangeGate />
{:else}
	<div class="flex min-h-svh flex-col">
		<AppHeader />
		<main class="flex min-w-0 flex-1 flex-col pb-16">
			{@render children()}
		</main>
	</div>
{/if}
