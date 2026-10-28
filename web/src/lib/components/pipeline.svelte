<script lang="ts">
	import type { Stage } from '#lib/stages.js';
	import StageNode from '#lib/components/stage-node.svelte';
	import { formatStage } from '#lib/time.js';
	import { cn } from '#lib/utils.js';

	// Start → stages → End, as a horizontal track. End is reached once every stage is done.
	let {
		stages,
		finished,
		class: className
	}: { stages: Stage[]; finished: boolean; class?: string } = $props();

	const timeTone = (s: Stage) =>
		s.state === 'failed'
			? 'text-red-600 dark:text-red-400'
			: s.state === 'running'
				? 'text-amber-600 dark:text-amber-300'
				: 'text-muted-foreground';
</script>

<!-- Each item is label (16px), node (28px), time (16px); links sit on the node's center line. -->
{#snippet link()}
	<li class="mx-1.5 mt-[37.5px] h-px min-w-3 flex-1 bg-border" aria-hidden="true"></li>
{/snippet}

{#snippet edge(label: string, done: boolean)}
	<li class="flex shrink-0 flex-col items-center gap-2">
		<span class="text-xs leading-4 text-muted-foreground">{label}</span>
		<span class="flex size-7 items-center justify-center">
			<span
				class={cn(
					'size-2.5 rounded-full border',
					done ? 'border-foreground/70' : 'border-muted-foreground'
				)}
			></span>
		</span>
		<span class="h-4"></span>
	</li>
{/snippet}

<ol class={cn('flex items-start', className)}>
	{@render edge('Start', false)}
	{#each stages as s (s.key)}
		{@render link()}
		<li class="flex max-w-40 min-w-0 flex-col items-center gap-2">
			<span class="max-w-full truncate text-xs leading-4 font-medium" title={s.label}
				>{s.label}</span
			>
			<span class="flex size-7 items-center justify-center"><StageNode state={s.state} /></span>
			<span class={cn('font-mono text-xs leading-4 tabular-nums', timeTone(s))}>
				{s.ms === null ? (s.state === 'skipped' ? 'skipped' : ' ') : formatStage(s.ms)}
			</span>
		</li>
	{/each}
	{@render link()}
	{@render edge('End', finished)}
</ol>
