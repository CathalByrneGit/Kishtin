<script lang="ts">
  import { TextInput } from "@kenn-io/kit-ui";

  interface Props {
    value: string;
    label: string;
    placeholder?: string;
    /** "decimal" for temperatures, "numeric" for whole numbers and times */
    mode?: "decimal" | "numeric";
    invalid?: boolean;
  }
  let { value = $bindable(), label, placeholder = "0.0", mode = "decimal", invalid = false }: Props = $props();

  let inputEl = $state<HTMLInputElement | undefined>();
  // TextInput has no inputmode prop; set it so phones show a number keypad.
  $effect(() => {
    inputEl?.setAttribute("inputmode", mode);
  });
</script>

<label class="field temp">
  <span>{label}</span>
  <TextInput bind:value bind:inputEl {placeholder} {invalid} size="lg" autocomplete="off" block />
</label>

<style>
  .temp :global(input) {
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
    font-size: var(--font-size-xl);
  }
</style>
