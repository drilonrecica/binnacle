<script lang="ts">
  // Stands in for the uPlot chart, which jsdom cannot draw.
  let { label }: { label: string } = $props();
</script>

<div data-testid="chart-stub">{label}</div>
