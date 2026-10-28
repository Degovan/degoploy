<script lang="ts">
	import { onMount } from 'svelte';
	import { deploymentsApi, isActive, shortRef } from '#lib/api/deployments.js';
	import type { Deployment, Project } from '#lib/api/types.gen.js';
	import LogConsole from '#lib/components/log-console.svelte';
	import StageNode from '#lib/components/stage-node.svelte';
	import { LogParser, sameError, type LogLine } from '#lib/log.js';
	import { decodeChunk, realtime } from '#lib/realtime.svelte.js';
	import type { Stage, StageKey } from '#lib/stages.js';
	import { formatStage, timeAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import HashIcon from '@lucide/svelte/icons/hash';
	import ListOrderedIcon from '@lucide/svelte/icons/list-ordered';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import TimerIcon from '@lucide/svelte/icons/timer';

	// The runner's steps, each expandable to its part of the log. Mount once per
	// deployment: the parser keeps the bytes received so far.
	let {
		deployment: d,
		project: p,
		stages,
		now
	}: { deployment: Deployment; project: Project; stages: Stage[]; now: number } = $props();

	/** Steps with more lines show only the newest; the raw log has everything. */
	const MAX_LINES = 5000;

	const parser = new LogParser();
	let version = $state(0);
	let ended = $state(false);
	let error = $state('');

	// When output arrived: [offset, unix ms], from the server's record and live chunks.
	let stored: number[][] = [];
	let live: number[][] = [];
	let times = $state.raw<number[][]>([]);
	function mergeTimes() {
		const all = [...stored, ...live].sort((a, b) => a[0] - b[0]);
		times = all.filter((e, i) => i === 0 || e[0] !== all[i - 1][0]);
	}
	async function loadTimes() {
		try {
			stored = (await deploymentsApi.logTimes(d.id)).entries;
			mergeTimes();
		} catch {
			// Times are a nicety; the log reads fine without them.
		}
	}

	const TIMES_KEY = 'degoploy.log.timestamps';
	let showTimes = $state(readShowTimes());
	function readShowTimes() {
		try {
			return localStorage.getItem(TIMES_KEY) !== 'off';
		} catch {
			return true;
		}
	}
	function toggleTimes() {
		showTimes = !showTimes;
		try {
			localStorage.setItem(TIMES_KEY, showTimes ? 'on' : 'off');
		} catch {
			// Not remembered when storage is unavailable.
		}
	}

	onMount(() => {
		if (!d.started_at && !isActive(d)) return;
		loadTimes();
		const topic = `deployment:${d.id}:log`;
		return realtime.subscribe(
			topic,
			(m) => {
				if (m.type === 'log.chunk') {
					const bytes = decodeChunk(m.data);
					const start = m.offset ?? 0;
					if (start > parser.size) {
						// Missed bytes (e.g. dropped connection): ask again from what we have.
						realtime.resubscribe(topic);
						return;
					}
					const skip = parser.size - start;
					if (skip >= bytes.length) return;
					if (m.time) {
						live.push([start, m.time]);
						mergeTimes();
					}
					parser.push(bytes.subarray(skip));
					version++;
				} else if (m.type === 'log.end') {
					ended = true;
					loadTimes();
				} else if (m.type === 'error') {
					error = String(m.data ?? 'Log unavailable');
				}
			},
			{ offset: () => parser.size }
		);
	});

	// The step to open by default: the one that failed or is running, else the script.
	const focus = $derived(
		stages.find((s) => s.state === 'failed' || s.state === 'running')?.key ?? 'script'
	);
	let open = $state<Partial<Record<StageKey, boolean>>>({});
	const isOpen = (key: StageKey) => open[key] ?? key === focus;

	function linesOf(key: StageKey): { lines: LogLine[]; hidden: number } {
		void version;
		const all = parser.linesOf(key);
		const hidden = Math.max(0, all.length - MAX_LINES);
		return { lines: all.slice(hidden), hidden };
	}

	function title(s: Stage) {
		return s.key === 'trigger' ? 'Trigger' : s.key === 'update' ? 'Update repository' : s.label;
	}

	function describe(s: Stage, lines: LogLine[]): string {
		if (s.key === 'trigger') {
			if (d.trigger === 'webhook') {
				const by = d.author ? ` by ${d.author}` : '';
				return `push to ${shortRef(d.ref) || 'unknown ref'}${by}`;
			}
			return `${d.trigger} deploy requested by ${d.triggered_by || 'unknown'}`;
		}
		if (s.key === 'update') {
			if (s.state === 'skipped') return 'auto pull is off';
			const cmds = lines.filter((l) => l.text.startsWith('$ git ')).map((l) => l.text.slice(2));
			return cmds.length ? cmds.join(' && ') : `update ${p.work_dir}`;
		}
		const header = lines[0]?.text;
		return header?.startsWith('==> Running ') ? header.slice('==> Running '.length) : p.work_dir;
	}

	// When each stage started, for "Started … ago".
	function startedAt(key: StageKey): string | null {
		if (key === 'trigger') return d.queued_at;
		if (key === 'update') return d.started_at;
		return d.script_started_at;
	}

	const isError = (s: Stage) => (line: LogLine) =>
		s.state === 'failed' && !!d.error && sameError(line.text, d.error);

	const headTone: Partial<Record<Stage['state'], string>> = {
		failed: 'bg-red-500/10 text-red-600 dark:text-red-400',
		running: 'bg-amber-400/10 text-amber-700 dark:text-amber-300'
	};
	const borderTone: Partial<Record<Stage['state'], string>> = {
		failed: 'border-red-500/40',
		running: 'border-amber-400/40'
	};
</script>

<section class="flex min-w-0 flex-col gap-3" aria-labelledby="steps">
	<div class="flex items-center justify-between gap-3 pb-1">
		<h2 id="steps" class="text-base font-semibold">Steps</h2>
		<div class="flex items-center gap-1">
			<button
				type="button"
				class={cn(
					'flex h-[30px] items-center gap-1.5 rounded-md border border-transparent px-2 text-xs font-medium transition-colors hover:text-foreground',
					showTimes ? 'text-foreground' : 'text-muted-foreground'
				)}
				aria-pressed={showTimes}
				onclick={toggleTimes}
			>
				<ListOrderedIcon class="size-[15px] text-muted-foreground" />
				Timestamps
			</button>
			{#if d.started_at}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- API download -->
				<a
					href={deploymentsApi.logUrl(d.id)}
					class="flex h-[30px] items-center gap-1.5 rounded-md border bg-background px-2.5 text-xs font-medium transition-colors hover:bg-accent"
					download
				>
					<DownloadIcon class="size-[15px] text-muted-foreground" />
					Raw log
				</a>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			{/if}
		</div>
	</div>

	{#each stages as s (s.key)}
		{@const { lines, hidden } = linesOf(s.key)}
		{@const expandable = lines.length > 0}
		{@const expanded = expandable && isOpen(s.key)}
		{@const since = startedAt(s.key)}
		<div
			class={cn(
				'overflow-hidden rounded-lg border bg-card',
				expanded && borderTone[s.state],
				s.state === 'pending' && 'opacity-60'
			)}
			data-step={s.key}
			data-state={s.state}
		>
			<button
				type="button"
				class={cn(
					'flex w-full min-w-0 items-center gap-3 px-4 py-3 text-left',
					expanded && 'border-b',
					expanded && headTone[s.state],
					expandable ? 'hover:bg-accent/40' : 'cursor-default'
				)}
				aria-expanded={expandable ? expanded : undefined}
				disabled={!expandable}
				onclick={() => (open[s.key] = !expanded)}
			>
				<ChevronRightIcon
					class={cn(
						'size-[15px] shrink-0 text-muted-foreground transition-transform',
						expanded && 'rotate-90',
						expanded && headTone[s.state] && 'text-current',
						!expandable && 'invisible'
					)}
				/>
				<StageNode
					state={s.state}
					class={s.state === 'failed' || s.state === 'running' ? 'size-[22px] border-0' : ''}
				/>
				<span
					class={cn(
						'shrink-0 text-sm font-medium',
						s.key === 'script' && 'font-mono',
						!(expanded && headTone[s.state]) && 'text-foreground'
					)}
				>
					{title(s)}
				</span>
				{#if expanded}
					<span class="flex-1"></span>
					<span
						class={cn(
							'flex shrink-0 items-center gap-4 text-xs',
							!headTone[s.state] && 'text-muted-foreground'
						)}
					>
						{#if s.ms !== null}
							<span class="flex items-center gap-1.5">
								<TimerIcon class="size-[13px]" />{formatStage(s.ms)}
							</span>
						{/if}
						{#if since}
							<span class="hidden items-center gap-1.5 sm:flex">
								<CalendarIcon class="size-[13px]" />Started {timeAgo(since, now)}
							</span>
						{/if}
						{#if s.key === 'script' && s.state === 'failed' && d.exit_code !== null}
							<span class="flex items-center gap-1.5">
								<HashIcon class="size-[13px]" />exit {d.exit_code}
							</span>
						{/if}
					</span>
				{:else}
					<span class="min-w-0 flex-1 truncate font-mono text-[13px] text-muted-foreground">
						{describe(s, lines)}
					</span>
					<span class="shrink-0 font-mono text-[13px] text-muted-foreground tabular-nums">
						{s.ms === null ? '' : formatStage(s.ms)}
					</span>
				{/if}
			</button>
			{#if expanded}
				{#if hidden}
					<div class="border-b bg-zinc-950 px-4 py-2 text-xs text-zinc-400 dark:bg-black">
						{hidden.toLocaleString()} earlier lines are not shown. Download the raw log for the full output.
					</div>
				{/if}
				<LogConsole
					{lines}
					first={hidden + 1}
					{times}
					{showTimes}
					isError={isError(s)}
					follow={isActive(d) && !ended}
				/>
			{/if}
		</div>
	{/each}

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{:else if isActive(d) && !ended && version === 0}
		<p class="flex items-center gap-2 text-sm text-muted-foreground">
			<LoaderIcon class="size-4 animate-spin" />
			{d.status === 'queued' ? 'Waiting in the queue…' : 'Waiting for output…'}
		</p>
	{/if}
</section>
