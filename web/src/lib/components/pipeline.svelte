<script lang="ts">
	import type { Stage } from '#lib/stages.js';
	import StageNode from '#lib/components/stage-node.svelte';
	import { formatStage } from '#lib/time.js';
	import { cn } from '#lib/utils.js';

	// Start → stages → End, as a horizontal track. End is reached once every stage is done.
	let { stages, finished }: { stages: Stage[]; finished: boolean } = $props();

	const timeTone = (s: Stage) =>
		s.state === 'failed'
			? 'text-red-600 dark:text-red-400'
			: s.state === 'running'
				? 'text-amber-600 dark:text-amber-300'
				: 'text-muted-foreground';
</script>

<ol class="relative grid grid-cols-[auto_repeat(3,minmax(0,1fr))_auto] items-start gap-2">
	<!-- The connecting line runs through the middle of the nodes. -->
	<span class="absolute top-[2.375rem] right-3 left-3 h-px bg-border" aria-hidden="true"></span>
	<li class="flex flex-col items-center gap-2.5">
		<span class="text-xs text-muted-foreground">Start</span>
		<span class="relative z-10 flex size-6 items-center justify-center">
			<span class="size-3 rounded-full border-2 border-muted-foreground/60 bg-card"></span>
		</span>
	</li>
	{#each stages as s (s.key)}
		<li class="flex min-w-0 flex-col items-center gap-2.5">
			<span class="max-w-full truncate text-xs font-medium" title={s.label}>{s.label}</span>
			<StageNode state={s.state} />
			<span class={cn('font-mono text-xs tabular-nums', timeTone(s))}>
				{s.ms === null ? (s.state === 'skipped' ? 'skipped' : ' ') : formatStage(s.ms)}
			</span>
		</li>
	{/each}
	<li class="flex flex-col items-center gap-2.5">
		<span class="text-xs text-muted-foreground">End</span>
		<span class="relative z-10 flex size-6 items-center justify-center">
			<span
				class={cn(
					'size-3 rounded-full border-2 bg-card',
					finished ? 'border-foreground/70' : 'border-muted-foreground/60'
				)}
			></span>
		</span>
	</li>
</ol>
