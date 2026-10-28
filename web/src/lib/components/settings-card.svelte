<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { cn } from '#lib/utils.js';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';

	// One setting: title, description, the control, and a footer with a note
	// and (when onsave is set) its own Save button.
	let {
		title,
		description,
		note,
		danger = false,
		dirty = false,
		saving = false,
		error = '',
		onsave,
		children,
		action
	}: {
		title: string;
		description?: string | Snippet;
		note: string;
		danger?: boolean;
		dirty?: boolean;
		saving?: boolean;
		/** Error that is not about a single field. */
		error?: string;
		onsave?: () => void;
		children?: Snippet;
		/** Replaces the Save button (e.g. a destructive action). */
		action?: Snippet;
	} = $props();

	function submit(e: SubmitEvent) {
		e.preventDefault();
		if (dirty && !saving) onsave?.();
	}
</script>

<form
	class={cn(
		'overflow-hidden rounded-xl border bg-card',
		danger && 'border-red-500/35 dark:border-[#E5484D]/35'
	)}
	onsubmit={submit}
	aria-label={title}
	novalidate
>
	<div class="flex flex-col gap-4 p-6">
		<div class="flex flex-col gap-2">
			<h3 class="text-xl font-semibold tracking-[-0.4px]">{title}</h3>
			{#if typeof description === 'string'}
				<p class="text-sm/normal text-muted-foreground">{description}</p>
			{:else if description}
				<p class="text-sm/normal text-muted-foreground">{@render description()}</p>
			{/if}
		</div>
		{@render children?.()}
		{#if error}<p class="text-[13px] text-destructive" role="alert">{error}</p>{/if}
	</div>
	<div
		class={cn(
			'flex items-center justify-between gap-3 border-t px-6 py-3',
			danger ? 'border-red-500/25 bg-red-500/[0.08]' : 'bg-background'
		)}
	>
		<p class="text-[13px] text-muted-foreground">{note}</p>
		{#if action}
			{@render action()}
		{:else if onsave}
			<Button type="submit" class="h-8 shrink-0 px-3.5 text-[13px]" disabled={!dirty || saving}>
				{#if saving}<LoaderIcon class="animate-spin" />{/if}
				Save
			</Button>
		{/if}
	</div>
</form>
