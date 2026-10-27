<script lang="ts" module>
	import type { DeployStatus } from '#lib/api/types.gen.js';

	export const statusDot: Record<DeployStatus, string> = {
		queued: 'bg-sky-400',
		running: 'bg-amber-400',
		success: 'bg-emerald-400',
		failed: 'bg-red-500',
		timeout: 'bg-red-500',
		canceled: 'bg-zinc-400',
		skipped: 'bg-zinc-500'
	};
</script>

<script lang="ts">
	import { statusLabel } from '#lib/components/deploy-status-badge.svelte';
	import { cn } from '#lib/utils.js';

	let {
		status,
		label = true,
		class: className
	}: { status: DeployStatus; label?: boolean; class?: string } = $props();
</script>

<span class={cn('inline-flex items-center gap-2', className)} data-status={status}>
	<span class="relative flex size-2 shrink-0">
		{#if status === 'running'}
			<span class="absolute inset-0 animate-ping rounded-full bg-amber-400/70"></span>
		{/if}
		<span class={cn('relative size-2 rounded-full', statusDot[status])}></span>
	</span>
	{#if label}<span>{statusLabel[status]}</span>{/if}
</span>
