<script lang="ts">
	import { timeAt, type LogLine } from '#lib/log.js';
	import { cn } from '#lib/utils.js';

	// One step's output: line numbers, optional write times, error lines marked.
	let {
		lines,
		first = 1,
		times,
		showTimes,
		isError,
		follow
	}: {
		lines: LogLine[];
		/** Number of the first line shown (earlier lines may be hidden). */
		first?: number;
		times: number[][];
		showTimes: boolean;
		isError: (line: LogLine) => boolean;
		/** Live output: keep the newest line in view while scrolled to the bottom. */
		follow: boolean;
	} = $props();

	const clock = new Intl.DateTimeFormat(undefined, {
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
		hour12: false
	});

	let el = $state<HTMLDivElement | null>(null);
	let atBottom = true;

	$effect.pre(() => {
		void lines;
		if (el) atBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 8;
	});
	$effect(() => {
		void lines;
		if (el && follow && atBottom) el.scrollTop = el.scrollHeight;
	});
</script>

<div
	bind:this={el}
	class="max-h-[min(70svh,40rem)] overflow-auto bg-zinc-950 pt-2.5 pb-3.5 font-mono text-[13px] text-zinc-200 dark:bg-black"
	data-testid="log-console"
>
	<div class="min-w-fit">
		{#each lines as line, i (line.offset)}
			{@const t = showTimes ? timeAt(times, line.offset) : null}
			<div
				class={cn(
					'flex min-h-6 items-start gap-4 pr-4 leading-6',
					isError(line) && 'bg-red-500/[0.12] text-red-400'
				)}
			>
				<span class="w-11 shrink-0 text-right text-xs leading-6 text-zinc-500 select-none">
					{first + i}
				</span>
				{#if showTimes}
					<span class="w-[58px] shrink-0 text-xs leading-6 text-zinc-600 select-none">
						{t === null ? '' : clock.format(t)}
					</span>
				{/if}
				<span class="whitespace-pre"
					>{#each line.spans as s, j (j)}<span class={s.cls} style={s.style}>{s.text}</span
						>{/each}</span
				>
			</div>
		{/each}
	</div>
</div>
