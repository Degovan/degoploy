<script lang="ts">
	import { cn } from '#lib/utils.js';

	let { class: className }: { class?: string } = $props();
</script>

<div
	class={cn(
		'flex aspect-square items-center justify-center rounded-lg bg-primary text-primary-foreground',
		className
	)}
>
	<svg viewBox="0 0 24 24" class="size-[60%]" fill="none" stroke="currentColor" stroke-width="2.2">
		<path d="M12 3v12" stroke-linecap="round" />
		<path d="m7 10 5 5 5-5" stroke-linecap="round" stroke-linejoin="round" />
		<path d="M5 20h14" stroke-linecap="round" />
	</svg>
</div>
