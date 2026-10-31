<script lang="ts">
	import { resolve } from '$app/paths';
	import { lineOf, runtimeExamples } from '#lib/api/runtimes.js';
	import type { RuntimeStatus } from '#lib/api/types.gen.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { cn } from '#lib/utils.js';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';

	// One runtime's setting in a project: Off, Auto, or one of the installed
	// versions. Installing happens on the Runtimes page only.
	let {
		name,
		title,
		value = $bindable(),
		used,
		missing = false,
		status,
		error = '',
		canManage
	}: {
		name: string;
		title: string;
		value: string;
		/** The version the latest deployment ran with. */
		used: string;
		/** value is a saved version that no installed one fits. */
		missing?: boolean;
		/** The runtime on this server, when the viewer may see it. */
		status?: RuntimeStatus;
		error?: string;
		canManage: boolean;
	} = $props();

	const ex = $derived(runtimeExamples[name] ?? { line: '1', exact: '1.0.0', files: [] });
	// Quick picks: the lines of the installed versions, newest first, each with
	// the version it takes now.
	const installed = $derived(status?.installed ?? []);
	const picks = $derived.by(() => {
		const out: { spec: string; version: string }[] = [];
		const lts = installed.find((v) => v.lts);
		if (name === 'node' && lts) out.push({ spec: 'lts', version: lts.version });
		for (const v of installed) {
			const line = lineOf(name, v.version);
			if (!out.some((p) => p.spec === line)) out.push({ spec: line, version: v.version });
		}
		return out;
	});
	const runtimesPage = $derived(resolve('/(app)/runtimes/[name]', { name }));

	type Mode = 'off' | 'auto' | 'version';
	const mode = $derived<Mode>(value === 'auto' ? 'auto' : value === 'off' ? 'off' : 'version');
	let lastSpec = '';
	function setMode(m: Mode) {
		if (m === mode) return;
		if (mode === 'version') lastSpec = value;
		value = m === 'version' ? lastSpec || picks[0]?.spec || '' : m;
	}
	const modes: { value: Mode; label: string }[] = [
		{ value: 'off', label: 'Off' },
		{ value: 'auto', label: 'Auto' },
		{ value: 'version', label: 'Version' }
	];
	let buttons = $state<HTMLButtonElement[]>([]);
	function onKeydown(e: KeyboardEvent) {
		const step = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: 1, ArrowUp: -1 }[e.key];
		if (!step) return;
		e.preventDefault();
		const i = modes.findIndex((m) => m.value === mode);
		const next = (i + step + modes.length) % modes.length;
		setMode(modes[next]!.value);
		buttons[next]?.focus();
	}

	const files = $derived(ex.files);
	const hint = 'text-[13px] text-muted-foreground/80';
</script>

{#snippet runtimesLink(label: string)}
	{#if canManage}
		<a
			href={runtimesPage}
			class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border border-input bg-background px-3 text-[13px] font-medium transition-colors hover:bg-accent"
		>
			{label}
			<ArrowRightIcon class="size-3.5" />
		</a>
	{:else}
		<span class="text-xs text-muted-foreground/80"
			>An admin can install it on the Runtimes page.</span
		>
	{/if}
{/snippet}

<div class="flex flex-col gap-3">
	<div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
		<div class="flex flex-wrap items-center gap-2">
			<span id={`rt-${name}`} class="text-sm font-medium">{title}</span>
			{#if status && !status.enabled}
				<span
					class="inline-flex h-[22px] items-center rounded-[5px] border border-input bg-muted px-[7px] text-xs text-muted-foreground"
					title="Turned off for every project on the Runtimes page">Off on this server</span
				>
			{/if}
			{#if used}
				<span class="text-xs text-muted-foreground/80"
					>last deployment: <span class="font-mono">{used}</span></span
				>
			{/if}
			{#if canManage}
				<a
					href={runtimesPage}
					class="text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
					>Manage versions</a
				>
			{/if}
		</div>
		<div
			class="grid h-9 w-full grid-cols-3 gap-0.5 rounded-md border border-input p-[3px] sm:w-[260px]"
			role="radiogroup"
			aria-labelledby={`rt-${name}`}
		>
			{#each modes as m, i (m.value)}
				{@const on = mode === m.value}
				<button
					bind:this={buttons[i]}
					type="button"
					role="radio"
					aria-checked={on}
					tabindex={on ? 0 : -1}
					class={cn(
						'rounded-sm text-[13px] font-medium transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
						on ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground'
					)}
					onclick={() => setMode(m.value)}
					onkeydown={onKeydown}>{m.label}</button
				>
			{/each}
		</div>
	</div>

	{#if mode === 'auto'}
		<p class={hint}>
			On when the repository has
			{#each files as f, i (f)}<span class="font-mono">{f}</span>{i < files.length - 2
					? ', '
					: i === files.length - 2
						? ' or '
						: ''}{/each}, with the version it names.
			{#if status?.default}
				If it names none: the default, {title}
				{status.default}.
			{:else}
				If it names none: the server's own {name}.
			{/if}
			That version must be installed on the Runtimes page: deployments download nothing.
		</p>
	{:else if mode === 'version'}
		{#if status && installed.length === 0}
			<div
				class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-md border border-dashed px-3.5 py-3"
			>
				<span class="text-[13px] text-muted-foreground"
					>No {title} version is installed on this server yet.</span
				>
				{@render runtimesLink(`Install ${title} on the Runtimes page`)}
			</div>
		{:else}
			<div class="flex flex-col gap-2">
				<Label for={`set-rt-${name}`} class="sr-only">{title} version</Label>
				<div class="flex flex-wrap items-center gap-2">
					<Input
						id={`set-rt-${name}`}
						bind:value
						spellcheck={false}
						autocomplete="off"
						placeholder={picks[0]?.spec ?? ex.line}
						class="h-9 w-[180px] bg-background px-3 font-mono text-[13px] md:text-[13px] dark:bg-background"
						aria-invalid={!!error || missing}
					/>
					{#each picks as p (p.spec)}
						<button
							type="button"
							title={`Takes ${title} ${p.version}`}
							class={cn(
								'h-7 rounded-md border px-2.5 font-mono text-xs transition-colors',
								value === p.spec
									? 'border-muted-foreground bg-muted text-foreground'
									: 'border-input text-muted-foreground hover:text-foreground'
							)}
							onclick={() => (value = p.spec)}>{p.spec}</button
						>
					{/each}
				</div>
				<p class={hint}>
					Always on, whatever the repository says, among the versions installed on this server. A
					line such as {ex.line} takes its newest installed release; {ex.exact} is exact.
					{#if canManage}Need another version?
						<a href={runtimesPage} class="text-foreground underline-offset-2 hover:underline"
							>Install it on the Runtimes page</a
						>.{/if}
				</p>
			</div>
		{/if}
		{#if missing}
			<div
				class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-md border border-amber-500/30 bg-amber-500/[0.07] px-3.5 py-3"
				role="alert"
			>
				<span class="flex items-start gap-2 text-[13px] text-amber-600 dark:text-amber-300">
					<TriangleAlertIcon class="mt-0.5 size-3.5 shrink-0" />
					{title}
					{value} is not installed: deployments fail until it is.
				</span>
				{@render runtimesLink('Install it')}
			</div>
		{/if}
	{:else}
		<p class={hint}>
			degoploy leaves {title} out: scripts use the server's own {name}, if there is one.
		</p>
	{/if}
	{#if error}<p class="text-[13px] text-destructive">{error}</p>{/if}
</div>
