<script lang="ts" module>
	import type { DeployStatus } from '#lib/api/types.gen.js';

	export const statusLabel: Record<DeployStatus, string> = {
		queued: 'Queued',
		running: 'Running',
		success: 'Success',
		failed: 'Failed',
		canceled: 'Canceled',
		timeout: 'Timed out',
		skipped: 'Skipped'
	};
</script>

<script lang="ts">
	import { cn } from '#lib/utils.js';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import CircleXIcon from '@lucide/svelte/icons/circle-x';
	import CircleSlashIcon from '@lucide/svelte/icons/circle-slash';
	import ClockIcon from '@lucide/svelte/icons/clock';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import SkipForwardIcon from '@lucide/svelte/icons/skip-forward';
	import TimerOffIcon from '@lucide/svelte/icons/timer-off';

	let { status, class: className }: { status: DeployStatus; class?: string } = $props();

	const styles: Record<DeployStatus, string> = {
		queued: 'bg-muted text-muted-foreground',
		running: 'bg-sky-500/15 text-sky-700 dark:text-sky-300',
		success: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
		failed: 'bg-red-500/15 text-red-700 dark:text-red-300',
		canceled: 'bg-amber-500/15 text-amber-700 dark:text-amber-300',
		timeout: 'bg-orange-500/15 text-orange-700 dark:text-orange-300',
		skipped: 'bg-muted text-muted-foreground'
	};
</script>

<span
	class={cn(
		'inline-flex h-5 shrink-0 items-center gap-1 rounded-full px-2 text-xs font-medium whitespace-nowrap',
		styles[status],
		className
	)}
	data-status={status}
>
	{#if status === 'running'}
		<LoaderIcon class="size-3 animate-spin" />
	{:else if status === 'queued'}
		<ClockIcon class="size-3" />
	{:else if status === 'success'}
		<CircleCheckIcon class="size-3" />
	{:else if status === 'failed'}
		<CircleXIcon class="size-3" />
	{:else if status === 'canceled'}
		<CircleSlashIcon class="size-3" />
	{:else if status === 'timeout'}
		<TimerOffIcon class="size-3" />
	{:else}
		<SkipForwardIcon class="size-3" />
	{/if}
	{statusLabel[status]}
</span>
