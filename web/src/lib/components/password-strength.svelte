<script lang="ts">
	import { passwordStrength } from '#lib/password.js';
	import { cn } from '#lib/utils.js';

	let { password }: { password: string } = $props();

	const strength = $derived(passwordStrength(password));
	const color = $derived(
		['', 'bg-red-500', 'bg-amber-400', 'bg-emerald-400', 'bg-emerald-400'][strength.score]
	);
</script>

<div class="flex items-center gap-2.5" aria-live="polite">
	<div class="flex w-40 shrink-0 gap-1" aria-hidden="true">
		{#each [1, 2, 3, 4] as i (i)}
			<span class={cn('h-1 flex-1 rounded-full', i <= strength.score ? color : 'bg-accent')}></span>
		{/each}
	</div>
	<span class="text-xs text-muted-foreground/80">
		{#if password}
			{strength.label} · {password.length} {password.length === 1 ? 'character' : 'characters'}
		{:else}
			At least 10 characters
		{/if}
	</span>
</div>
