<script lang="ts">
	import type { Snippet } from 'svelte';

	// Full-width hero band at the top of a page; the page body follows in .container-page.
	let {
		title,
		description,
		actions,
		aside
	}: { title: string; description?: string; actions?: Snippet; aside?: Snippet } = $props();
</script>

<svelte:head>
	<title>{title} · degoploy</title>
</svelte:head>

<section class="border-b">
	<div class="container-page flex flex-wrap items-end justify-between gap-x-8 gap-y-4 py-10">
		<div class="flex min-w-0 flex-col gap-2">
			<h1 class="text-3xl font-semibold tracking-tight md:text-4xl">{title}</h1>
			{#if description}
				<p class="text-muted-foreground">{description}</p>
			{/if}
		</div>
		{#if actions || aside}
			<div class="flex flex-wrap items-center gap-2">
				{@render aside?.()}
				{@render actions?.()}
			</div>
		{/if}
	</div>
</section>
