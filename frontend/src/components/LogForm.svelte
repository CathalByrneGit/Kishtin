<script module lang="ts">
  import { can, canWithPassword } from "../lib/can.svelte";
  import type { HaccpRecord } from "../lib/api";

  export interface LogPreset {
    /** a record type, such as fridge or cleaning */
    type: string;
    unit?: string;
    task?: string;
    /** a record to correct: the form starts from its values */
    corrects?: HaccpRecord;
  }
</script>

<script lang="ts">
  import {
    Button,
    Checkbox,
    Chip,
    DatePicker,
    Notice,
    SegmentedControl,
    SelectDropdown,
    TextInput,
    debounce,
    showFlash,
  } from "@kenn-io/kit-ui";
  import { Camera } from "@lucide/svelte";
  import { onMount } from "svelte";
  import TempInput from "./TempInput.svelte";
  import { api, message, OfflineError, type Check, type Lists, type NewRecord } from "../lib/api";
  import { title as recordTitle, typeLabel } from "../lib/labels";
  import { addDays, coolingTimes, dublinDate, dublinTime, dublinToDate, isClock, longDate, normaliseClock, parseTemp } from "../lib/format";
  import { live, onChange } from "../lib/live.svelte";
  import { keep, LISTS_KEY } from "../lib/kept.svelte";
  import { t, tAny } from "../lib/i18n.svelte";
  import { load as loadLocal, save as saveLocal } from "../lib/storage";
  import { session } from "../lib/session.svelte";

  interface Props {
    preset: LogPreset | null;
    onsaved: () => void;
  }
  let { preset, onsaved }: Props = $props();

  type Kind = "temp" | "delivery" | "cook" | "cooling" | "hot_hold" | "batch_label" | "cleaning" | "transport" | "training" | "fitness";
  const canInspect = $derived(can("haccp.inspect"));
  // Training is for the head chef and managers; fitness to work, which is
  // health information, for a manager with a password.
  const kinds = $derived(
    (
      [
        { id: "temp", label: t("Fridge / freezer") },
        { id: "delivery", label: t("Delivery") },
        { id: "cook", label: t("Cook / reheat") },
        { id: "cooling", label: t("Cooling") },
        { id: "hot_hold", label: t("Hot holding") },
        { id: "batch_label", label: t("Batch label") },
        { id: "cleaning", label: t("Cleaning") },
        { id: "transport", label: t("To a customer") },
        { id: "training", label: "Training", show: canInspect },
        { id: "fitness", label: "Fitness to work", show: can("haccp.fitness") },
      ] as { id: Kind; label: string; show?: boolean }[]
    ).filter((k) => k.show !== false),
  );

  let lists = $state<Lists | null>(null);
  let kind = $state<Kind>("temp");

  // Form fields, kept across type changes so nothing typed is lost.
  let unit = $state("");
  let tempText = $state("");
  let supplier = $state("");
  let product = $state("");
  let deliveryKind = $state("chilled");
  let packagingOk = $state(false);
  let inDate = $state(false);
  let batchCode = $state("");
  let deliveryUseBy = $state<string | null>(null);
  /** vehicle is the delivery vehicle check: not checked, OK or failed. */
  let vehicle = $state<"" | "ok" | "failed">("");
  let docket = $state<{ sha: string; name: string } | null>(null);
  let uploading = $state(false);
  let food = $state("");
  let stage = $state("cook");
  let category = $state("");
  let cookedAt = $state("");
  let fridgeAt = $state("");
  let cutoffText = $state("");
  let madeOn = $state(dublinDate(new Date()));
  let shelfLife = $state("5");
  let task = $state("");
  let action = $state("");
  let notes = $state("");
  let correctionReason = $state("");
  // When the check was done: now, or earlier (a date and time), and who did
  // it when that was someone else.
  let when = $state<"now" | "earlier">("now");
  let whenDate = $state(dublinDate(new Date()));
  let whenTime = $state("");
  let doneBy = $state("");
  // Transport, training and fitness.
  let customer = $state("");
  let separated = $state<"" | "yes" | "no">("");
  let staffName = $state("");
  let trainingKind = $state("in_house");
  let topics = $state<string[]>([]);
  let course = $state("");
  let trainer = $state(session.user?.name ?? "");
  let provider = $state("");
  let onDate = $state(dublinDate(new Date()));
  let fitnessReason = $state("existing");
  let fit = $state<"" | "yes" | "no">("");
  /** topicList is the training topics of the booklet in use, in sections. */
  let topicList = $state<{ title?: string; items: string[] }[]>([]);

  let verdict = $state<Check | null>(null);
  /** unreachable is set when Kishtin could not be asked to check a reading. */
  let unreachable = $state(false);
  let verdictError = $state("");
  let saving = $state(false);
  let fileInput = $state<HTMLInputElement | undefined>();

  async function loadLists() {
    try {
      lists = (await api.lists()).value;
      // Kept on the device so the form still knows the fridges and tasks
      // when Kishtin cannot be reached.
      saveLocal(LISTS_KEY, JSON.stringify(lists));
    } catch (err) {
      verdictError = message(err);
      if (err instanceof OfflineError) {
        unreachable = true;
        try {
          lists = JSON.parse(loadLocal(LISTS_KEY) ?? "null") as Lists | null;
        } catch {
          lists = null;
        }
      }
    }
    if (lists) {
      if (!unit && lists.units[0]) unit = lists.units[0].name;
      if (!task && lists.cleaning_tasks[0]) task = lists.cleaning_tasks[0].task;
    }
  }

  async function loadTopics() {
    if (!canInspect) return;
    try {
      const rb = await api.recordBooks();
      const t = rb.booklets.find((b) => !b.off && b.template.builder === "training")?.template;
      topicList = t?.checklist ?? [];
    } catch {
      topicList = [];
    }
  }

  onMount(() => {
    void loadLists();
    void loadTopics();
    return onChange((k) => {
      if (k === "settings") void loadLists();
    });
  });

  /** startFrom fills the form from a record being corrected. */
  function startFrom(r: HaccpRecord) {
    const rd = r.reading;
    const t = (c: number | undefined | null) => (c === undefined || c === null ? "" : String(c));
    kind = r.type === "fridge" || r.type === "freezer" ? "temp" : r.type === "reheat" ? "cook" : (r.type as Kind);
    customer = rd.customer ?? "";
    separated = rd.separated === true ? "yes" : rd.separated === false ? "no" : "";
    staffName = rd.staff ?? "";
    if (rd.training_kind) trainingKind = rd.training_kind;
    topics = rd.training_kind === "in_house" ? [...(rd.topics ?? [])] : [];
    course = rd.training_kind === "further" ? (rd.topics ?? []).join(", ") : "";
    trainer = rd.trainer ?? trainer;
    provider = rd.provider ?? "";
    if (rd.on_date) onDate = rd.on_date;
    if (rd.fitness_reason) fitnessReason = rd.fitness_reason;
    fit = rd.fit === true ? "yes" : rd.fit === false ? "no" : "";
    if (r.type === "cook" || r.type === "reheat") stage = r.type;
    unit = rd.unit ?? unit;
    tempText = t(rd.temp_c);
    supplier = rd.supplier ?? "";
    product = rd.product ?? "";
    deliveryKind = rd.delivery_kind ?? "chilled";
    packagingOk = rd.packaging_ok ?? false;
    inDate = rd.in_date ?? false;
    batchCode = rd.batch_code ?? "";
    deliveryUseBy = r.type === "delivery" ? (rd.use_by ?? null) : null;
    vehicle = rd.vehicle_ok === true ? "ok" : rd.vehicle_ok === false ? "failed" : "";
    docket = rd.docket_file ? { sha: rd.docket_file, name: "attached docket" } : null;
    food = rd.food ?? "";
    category = rd.food_category ?? "";
    cookedAt = rd.cooked_at ? dublinTime(rd.cooked_at) : "";
    fridgeAt = rd.into_fridge_at ? dublinTime(rd.into_fridge_at) : "";
    cutoffText = t(rd.temp_at_cutoff_c);
    if (rd.made_on) madeOn = rd.made_on;
    if (rd.made_on && rd.use_by) {
      shelfLife = String(Math.round((Date.parse(rd.use_by) - Date.parse(rd.made_on)) / 86_400_000));
    }
    task = rd.task ?? task;
    notes = rd.notes ?? "";
    action = "";
    correctionReason = "";
    const at = new Date(rd.taken_at ?? r.logged_at);
    when = "now";
    whenDate = dublinDate(at);
    whenTime = dublinTime(at);
    doneBy = rd.signed_by ?? "";
  }

  const correcting = $derived(preset?.corrects ?? null);

  $effect(() => {
    if (!preset) return;
    if (preset.corrects) {
      startFrom(preset.corrects);
    } else if (preset.type === "fridge" || preset.type === "freezer") {
      kind = "temp";
      if (preset.unit) unit = preset.unit;
    } else if (preset.type === "cleaning") {
      kind = "cleaning";
      if (preset.task) task = preset.task;
    }
  });

  const unitKind = $derived(lists?.units.find((u) => u.name === unit)?.kind ?? "fridge");
  // Training and fitness to work have their own date; the others can say
  // when the check was done, if a manager allows it.
  const showWhen = $derived(kind !== "training" && kind !== "fitness" && (canWithPassword("haccp.backdate") || Boolean(correcting)));
  const earlier = $derived(showWhen && when === "earlier");
  const takenAt = $derived.by((): string | null | "bad" => {
    if (!earlier) return null;
    const t = normaliseClock(whenTime);
    if (!/^\d{4}-\d{2}-\d{2}$/.test(whenDate) || !isClock(t)) return "bad";
    return dublinToDate(whenDate, t).toISOString();
  });
  const cooling = $derived(coolingTimes(earlier ? whenDate : dublinDate(new Date()), normaliseClock(cookedAt), normaliseClock(fridgeAt)));
  const useBy = $derived(/^\d+$/.test(shelfLife.trim()) ? addDays(madeOn, Number(shelfLife)) : "");

  /** draft builds the record from the form, or says what is missing. */
  const draft = $derived.by((): { record: NewRecord } | { missing: string } => {
    const t = parseTemp(tempText);
    const needTemp = (what: string) => (t === null ? { missing: tempText.trim() ? `${what} must be a number, such as 3.4` : `Enter the ${what}.` } : null);
    switch (kind) {
      case "temp":
        if (!unit) return { missing: "Choose a fridge or freezer." };
        return needTemp("temperature") ?? { record: { type: unitKind, reading: { unit, temp_c: t! } } };
      case "delivery": {
        if (!supplier.trim() || !product.trim()) return { missing: "Enter the supplier and product." };
        const reading: NewRecord["reading"] = {
          supplier, product, delivery_kind: deliveryKind, packaging_ok: packagingOk, in_date: inDate,
        };
        if (docket) reading.docket_file = docket.sha;
        if (batchCode.trim()) reading.batch_code = batchCode.trim();
        if (deliveryUseBy) reading.use_by = deliveryUseBy;
        if (vehicle) reading.vehicle_ok = vehicle === "ok";
        if (deliveryKind !== "ambient") {
          const m = needTemp("probe temperature");
          if (m) return m;
          reading.temp_c = t!;
        }
        return { record: { type: "delivery", reading } };
      }
      case "cook": {
        if (!food.trim()) return { missing: "Enter the food." };
        const reading: NewRecord["reading"] = { food, temp_c: t ?? undefined };
        if (category) reading.food_category = category;
        return needTemp("core temperature") ?? { record: { type: stage, reading } };
      }
      case "cooling": {
        if (!food.trim()) return { missing: "Enter the food." };
        if (!isClock(normaliseClock(cookedAt)) || !isClock(normaliseClock(fridgeAt))) {
          return { missing: "Enter both times, such as 14:30." };
        }
        if (!cooling) return { missing: "Check the times: the gap is 12 hours or more." };
        const reading: NewRecord["reading"] = {
          food, cooked_at: cooling.cookedAt.toISOString(), into_fridge_at: cooling.intoFridgeAt.toISOString(),
        };
        if (t !== null) reading.temp_c = t;
        const cut = parseTemp(cutoffText);
        if (cooling.minutes > 120 && cut !== null) reading.temp_at_cutoff_c = cut;
        return { record: { type: "cooling", reading } };
      }
      case "hot_hold":
        if (!food.trim()) return { missing: "Enter the food." };
        return needTemp("core temperature") ?? { record: { type: "hot_hold", reading: { food, temp_c: t! } } };
      case "batch_label":
        if (!product.trim()) return { missing: "Enter the item." };
        if (!useBy) return { missing: "Shelf life must be a whole number of days." };
        return { record: { type: "batch_label", reading: { product, made_on: madeOn, shelf_life_days: Number(shelfLife) } } };
      case "cleaning":
        if (!task) return { missing: "Choose a task." };
        return { record: { type: "cleaning", reading: { task, done: true } } };
      case "transport": {
        if (!product.trim() || !customer.trim()) return { missing: "Enter the food and the customer." };
        if (!separated) return { missing: "Say whether raw and ready-to-eat food were kept apart." };
        const reading: NewRecord["reading"] = { product, customer, delivery_kind: deliveryKind, separated: separated === "yes" };
        if (batchCode.trim()) reading.batch_code = batchCode.trim();
        if (deliveryUseBy) reading.use_by = deliveryUseBy;
        if (deliveryKind !== "ambient") {
          const m = needTemp("temperature on arrival");
          if (m) return m;
          reading.temp_c = t!;
        }
        return { record: { type: "transport", reading } };
      }
      case "training": {
        if (!staffName.trim()) return { missing: "Enter who was trained." };
        const reading: NewRecord["reading"] = { staff: staffName, training_kind: trainingKind, on_date: onDate };
        if (trainingKind === "in_house") {
          if (topics.length === 0) return { missing: "Tick the topics covered." };
          if (!trainer.trim()) return { missing: "Enter the trainer." };
          reading.topics = topics;
          reading.trainer = trainer;
        } else {
          if (!course.trim() || !provider.trim()) return { missing: "Enter the course and its provider." };
          reading.topics = [course];
          reading.provider = provider;
        }
        return { record: { type: "training", reading } };
      }
      case "fitness":
        if (!staffName.trim()) return { missing: "Enter who was assessed." };
        if (!fit) return { missing: "Say whether they are fit to work." };
        return { record: { type: "fitness", reading: { staff: staffName, fitness_reason: fitnessReason, fit: fit === "yes", on_date: onDate } } };
    }
  });

  /** full adds the notes, when the check was done and who did it. */
  function full(record: NewRecord): NewRecord {
    const reading = { ...record.reading, notes: kind === "fitness" ? undefined : notes || undefined };
    if (takenAt && takenAt !== "bad") reading.taken_at = takenAt;
    if (showWhen && doneBy.trim()) reading.signed_by = doneBy.trim();
    return { ...record, reading };
  }

  const runCheck = debounce(async (body: NewRecord) => {
    try {
      verdict = await api.check(body);
      verdictError = "";
      unreachable = false;
    } catch (err) {
      verdict = null;
      verdictError = message(err);
      unreachable = err instanceof OfflineError;
    }
  }, 300);

  $effect(() => {
    const d = draft;
    if ("record" in d && takenAt !== "bad") {
      runCheck(full(d.record));
    } else {
      runCheck.cancel();
      verdict = null;
      verdictError = "";
    }
  });

  const recordType = $derived("record" in draft ? draft.record.type : "");
  const presets = $derived((lists?.corrective_actions ?? []).filter((a) => a.type === recordType));
  const needsAction = $derived(Boolean(verdict?.needs_corrective_action) || (kind === "fitness" && fit === "no"));
  const canSave = $derived(
    Boolean(verdict) &&
      "record" in draft &&
      takenAt !== "bad" &&
      (!needsAction || action.trim() !== "") &&
      (!correcting || correctionReason.trim() !== "") &&
      !saving,
  );

  // With Kishtin out of reach, a check can be kept on this device and sent
  // later. Not corrections (the original must be read), not a delivery with
  // a docket photo (it cannot be uploaded), and not fitness to work, which
  // is health information and stays off the device.
  const offline = $derived(unreachable || live.status === "offline");
  const canKeep = $derived(offline && !verdict && "record" in draft && takenAt !== "bad" && !correcting && !docket && kind !== "fitness" && !saving);

  function clearForm() {
    tempText = "";
    action = "";
    notes = "";
    correctionReason = "";
    cutoffText = "";
    whenTime = "";
    docket = null;
    if (kind !== "temp") {
      food = "";
      product = "";
      supplier = "";
      cookedAt = "";
      fridgeAt = "";
      packagingOk = false;
      inDate = false;
      batchCode = "";
      deliveryUseBy = null;
      vehicle = "";
      customer = "";
      separated = "";
      staffName = "";
      topics = [];
      course = "";
      provider = "";
      fit = "";
    }
  }

  function keepHere() {
    if (!("record" in draft)) return;
    const body = full(draft.record);
    if (action.trim()) body.corrective_action = action.trim();
    const rd = body.reading;
    const what = rd.unit ?? rd.food ?? rd.product ?? rd.task ?? rd.staff ?? "";
    const label = [typeLabel(body.type), what, rd.temp_c !== undefined ? `${rd.temp_c}°C` : ""].filter(Boolean).join(" · ");
    if (keep(body, label)) {
      live.saveFailed = true;
      showFlash(t("Kept on this device. It is not in Kishtin until the connection is back."), { tone: "warning", durationMs: 8000 });
      clearForm();
      onsaved();
    } else {
      showFlash(t("This device could not keep the check. Write it down, and log it when the connection is back."), { tone: "danger", durationMs: 10000 });
    }
  }

  async function pickPhoto(e: Event) {
    const file = (e.currentTarget as HTMLInputElement).files?.[0];
    if (!file) return;
    uploading = true;
    try {
      const m = await api.upload(file);
      docket = { sha: m.sha256, name: file.name };
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    } finally {
      uploading = false;
      if (fileInput) fileInput.value = "";
    }
  }

  async function save() {
    if (!("record" in draft)) return;
    saving = true;
    try {
      const body = full(draft.record);
      if (needsAction) body.corrective_action = action.trim();
      if (correcting) {
        body.corrects = correcting.id;
        body.correction_reason = correctionReason.trim();
      }
      await api.log(body);
      live.saveFailed = false;
      showFlash(t("Saved"), { tone: "success" });
      clearForm();
      onsaved();
    } catch (err) {
      if (err instanceof OfflineError) {
        live.saveFailed = true;
        unreachable = true;
        verdict = null;
      }
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      saving = false;
    }
  }

  const verdictTone = $derived(
    verdict?.result.outcome === "fail" ? "error" : verdict?.result.outcome === "borderline" ? "warning" : verdict?.result.outcome === "pass" ? "success" : "info",
  );
  const verdictTitle = $derived(
    ({ pass: t("Pass"), borderline: t("Borderline"), fail: t("Fail"), recorded: t("Recorded") } as Record<string, string>)[verdict?.result.outcome ?? ""] ?? "",
  );
  const limitSource = $derived.by(() => {
    const l = verdict?.limit;
    if (!l) return "";
    const checks: Record<string, string> = {
      fridge: "fridges", freezer: "freezers", cook: "cooking", reheat: "reheating", hot_hold: "hot holding",
      cooling: "cooling", delivery_chilled: "chilled deliveries", delivery_frozen: "frozen deliveries",
    };
    return l.scope.kind === "default" ? `Limit for all ${checks[l.check] ?? l.check}` : `Limit for ${l.scope.kind} ${l.scope.ref}`;
  });
</script>

<div class="stack form">
  {#if correcting}
    <Notice
      tone="info"
      title={`Correcting record ${correcting.seq}: ${recordTitle(correcting)}`}
      message={`The original ${typeLabel(correcting.type).toLowerCase()} record stays as it was. This new record points to it and takes its place on Today.`}
    />
  {:else}
    <div class="kinds">
      {#each kinds as k (k.id)}
        <Button label={k.label} tone={kind === k.id ? "info" : "neutral"} surface={kind === k.id ? "solid" : "outline"} onclick={() => (kind = k.id)} />
      {/each}
    </div>
  {/if}

  <div class="stack">
      {#if kind === "temp"}
        <label class="field">
          <span>{t("Fridge or freezer")}</span>
          <SelectDropdown
            value={unit}
            options={(lists?.units ?? []).map((u) => ({ value: u.name, label: `${u.name} (${u.kind})` }))}
            onchange={(v) => (unit = v)}
            title={t("Fridge or freezer")}
          />
        </label>
        <TempInput bind:value={tempText} label={t("Temperature (°C)")} />
      {:else if kind === "delivery"}
        <label class="field"><span>{t("Supplier")}</span><TextInput bind:value={supplier} block /></label>
        <label class="field"><span>{t("Product")}</span><TextInput bind:value={product} block /></label>
        <SegmentedControl
          block
          ariaLabel="Delivery kind"
          value={deliveryKind}
          onchange={(v) => (deliveryKind = v)}
          options={[
            { value: "chilled", label: t("Chilled") },
            { value: "frozen", label: t("Frozen") },
            { value: "ambient", label: t("Ambient") },
          ]}
        />
        {#if deliveryKind !== "ambient"}
          <TempInput bind:value={tempText} label={t("Probe temperature (°C)")} />
        {/if}
        <Checkbox bind:checked={packagingOk} label={t("Packaging intact")} />
        <Checkbox bind:checked={inDate} label={t("In date")} />
        <div class="field">
          <span>{t("Delivery vehicle check")}</span>
          <SegmentedControl
            block
            ariaLabel="Delivery vehicle check"
            value={vehicle}
            onchange={(v) => (vehicle = v as typeof vehicle)}
            options={[
              { value: "", label: t("Not checked") },
              { value: "ok", label: t("Clean and cold") },
              { value: "failed", label: t("Failed") },
            ]}
          />
        </div>
        <label class="field"><span>{t("Batch code (optional)")}</span><TextInput bind:value={batchCode} block /></label>
        <div class="field">
          <span>{t("Use-by date (optional)")}</span>
          <div class="row">
            <DatePicker value={deliveryUseBy} onchange={(d) => (deliveryUseBy = d)} ariaLabel="Use-by date" block />
            {#if deliveryUseBy}<Button label={t("Clear")} onclick={() => (deliveryUseBy = null)} />{/if}
          </div>
        </div>
        <div class="row">
          <input bind:this={fileInput} class="kit-sr-only" type="file" accept="image/*,application/pdf" capture="environment" onchange={pickPhoto} />
          <Button label={uploading ? t("Uploading…") : docket ? t("Replace docket photo") : t("Add docket photo")} disabled={uploading} onclick={() => fileInput?.click()}>
            <Camera size={20} />
          </Button>
          {#if docket}<Chip tone="success" uppercase={false}>{t("Attached: {name}", { name: docket.name })}</Chip>{/if}
        </div>
      {:else if kind === "cook"}
        <label class="field"><span>{t("Food")}</span><TextInput bind:value={food} block /></label>
        <SegmentedControl
          block
          ariaLabel="Stage"
          value={stage}
          onchange={(v) => (stage = v)}
          options={[
            { value: "cook", label: t("Cooking") },
            { value: "reheat", label: t("Reheating") },
          ]}
        />
        {#if (lists?.food_categories.length ?? 0) > 0}
          <label class="field">
            <span>{t("Food category (sets the limit)")}</span>
            <SelectDropdown
              value={category}
              options={[{ value: "", label: t("None") }, ...(lists?.food_categories ?? []).map((c) => ({ value: c, label: c }))]}
              onchange={(v) => (category = v)}
              title="Food category"
            />
          </label>
        {/if}
        <TempInput bind:value={tempText} label={t("Core temperature (°C)")} />
      {:else if kind === "cooling"}
        <label class="field"><span>{t("Food")}</span><TextInput bind:value={food} block /></label>
        <div class="two">
          <div class="stack">
            <TempInput bind:value={cookedAt} label={t("Finished cooking")} placeholder="14:30" mode="numeric" />
            <Button label={t("Now")} onclick={() => (cookedAt = dublinTime(new Date()))} />
          </div>
          <div class="stack">
            <TempInput bind:value={fridgeAt} label={t("Into fridge or blast chiller")} placeholder="15:45" mode="numeric" />
            <Button label={t("Now")} onclick={() => (fridgeAt = dublinTime(new Date()))} />
          </div>
        </div>
        {#if cooling}
          <span class="muted">{t("{h} h {m} min between cooking and the fridge", { h: Math.floor(cooling.minutes / 60), m: cooling.minutes % 60 })}{dublinDate(cooling.cookedAt) !== dublinDate(cooling.intoFridgeAt) ? ` ${t("(over midnight)")}` : ""}.</span>
        {/if}
        <TempInput bind:value={tempText} label={t("Core temperature going in (°C, optional)")} />
        {#if cooling && cooling.minutes > 120}
          <TempInput bind:value={cutoffText} label={t("Core temperature at 2.5 h (°C)")} />
        {/if}
      {:else if kind === "hot_hold"}
        <label class="field"><span>{t("Food")}</span><TextInput bind:value={food} block /></label>
        <TempInput bind:value={tempText} label={t("Core temperature (°C)")} />
      {:else if kind === "batch_label"}
        <label class="field"><span>{t("Item")}</span><TextInput bind:value={product} block /></label>
        <label class="field">
          <span>{t("Made on")}</span>
          <DatePicker value={madeOn} onchange={(d) => (madeOn = d)} block ariaLabel="Made on" />
        </label>
        <TempInput bind:value={shelfLife} label={t("Shelf life (days)")} placeholder="5" mode="numeric" />
        {#if useBy}<strong>{t("Use by {date}", { date: longDate(useBy) })}</strong>{/if}
      {:else if kind === "cleaning"}
        <label class="field">
          <span>{t("Task")}</span>
          <SelectDropdown
            value={task}
            options={(lists?.cleaning_tasks ?? []).map((c) => ({ value: c.task, label: c.task }))}
            onchange={(v) => (task = v)}
            title="Cleaning task"
          />
        </label>
      {/if}

      {#if kind === "transport"}
        <label class="field"><span>{t("Food")}</span><TextInput bind:value={product} block /></label>
        <label class="field"><span>{t("Customer")}</span><TextInput bind:value={customer} block placeholder={t("Name or order reference")} /></label>
        <SegmentedControl
          block
          ariaLabel="Food kind"
          value={deliveryKind}
          onchange={(v) => (deliveryKind = v)}
          options={[
            { value: "chilled", label: t("Chilled") },
            { value: "frozen", label: t("Frozen") },
            { value: "hot", label: t("Hot") },
            { value: "ambient", label: t("Ambient") },
          ]}
        />
        {#if deliveryKind !== "ambient"}
          <TempInput bind:value={tempText} label={t("Temperature on arrival (°C)")} />
        {/if}
        <div class="field">
          <span>{t("Raw and ready-to-eat food kept apart?")}</span>
          <SegmentedControl block ariaLabel="Raw and ready-to-eat food kept apart" value={separated} onchange={(v) => (separated = v as typeof separated)} options={[{ value: "yes", label: t("Yes") }, { value: "no", label: t("No") }]} />
        </div>
        <label class="field"><span>{t("Batch code (optional)")}</span><TextInput bind:value={batchCode} block /></label>
        <div class="field">
          <span>{t("Use-by date (optional)")}</span>
          <div class="row">
            <DatePicker value={deliveryUseBy} onchange={(d) => (deliveryUseBy = d)} ariaLabel="Use-by date" block />
            {#if deliveryUseBy}<Button label={t("Clear")} onclick={() => (deliveryUseBy = null)} />{/if}
          </div>
        </div>
      {:else if kind === "training"}
        <label class="field"><span>Who was trained</span><TextInput bind:value={staffName} block /></label>
        <SegmentedControl block ariaLabel="Kind of training" value={trainingKind} onchange={(v) => (trainingKind = v)} options={[{ value: "in_house", label: "In-house" }, { value: "further", label: "Course or further training" }]} />
        <label class="field"><span>{t("Date")}</span><DatePicker value={onDate} onchange={(d) => (onDate = d)} ariaLabel="Date" block /></label>
        {#if trainingKind === "in_house"}
          <div class="field">
            <span>Topics covered</span>
            {#if topicList.length === 0}<span class="muted">The record books in use have no training topics.</span>{/if}
            {#each topicList as sec, si (si)}
              {#if sec.title}<strong class="small">{sec.title}</strong>{/if}
              {#each sec.items as it (it)}
                <Checkbox
                  checked={topics.includes(it)}
                  label={it}
                  onchange={(on) => (topics = on ? [...topics, it] : topics.filter((x) => x !== it))}
                />
              {/each}
            {/each}
          </div>
          <label class="field"><span>Trainer</span><TextInput bind:value={trainer} block /></label>
        {:else}
          <label class="field"><span>Course or training</span><TextInput bind:value={course} block placeholder="For example: food safety level 2" /></label>
          <label class="field"><span>Course provider</span><TextInput bind:value={provider} block /></label>
        {/if}
      {:else if kind === "fitness"}
        <label class="field"><span>Employee</span><TextInput bind:value={staffName} block /></label>
        <label class="field"><span>Date of assessment</span><DatePicker value={onDate} onchange={(d) => (onDate = d)} ariaLabel="Date of assessment" block /></label>
        <div class="field">
          <span>Reason for assessment</span>
          <SegmentedControl block ariaLabel="Reason for assessment" value={fitnessReason} onchange={(v) => (fitnessReason = v)} options={[{ value: "existing", label: "Existing" }, { value: "pre_employment", label: "Pre-employment" }, { value: "return_after_illness", label: "Return after illness" }]} />
        </div>
        <div class="field">
          <span>Outcome</span>
          <SegmentedControl block ariaLabel="Fit to work" value={fit} onchange={(v) => (fit = v as typeof fit)} options={[{ value: "yes", label: "Fit to work" }, { value: "no", label: "Not fit to work" }]} />
        </div>
      {/if}

      {#if showWhen}
        <div class="field">
          <span>{t("When was this done?")}</span>
          <SegmentedControl
            block
            ariaLabel="When was this done?"
            value={when}
            onchange={(v) => (when = v as typeof when)}
            options={correcting
              ? [{ value: "now", label: "Same time as the original" }, { value: "earlier", label: "A different time" }]
              : [{ value: "now", label: t("Just now") }, { value: "earlier", label: t("Earlier") }]}
          />
        </div>
        {#if earlier}
          <div class="two">
            <label class="field"><span>{t("Date")}</span><DatePicker value={whenDate} onchange={(d) => (whenDate = d)} ariaLabel="Date the check was done" block /></label>
            <label class="field"><span>{t("Time")}</span><TextInput bind:value={whenTime} block placeholder="HH:MM" ariaLabel="Time the check was done" /></label>
          </div>
          {#if takenAt === "bad"}<Notice tone="info" message={t("Enter the time the check was done, such as 09:00.")} />{/if}
          <label class="field"><span>{t("Done by (if not you)")}</span><TextInput bind:value={doneBy} block placeholder={t("Name or initials")} /></label>
        {/if}
      {/if}

      {#if kind === "fitness"}
        <p class="muted">Kishtin keeps only the outcome: who, when, why, fit or not, and the action taken. Keep the signed questionnaire on paper. Only managers can see these records.</p>
      {:else}
        <label class="field"><span>{t("Notes (optional)")}</span><TextInput bind:value={notes} block /></label>
      {/if}
      {#if correcting}
        <label class="field">
          <span>Why the original was wrong (required)</span>
          <TextInput bind:value={correctionReason} block placeholder="For example: read the wrong fridge" />
        </label>
      {/if}
  </div>

  {#if verdict}
    <Notice tone={verdictTone} toneLabel={verdictTitle} message={verdict.result.reason + (limitSource ? ` ${limitSource}.` : "")} />
  {:else if "missing" in draft}
    <Notice tone="info" message={tAny(draft.missing)} />
  {:else if canKeep}
    <Notice
      tone="warning"
      title={t("Not connected to Kishtin")}
      message={t("You can keep this check on this device. It is sent to Kishtin when the connection is back and you are logged in here. Until then it is not a record, and Kishtin has not checked it against the limit.")}
    />
    <div class="field">
      <span>{t("If the reading is out of limit, what did you do? (Kishtin asks again if it needs this)")}</span>
      <textarea class="text-area" bind:value={action} aria-label="Corrective action, if out of limit" placeholder={t("Leave empty if the reading is fine")}></textarea>
    </div>
  {:else if verdictError}
    <Notice tone="error" message={verdictError} />
  {/if}

  {#if needsAction}
    <div class="field">
      <span>{kind === "fitness" ? "Action taken (required)" : t("Corrective action (required)")}</span>
      {#if presets.length > 0}
        <div class="row">
          {#each presets as p (p.text)}
            <Chip interactive uppercase={false} tone={action === p.text ? "info" : "neutral"} onclick={() => (action = p.text)}>{p.text}</Chip>
          {/each}
        </div>
      {/if}
      <textarea class="text-area" bind:value={action} aria-label={t("Corrective action")} placeholder={t("What did you do about it?")}></textarea>
    </div>
  {/if}

  <span class="muted">{t("Logging as {name}.", { name: session.user?.name ?? "" })}</span>
  {#if canKeep}
    <Button tone="info" surface="solid" label={t("Keep on this device")} onclick={keepHere} />
  {:else}
    <Button tone="success" surface="solid" label={saving ? t("Saving…") : correcting ? "Save correction" : t("Save record")} disabled={!canSave} onclick={save} />
  {/if}
</div>

<style>
  .form {
    padding: var(--space-5) var(--space-6) var(--space-7);
  }
  .kinds {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: var(--space-4);
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-5);
  }
</style>
