<script lang="ts">
	import type { StageState } from '#lib/stages.js';
	import { cn } from '#lib/utils.js';
	import CheckIcon from '@lucide/svelte/icons/check';
	import XIcon from '@lucide/svelte/icons/x';
	import MinusIcon from '@lucide/svelte/icons/minus';
	import SlashIcon from '@lucide/svelte/icons/slash';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';

	let { state, class: className }: { state: StageState; class?: string } = $props();

	const tone: Record<StageState, string> = {
		success: 'border-emerald-500/60 bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
		failed: 'border-red-500/70 bg-red-500/15 text-red-600 dark:text-red-400',
		running: 'border-amber-400/70 bg-amber-400/15 text-amber-600 dark:text-amber-300',
		canceled: 'border-zinc-500/50 bg-muted text-muted-foreground',
		skipped: 'border-dashed border-zinc-500/50 text-muted-foreground',
		pending: 'border-zinc-500/50 text-muted-foreground'
	};
</script>

<span
	class={cn(
		'relative z-10 flex size-6 items-center justify-center rounded-full border bg-card [&_svg]:size-3.5',
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
