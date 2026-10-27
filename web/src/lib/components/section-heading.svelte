<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		title,
		description,
		actions,
		id
	}: { title: string; description?: string; actions?: Snippet; id?: string } = $props();
</script>

<div class="flex flex-wrap items-end justify-between gap-3">
	<div class="flex min-w-0 flex-col gap-1">
		<h2 class="text-lg font-semibold tracking-tight" {id}>{title}</h2>
		{#if description}<p class="text-sm text-muted-foreground">{description}</p>{/if}
	</div>
	{#if actions}
		<div class="flex flex-wrap items-center gap-2">{@render actions()}</div>
	{/if}
</div>
