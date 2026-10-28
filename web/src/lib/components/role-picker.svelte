<script lang="ts" module>
	import type { Role } from '#lib/api/types.gen.js';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import RocketIcon from '@lucide/svelte/icons/rocket';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';

	export const roles = [
		{
			value: 'admin',
			label: 'Admin',
			description: 'Everything, including users, secrets and scripts.',
			icon: ShieldCheckIcon
		},
		{
			value: 'deployer',
			label: 'Deployer',
			description: 'View projects and logs. Deploy, cancel and redeploy.',
			icon: RocketIcon
		},
		{
			value: 'viewer',
			label: 'Viewer',
			description: 'Read-only access to projects, deployments and logs.',
			icon: EyeIcon
		}
	] as const satisfies readonly {
		value: Role;
		label: string;
		description: string;
		icon: unknown;
	}[];

	export const roleInfo = (r: Role) => roles.find((x) => x.value === r) ?? roles[2];
</script>

<script lang="ts">
	import { cn } from '#lib/utils.js';

	// Three role cards behaving as a radio group.
	let {
		value = $bindable('viewer'),
		disabled = false,
		labelledby
	}: { value?: Role; disabled?: boolean; labelledby?: string } = $props();

	let buttons = $state<HTMLButtonElement[]>([]);

	function onkeydown(e: KeyboardEvent) {
		const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
		if (!step) return;
		e.preventDefault();
		const i = (roles.findIndex((r) => r.value === value) + step + roles.length) % roles.length;
		value = roles[i].value;
		buttons[i]?.focus();
	}
</script>

<div
	class="grid gap-2.5 sm:grid-cols-3"
	role="radiogroup"
	aria-labelledby={labelledby}
	aria-disabled={disabled}
>
	{#each roles as r, i (r.value)}
		{@const active = value === r.value}
		<button
			bind:this={buttons[i]}
			type="button"
			role="radio"
			aria-checked={active}
			tabindex={active ? 0 : -1}
			{disabled}
			class={cn(
				'flex flex-col gap-2.5 rounded-lg border p-3.5 text-left transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50',
				active
					? 'border-foreground bg-muted'
					: 'border-input bg-background enabled:hover:border-muted-foreground/60'
			)}
			onclick={() => (value = r.value)}
			{onkeydown}
		>
			<span class="flex items-center justify-between">
				<span
					class={cn(
						'flex size-7 items-center justify-center rounded-md [&_svg]:size-3.5',
						active ? 'bg-foreground text-background' : 'bg-muted text-muted-foreground'
					)}
				>
					<r.icon />
				</span>
				<span
					class={cn(
						'flex size-4 items-center justify-center rounded-full',
						active ? 'bg-foreground' : 'border border-input'
					)}
					aria-hidden="true"
				>
					{#if active}<span class="size-1.5 rounded-full bg-background"></span>{/if}
				</span>
			</span>
			<span class="flex flex-col gap-1">
				<span class="text-sm font-medium">{r.label}</span>
				<span
					class={cn(
						'text-xs/[1.4] text-pretty',
						active ? 'text-muted-foreground' : 'text-muted-foreground/80'
					)}>{r.description}</span
				>
			</span>
		</button>
	{/each}
</div>
