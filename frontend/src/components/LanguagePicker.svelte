<script lang="ts">
  import { SegmentedControl } from "@kenn-io/kit-ui";
  import { LANGUAGES, lang, setLang, t, type Lang } from "../lib/i18n.svelte";

  // Chooses the language of the staff screens: for the person logged in on
  // this device, or for the device when nobody is.
</script>

<div class="field">
  <span>{t("Language")}</span>
  <SegmentedControl block ariaLabel="Language" value={lang()} options={LANGUAGES} onchange={(v) => setLang(v as Lang)} />
</div>
