<script lang="ts">
  interface Props {
    /** rows of the code, top first: "1" dark, "0" light */
    rows: string[];
    /** what the code holds, for screen readers */
    label: string;
    size?: number;
  }
  let { rows, label, size = 260 }: Props = $props();

  // A quiet zone of four modules, as scanners expect.
  const quiet = 4;
  const n = $derived(rows.length + quiet * 2);
  const path = $derived(
    rows
      .flatMap((row, y) => [...row].map((c, x) => (c === "1" ? `M${x + quiet} ${y + quiet}h1v1h-1z` : "")))
      .join(""),
  );
</script>

<!-- Always dark on light, whatever the theme: phone cameras need that contrast. -->
<svg class="qr" width={size} height={size} viewBox="0 0 {n} {n}" role="img" aria-label={label} shape-rendering="crispEdges">
  <rect width={n} height={n} fill="white" />
  <path d={path} fill="black" />
</svg>

<style>
  .qr {
    display: block;
    max-width: 100%;
    height: auto;
    border-radius: var(--radius-md);
  }
</style>
