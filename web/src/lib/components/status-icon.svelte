<script lang="ts">
	import type { DeployStatus } from '#lib/api/types.gen.js';
	import { statusLabel } from '#lib/components/deploy-status-badge.svelte';
	import { cn } from '#lib/utils.js';
	import CheckIcon from '@lucide/svelte/icons/check';
	import XIcon from '@lucide/svelte/icons/x';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import ClockIcon from '@lucide/svelte/icons/clock';
	import MinusIcon from '@lucide/svelte/icons/minus';
	import SlashIcon from '@lucide/svelte/icons/slash';

	let { status, class: className }: { status: DeployStatus; class?: string } = $props();

	const tone: Record<DeployStatus, string> = {
		success: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
		failed: 'bg-red-500/15 text-red-600 dark:text-red-400',
		timeout: 'bg-red-500/15 text-red-600 dark:text-red-400',
		running: 'bg-amber-500/15 text-amber-600 dark:text-amber-300',
		queued: 'bg-sky-500/15 text-sky-600 dark:text-sky-300',
		canceled: 'bg-muted text-muted-foreground',
		skipped: 'bg-muted text-muted-foreground'
	};
</script>

<span
	class={cn(
		'inline-flex size-6 shrink-0 items-center justify-center rounded-full [&_svg]:size-3.5',
		tone[status],
		className
	)}
	title={statusLabel[status]}
>
	{#if status === 'success'}
		<CheckIcon />
	{:else if status === 'failed' || status === 'timeout'}
		<XIcon />
	{:else if status === 'running'}
		<LoaderIcon class="animate-spin" />
	{:else if status === 'queued'}
		<ClockIcon />
	{:else if status === 'canceled'}
		<SlashIcon />
	{:else}
		<MinusIcon />
	{/if}
	<span class="sr-only">{statusLabel[status]}</span>
</span>
