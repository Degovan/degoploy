<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '#lib/utils.js';

	let {
		title,
		description,
		actions,
		id,
		size = 'default'
	}: {
		title: string;
		description?: string;
		actions?: Snippet;
		id?: string;
		size?: 'default' | 'sm';
	} = $props();
</script>

<div class="flex flex-wrap items-end justify-between gap-3">
	<div class="flex min-w-0 flex-col gap-1">
		<h2 class={cn('font-semibold', size === 'sm' ? 'text-base' : 'text-lg tracking-tight')} {id}>
			{title}
		</h2>
		{#if description}<p class="text-sm text-muted-foreground">{description}</p>{/if}
	</div>
	{#if actions}
		<div class="flex flex-wrap items-center gap-2">{@render actions()}</div>
	{/if}
</div>
