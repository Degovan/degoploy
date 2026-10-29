<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { runtimeKeys, runtimesApi } from '#lib/api/runtimes.js';
	import RuntimeSection from '#lib/components/runtime-section.svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	// Requests that install, started on this page. Declared before the query,
	// which reads it as soon as it is created.
	let busy = $state(0);
	// While anything installs (from here, the CLI or a deployment), poll for its
	// progress. busy is read here, not in refetchInterval, so the options
	// change (and polling starts) as soon as an install starts.
	const runtimes = createQuery(() => {
		const polling = busy > 0;
		return {
			queryKey: runtimeKeys.all,
			queryFn: runtimesApi.list,
			refetchInterval: (q) =>
				polling || q.state.data?.some((r) => r.installing.length) ? 500 : false
		};
	});
	const runtime = $derived(runtimes.data?.find((r) => r.name === data.name));
</script>

<svelte:head>
	<title>{runtime?.title ?? 'Runtimes'} · Runtimes · degoploy</title>
</svelte:head>

{#if runtimes.isError}
	<Alert.Root variant="destructive">
		<CircleAlertIcon />
		<Alert.Title>{runtimes.error.message}</Alert.Title>
	</Alert.Root>
{:else if !runtimes.data}
	<Skeleton class="h-[32rem] rounded-xl" />
{:else if !runtime}
	<Alert.Root>
		<CircleAlertIcon />
		<Alert.Title>No runtime called “{data.name}”</Alert.Title>
		<Alert.Description>Pick one from the menu.</Alert.Description>
	</Alert.Root>
{:else}
	{#key runtime.name}
		<RuntimeSection {runtime} onbusy={(d) => (busy += d)} />
	{/key}
{/if}
