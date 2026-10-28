<script lang="ts">
	import type { TriggerEvent } from '#lib/api/types.gen.js';
	import { cn } from '#lib/utils.js';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import TagIcon from '@lucide/svelte/icons/tag';

	// Segmented Branch / Tag choice, behaving as a radio group.
	let {
		value = $bindable('push'),
		labelledby,
		label,
		class: className
	}: {
		value?: TriggerEvent;
		labelledby?: string;
		label?: string;
		class?: string;
	} = $props();

	const kinds = [
		{ value: 'push', label: 'Branch', icon: GitBranchIcon },
		{ value: 'tag', label: 'Tag', icon: TagIcon }
	] as const;

	let buttons = $state<HTMLButtonElement[]>([]);

	function onkeydown(e: KeyboardEvent) {
		if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key)) return;
		e.preventDefault();
		const i = kinds.findIndex((k) => k.value !== value);
		value = kinds[i].value;
		buttons[i]?.focus();
	}
</script>

<div
	class={cn('grid grid-cols-2 gap-0.5 rounded-md border border-input p-[3px]', className)}
	role="radiogroup"
	aria-labelledby={labelledby}
	aria-label={label}
>
	{#each kinds as k, i (k.value)}
		{@const active = value === k.value}
		<button
			bind:this={buttons[i]}
			type="button"
			role="radio"
			aria-checked={active}
			tabindex={active ? 0 : -1}
			class={cn(
				'flex items-center justify-center gap-1.5 rounded-sm text-[13px] font-medium transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50 [&_svg]:size-3.5',
				active
					? 'bg-accent text-foreground'
					: 'text-muted-foreground hover:text-foreground [&_svg]:text-muted-foreground/80'
			)}
			onclick={() => (value = k.value)}
			{onkeydown}
		>
			<k.icon />
			{k.label}
		</button>
	{/each}
</div>
