<script lang="ts">
  interface Props {
    percent: number;
    /** attention (red), complete (green) or in_progress (blue) */
    state: string;
    label: string;
  }
  let { percent, state, label }: Props = $props();

  const r = 52;
  const circumference = 2 * Math.PI * r;
  const offset = $derived(circumference * (1 - Math.min(100, Math.max(0, percent)) / 100));
  const tone = $derived(state === "attention" ? "danger" : state === "complete" ? "success" : "info");
</script>

<div class="ring" data-kit-tone={tone} role="img" aria-label={label}>
  <svg viewBox="0 0 120 120" width="120" height="120" aria-hidden="true">
    <circle class="track" cx="60" cy="60" r={r} />
    <circle class="value" cx="60" cy="60" r={r} stroke-dasharray={circumference} stroke-dashoffset={offset} />
  </svg>
  <span class="pct num">{percent}%</span>
</div>

<style>
  .ring {
    position: relative;
    width: 120px;
    height: 120px;
    flex: none;
  }
  svg {
    transform: rotate(-90deg);
  }
  circle {
    fill: none;
    stroke-width: 12;
  }
  .track {
    stroke: var(--bg-inset);
  }
  .value {
    stroke: var(--kit-tone);
    stroke-linecap: round;
    transition: stroke-dashoffset var(--transition-fast);
  }
  .pct {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    font-size: var(--font-size-2xl);
    font-weight: var(--font-weight-bold);
  }
</style>
