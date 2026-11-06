<script lang="ts">
  let { size = 36 }: { size?: number } = $props();
</script>

<!--
  The M1 compass card (DESIGN §Logo): brass on dark surfaces, slate on light.
  Decorative; the wordmark or the page title beside it names the product.
  The inline theme script in index.html sets data-theme before first paint,
  so the right variant shows from the first frame.
-->
<span class="brand-mark" style:--mark-size={`${size}px`}>
  <img
    class="dark"
    src="/brand/binnacle-mark.svg"
    alt=""
    width={size}
    height={size}
  />
  <img
    class="light"
    src="/brand/binnacle-mark-on-light.svg"
    alt=""
    width={size}
    height={size}
  />
</span>

<style>
  .brand-mark {
    display: inline-flex;
    flex: none;
    width: var(--mark-size);
    height: var(--mark-size);
  }
  img {
    display: block;
  }
  .light {
    display: none;
  }
  :global(html[data-theme='light']) .light {
    display: block;
  }
  :global(html[data-theme='light']) .dark {
    display: none;
  }
</style>
