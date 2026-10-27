<script lang="ts" module>
	// Deterministic gradient per project, so it is recognizable across pages.
	const gradients = [
		['#fb923c', '#ec4899'],
		['#60a5fa', '#a78bfa'],
		['#2dd4bf', '#3b82f6'],
		['#fbbf24', '#f43f5e'],
		['#34d399', '#06b6d4'],
		['#c084fc', '#f472b6'],
		['#a3e635', '#14b8a6'],
		['#38bdf8', '#818cf8']
	] as const;

	export function gradientFor(seed: string): string {
		let h = 0;
		for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) | 0;
		const [a, b] = gradients[Math.abs(h) % gradients.length]!;
		return `linear-gradient(135deg, ${a}, ${b})`;
	}
</script>

<script lang="ts">
	import { cn } from '#lib/utils.js';

	let {
		seed,
		muted = false,
		class: className
	}: { seed: string; muted?: boolean; class?: string } = $props();
</script>

<span
	aria-hidden="true"
	class={cn('inline-block size-8 shrink-0 rounded-full', muted && 'opacity-60', className)}
	style:background={muted ? 'linear-gradient(135deg, #52525b, #27272a)' : gradientFor(seed)}
></span>
