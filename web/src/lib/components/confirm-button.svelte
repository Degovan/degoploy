<script lang="ts">
	import type { Snippet } from 'svelte';
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.js';
	import {
		Button,
		buttonVariants,
		type ButtonSize,
		type ButtonVariant
	} from '#lib/components/ui/button/index.js';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';

	let {
		title,
		description,
		confirmLabel = 'Confirm',
		destructive = false,
		variant = 'outline',
		size = 'default',
		disabled = false,
		label,
		onconfirm,
		children
	}: {
		title: string;
		description: string;
		confirmLabel?: string;
		destructive?: boolean;
		variant?: ButtonVariant;
		size?: ButtonSize;
		disabled?: boolean;
		/** Accessible name; required when children are only an icon. */
		label?: string;
		onconfirm: () => Promise<void> | void;
		children: Snippet;
	} = $props();

	let open = $state(false);
	let busy = $state(false);

	async function confirm() {
		busy = true;
		try {
			await onconfirm();
			open = false;
		} finally {
			busy = false;
		}
	}
</script>

<AlertDialog.Root bind:open>
	<AlertDialog.Trigger class={buttonVariants({ variant, size })} {disabled} aria-label={label}>
		{@render children()}
	</AlertDialog.Trigger>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{title}</AlertDialog.Title>
			<AlertDialog.Description>{description}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel disabled={busy}>Cancel</AlertDialog.Cancel>
			<Button variant={destructive ? 'destructive' : 'default'} onclick={confirm} disabled={busy}>
				{#if busy}<LoaderIcon class="animate-spin" />{/if}
				{confirmLabel}
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
