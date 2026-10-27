<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { Button } from '#lib/components/ui/button/index.js';
	import { copyText } from '#lib/clipboard.js';
	import { cn } from '#lib/utils.js';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';

	let {
		value,
		secret = false,
		multiline = false,
		label = 'Value',
		class: className
	}: {
		value: string;
		secret?: boolean;
		multiline?: boolean;
		label?: string;
		class?: string;
	} = $props();

	let revealed = $state(false);
	let copied = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;

	const shown = $derived(secret && !revealed ? '•'.repeat(Math.min(value.length, 32)) : value);

	async function copy() {
		if (await copyText(value)) {
			copied = true;
			clearTimeout(timer);
			timer = setTimeout(() => (copied = false), 1500);
		} else {
			toast.error('Could not copy; select the text manually');
		}
	}
</script>

<div
	class={cn(
		'flex items-start gap-1 rounded-md border bg-muted/40 p-1 pl-3 font-mono text-xs',
		className
	)}
>
	<code
		class={cn(
			'min-w-0 flex-1 self-center py-1.5 select-all',
			multiline ? 'break-all whitespace-pre-wrap' : 'truncate'
		)}
		aria-label={label}>{shown}</code
	>
	{#if secret}
		<Button
			variant="ghost"
			size="icon-sm"
			onclick={() => (revealed = !revealed)}
			aria-label={revealed ? `Hide ${label}` : `Show ${label}`}
		>
			{#if revealed}<EyeOffIcon />{:else}<EyeIcon />{/if}
		</Button>
	{/if}
	<Button variant="ghost" size="icon-sm" onclick={copy} aria-label={`Copy ${label}`}>
		{#if copied}<CheckIcon class="text-emerald-600" />{:else}<CopyIcon />{/if}
	</Button>
</div>
