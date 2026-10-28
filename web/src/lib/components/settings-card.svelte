<script lang="ts">
	import type { Component, Snippet } from 'svelte';
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
		action,
		icon: Icon,
		titleAside,
		aside,
		extra
	}: {
		title: string;
		description?: string | Snippet;
		note: string | Snippet;
		danger?: boolean;
		dirty?: boolean;
		saving?: boolean;
		/** Error that is not about a single field. */
		error?: string;
		onsave?: () => void;
		children?: Snippet;
		/** Replaces the Save button (e.g. a destructive action). */
		action?: Snippet;
		/** Icon box left of the title; the body then lines up under the text. */
		icon?: Component<{ class?: string }>;
		/** Next to the title, e.g. a status. */
		titleAside?: Snippet;
		/** Top right of the card, e.g. an on/off switch. */
		aside?: Snippet;
		/** Before the Save button, e.g. a secondary action. */
		extra?: Snippet;
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
	<div class={cn('flex flex-col p-6', Icon ? 'gap-5' : 'gap-4')}>
		<div class="flex items-start gap-4">
			{#if Icon}
				<span
					class="flex size-10 shrink-0 items-center justify-center rounded-lg border border-input bg-muted"
				>
					<Icon class="size-[18px]" />
				</span>
			{/if}
			<div class="flex min-w-0 flex-1 flex-col gap-2" class:gap-1.5={Icon}>
				<div class="flex flex-wrap items-center gap-x-3 gap-y-1">
					<h3 class="text-xl font-semibold tracking-[-0.4px]">{title}</h3>
					{@render titleAside?.()}
				</div>
				{#if typeof description === 'string'}
					<p class="text-sm/normal text-muted-foreground">{description}</p>
				{:else if description}
					<p class="text-sm/normal text-muted-foreground">{@render description()}</p>
				{/if}
			</div>
			{@render aside?.()}
		</div>
		{#if children || error}
			<div class={cn('flex min-w-0 flex-col gap-4', Icon && 'sm:pl-14')}>
				{@render children?.()}
				{#if error}<p class="text-[13px] text-destructive" role="alert">{error}</p>{/if}
			</div>
		{/if}
	</div>
	<div
		class={cn(
			'flex items-center justify-between gap-3 border-t px-6 py-3',
			danger ? 'border-red-500/25 bg-red-500/[0.08]' : 'bg-background'
		)}
	>
		<p class="text-[13px] text-muted-foreground">
			{#if typeof note === 'string'}{note}{:else}{@render note()}{/if}
		</p>
		{#if action}
			{@render action()}
		{:else if onsave || extra}
			<div class="flex shrink-0 items-center gap-2">
				{@render extra?.()}
				{#if onsave}
					<Button type="submit" class="h-8 shrink-0 px-3.5 text-[13px]" disabled={!dirty || saving}>
						{#if saving}<LoaderIcon class="animate-spin" />{/if}
						Save
					</Button>
				{/if}
			</div>
		{/if}
	</div>
</form>
