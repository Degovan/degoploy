<script lang="ts">
	import type { StageState } from '#lib/stages.js';
	import { cn } from '#lib/utils.js';
	import CheckIcon from '@lucide/svelte/icons/check';
	import XIcon from '@lucide/svelte/icons/x';
	import MinusIcon from '@lucide/svelte/icons/minus';
	import SlashIcon from '@lucide/svelte/icons/slash';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';

	let { state, class: className }: { state: StageState; class?: string } = $props();

	// The stage that needs attention (failed or running) is drawn larger, with a ring.
	const tone: Record<StageState, string> = {
		success: 'size-[22px] bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
		failed:
			'size-7 border border-red-500 bg-red-500/15 text-red-600 dark:border-red-400 dark:text-red-400',
		running:
			'size-7 border border-amber-500 bg-amber-400/15 text-amber-600 dark:border-amber-400 dark:text-amber-300',
		canceled: 'size-[22px] bg-muted text-muted-foreground',
		skipped: 'size-[22px] border border-dashed border-muted-foreground/50 text-muted-foreground',
		pending: 'size-[22px] border border-muted-foreground/40 text-muted-foreground'
	};
</script>

<span
	class={cn(
		'flex shrink-0 items-center justify-center rounded-full [&_svg]:size-[13px]',
		tone[state],
		className
	)}
>
	{#if state === 'success'}
		<CheckIcon />
	{:else if state === 'failed'}
		<XIcon />
	{:else if state === 'running'}
		<LoaderIcon class="animate-spin" />
	{:else if state === 'canceled'}
		<SlashIcon />
	{:else if state === 'skipped'}
		<MinusIcon />
	{/if}
	<span class="sr-only">{state}</span>
</span>
