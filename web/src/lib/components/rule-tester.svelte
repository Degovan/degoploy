<script lang="ts">
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import { rulesApi } from '#lib/api/rules.js';
	import type { RuleTestResponse, TriggerEvent } from '#lib/api/types.gen.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import { cn } from '#lib/utils.js';
	import CheckIcon from '@lucide/svelte/icons/check';
	import ChevronsRightIcon from '@lucide/svelte/icons/chevrons-right';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import PlayIcon from '@lucide/svelte/icons/play';
	import TagIcon from '@lucide/svelte/icons/tag';

	// Dry-runs the project's saved rules against a push described in the form.
	let { projectId }: { projectId: number } = $props();

	let event = $state<TriggerEvent>('push');
	let ref = $state('main');
	let message = $state('');
	let files = $state('');
	let result = $state<RuleTestResponse | null>(null);
	let error = $state('');
	let running = $state(false);

	const kinds = [
		{ value: 'push', label: 'Branch', icon: GitBranchIcon },
		{ value: 'tag', label: 'Tag', icon: TagIcon }
	] as const;

	async function run(e: SubmitEvent) {
		e.preventDefault();
		running = true;
		error = '';
		try {
			result = await rulesApi.test(projectId, {
				event,
				ref,
				message,
				files: files.split('\n')
			});
		} catch (err) {
			result = null;
			error = err instanceof ApiError ? err.message : errorMessage(err);
		} finally {
			running = false;
		}
	}
</script>

<form
	class="flex flex-col overflow-hidden rounded-xl border bg-card"
	onsubmit={run}
	aria-labelledby="rule-tester"
>
	<div class="flex flex-col gap-[18px] p-6">
		<div class="flex flex-col gap-1.5">
			<h2 id="rule-tester" class="text-base font-semibold">Test a Push</h2>
			<p class="text-sm/normal text-muted-foreground">
				Dry-run the rules against a push. Nothing is deployed.
			</p>
		</div>

		<div
			class="grid h-9 grid-cols-2 gap-0.5 rounded-md border border-input p-[3px]"
			role="radiogroup"
			aria-label="Push type"
		>
			{#each kinds as k (k.value)}
				{@const active = event === k.value}
				<button
					type="button"
					role="radio"
					aria-checked={active}
					class={cn(
						'flex items-center justify-center gap-1.5 rounded-sm text-[13px] font-medium transition-colors [&_svg]:size-3.5',
						active
							? 'bg-accent text-foreground'
							: 'text-muted-foreground hover:text-foreground [&_svg]:text-muted-foreground/80'
					)}
					onclick={() => (event = k.value)}
				>
					<k.icon />
					{k.label}
				</button>
			{/each}
		</div>

		<div class="flex flex-col gap-2">
			<Label for="sim-ref" class="text-[13px]">{event === 'tag' ? 'Tag' : 'Branch'}</Label>
			<Input
				id="sim-ref"
				required
				bind:value={ref}
				placeholder={event === 'tag' ? 'v1.2.0' : 'main'}
				class="h-10 font-mono text-[13px] md:text-[13px]"
			/>
		</div>
		<div class="flex flex-col gap-2">
			<Label for="sim-msg" class="text-[13px]">Commit message</Label>
			<Input id="sim-msg" bind:value={message} placeholder="fix: checkout bug" class="h-10" />
		</div>
		<div class="flex flex-col gap-2">
			<Label for="sim-files" class="text-[13px]">Changed files</Label>
			<Textarea
				id="sim-files"
				bind:value={files}
				rows={2}
				class="min-h-[72px] px-3 py-2.5 font-mono text-[13px] leading-[1.6] md:text-[13px]"
				placeholder="src/app.go&#10;README.md"
			/>
			<p class="text-xs text-muted-foreground/80">One path per line. Leave empty if unknown.</p>
		</div>

		{#if error}
			<p class="text-sm text-destructive" role="alert">{error}</p>
		{:else if result}
			{@const deploys = result.matched_rule_id !== null}
			<div class="overflow-hidden rounded-lg border bg-background" data-testid="sim-results">
				<div class="flex items-center justify-between border-b px-3.5 py-2.5 text-xs">
					<span class="font-medium text-muted-foreground">Result</span>
					<span class="flex items-center gap-2">
						<span
							class={cn('size-2 rounded-full', deploys ? 'bg-emerald-400' : 'bg-muted-foreground')}
						></span>
						{deploys ? 'Would deploy' : 'Would not deploy'}
					</span>
				</div>
				{#each result.results as res, i (res.rule_id)}
					<div class="flex gap-2.5 px-3.5 py-2.5">
						<span
							class={cn(
								'mt-px flex size-[18px] shrink-0 items-center justify-center rounded-full [&_svg]:size-[11px]',
								res.matched
									? 'bg-emerald-400/15 text-emerald-600 dark:text-emerald-400'
									: 'bg-foreground/[0.08] text-muted-foreground'
							)}
						>
							{#if res.matched}<CheckIcon />{:else}<ChevronsRightIcon />{/if}
						</span>
						<div class="flex min-w-0 flex-col gap-0.5">
							<span class="text-[13px] font-medium">#{i + 1} {res.name}</span>
							<span class="text-xs break-words text-muted-foreground/80">{res.reason}</span>
						</div>
					</div>
				{:else}
					<p class="px-3.5 py-2.5 text-xs text-muted-foreground">No rules configured.</p>
				{/each}
			</div>
		{/if}
	</div>

	<div class="flex items-center justify-between gap-4 border-t bg-background px-6 py-3">
		<span class="text-[13px] text-muted-foreground/80">Uses the saved rules</span>
		<Button type="submit" variant="outline" class="h-8 px-3 text-[13px]" disabled={running}>
			{#if running}<LoaderIcon class="animate-spin" />{:else}<PlayIcon />{/if}
			Run test
		</Button>
	</div>
</form>
