<script lang="ts" module>
	import type { ProjectHealth } from '#lib/api/types.gen.js';

	export const healthPercent = (h: ProjectHealth) => Math.round((h.passed / h.total) * 100);

	export const healthSummary = (h: ProjectHealth) =>
		`${h.passed} of the last ${h.total} deployment${h.total === 1 ? '' : 's'} passed`;
</script>

<script lang="ts">
	import { cn } from '#lib/utils.js';
	import SunIcon from '@lucide/svelte/icons/sun';
	import CloudSunIcon from '@lucide/svelte/icons/cloud-sun';
	import CloudIcon from '@lucide/svelte/icons/cloud';
	import CloudRainIcon from '@lucide/svelte/icons/cloud-rain';
	import CloudLightningIcon from '@lucide/svelte/icons/cloud-lightning';

	// Weather report as in Jenkins: sunny when nearly every recent run passed.
	let { health, class: className }: { health: ProjectHealth; class?: string } = $props();

	const pct = $derived(healthPercent(health));
	const Icon = $derived(
		pct > 80
			? SunIcon
			: pct > 60
				? CloudSunIcon
				: pct > 40
					? CloudIcon
					: pct > 20
						? CloudRainIcon
						: CloudLightningIcon
	);
	const tone = $derived(
		pct > 80 ? 'text-foreground' : pct > 40 ? 'text-amber-500 dark:text-amber-300' : 'text-red-500'
	);
</script>

<Icon class={cn('size-4', tone, className)} aria-hidden="true" />
