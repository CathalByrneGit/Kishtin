<script lang="ts">
  import { Button, SelectDropdown, showFlash } from "@kenn-io/kit-ui";
  import { Printer } from "@lucide/svelte";
  import { api, download, message } from "../lib/api";
  import { load, save } from "../lib/storage";

  // Prints a batch label record as a label. The label is a PDF the exact
  // size of the label, opened in a new tab to print from; the size is
  // remembered on this device, which is usually the one beside the printer.

  interface Props {
    recordId: string;
    seq: number;
    product: string;
  }
  let { recordId, seq, product }: Props = $props();

  const SIZE_KEY = "kishtin-label-size";
  const SIZES = [
    { value: "62x29", label: "62 × 29 mm label" },
    { value: "100x50", label: "100 × 50 mm label" },
    { value: "a4", label: "A4 page, to cut out" },
  ];
  let size = $state(SIZES.some((s) => s.value === load(SIZE_KEY)) ? (load(SIZE_KEY) as string) : "62x29");
  let busy = $state(false);

  async function print() {
    busy = true;
    // Open the tab now, while the tap still counts as the reason for it.
    const tab = window.open("", "_blank");
    try {
      const blob = await api.labelPDF(recordId, size);
      const url = URL.createObjectURL(blob);
      if (tab) {
        tab.location.href = url;
      } else {
        download(blob, `label-${seq}.pdf`);
        showFlash("Label downloaded. Open it to print.", { tone: "success" });
      }
      setTimeout(() => URL.revokeObjectURL(url), 60_000);
    } catch (err) {
      tab?.close();
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    } finally {
      busy = false;
    }
  }
</script>

<div class="label">
  <SelectDropdown
    value={size}
    options={SIZES}
    title="Label size"
    onchange={(v) => {
      size = v;
      save(SIZE_KEY, v);
    }}
  />
  <Button label={busy ? "Making label…" : "Print label"} ariaLabel={`Print a label for ${product}`} disabled={busy} onclick={print}><Printer size={16} /></Button>
</div>

<style>
  .label {
    display: flex;
    gap: var(--space-2);
    align-items: center;
    flex-wrap: wrap;
  }
</style>
