<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { runtimeKeys, runtimesApi, runtimeTitles } from '#lib/api/runtimes.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import { cn } from '#lib/utils.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	// The menu lists every runtime the server offers; each has its own page.
	const runtimes = createQuery(() => ({ queryKey: runtimeKeys.all, queryFn: runtimesApi.list }));
	const menu = $derived(
		runtimes.data?.map((r) => ({ name: r.name, title: r.title, enabled: r.enabled })) ??
			Object.entries(runtimeTitles).map(([name, title]) => ({ name, title, enabled: true }))
	);
</script>

<PageHeader
	title="Runtimes"
	description="Language versions for deploy scripts. Each project turns on the ones it needs, in its settings or through files in its repository."
/>

<div class="container-page grid gap-8 pt-8 pb-24 lg:grid-cols-[13.75rem_minmax(0,1fr)] lg:gap-12">
	<nav
		class="-mx-4 flex gap-0.5 overflow-x-auto px-4 lg:sticky lg:top-32 lg:mx-0 lg:flex-col lg:self-start lg:px-0"
		aria-label="Runtimes"
	>
		{#each menu as r (r.name)}
			{@const on = page.params.name === r.name}
			<a
				href={resolve('/(app)/runtimes/[name]', { name: r.name })}
				class={cn(
					'flex shrink-0 items-center justify-between gap-3 rounded-md px-3 py-2 text-sm whitespace-nowrap transition-colors',
					on
						? 'bg-muted font-medium text-foreground'
						: 'text-muted-foreground hover:text-foreground'
				)}
				aria-current={on ? 'page' : undefined}
			>
				{r.title}
				{#if !r.enabled}<span class="text-xs font-normal text-muted-foreground/70">Off</span>{/if}
			</a>
		{/each}
	</nav>

	<div class="flex min-w-0 flex-col gap-8">
		{@render children()}
	</div>
</div>
