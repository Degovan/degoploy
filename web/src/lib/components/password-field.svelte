<script lang="ts">
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { generatePassword } from '#lib/password.js';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';
	import WandIcon from '@lucide/svelte/icons/wand-sparkles';

	let {
		id,
		value = $bindable(''),
		autocomplete = 'new-password',
		generate = true,
		invalid = false
	}: {
		id: string;
		value?: string;
		autocomplete?: 'new-password' | 'current-password';
		generate?: boolean;
		invalid?: boolean;
	} = $props();

	let visible = $state(false);
</script>

<div class="flex gap-1">
	<Input
		{id}
		type={visible ? 'text' : 'password'}
		bind:value
		{autocomplete}
		required
		minlength={autocomplete === 'new-password' ? 10 : undefined}
		aria-invalid={invalid}
		class="font-mono"
	/>
	<Button
		variant="outline"
		size="icon"
		onclick={() => (visible = !visible)}
		aria-label={visible ? 'Hide password' : 'Show password'}
	>
		{#if visible}<EyeOffIcon />{:else}<EyeIcon />{/if}
	</Button>
	{#if generate}
		<Button
			variant="outline"
			size="icon"
			onclick={() => {
				value = generatePassword();
				visible = true;
			}}
			aria-label="Generate password"
			title="Generate a random password"
		>
			<WandIcon />
		</Button>
	{/if}
</div>
