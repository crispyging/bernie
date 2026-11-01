// The facts of the host on the Setup tab: the numbers and why they matter.
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

/** paintSetupFacts draws the plan of the host and the facts under it. */
async function paintSetupFacts(doc) {
  if (!setupState.hero) return;
  const facts = await setupFactList(doc);
  const sentence = setupPlanSentence();
  put(setupState.hero,
    h("div.setup-plan",
      h("p", "bernie picked the setup that fits this host."),
      h("p.small.muted", "Anything that needs a password is shown as a command to paste."),
      sentence ? h("p.small.muted", sentence) : null),
    facts.length ? h("dl.facts", facts.map(setupFact)) : null);
}

/** setupFactList is the facts of the host. The onboarding document carries
 *  them; the hardware and the doctor report fill them until it does. */
async function setupFactList(doc) {
  if (Array.isArray(doc.facts) && doc.facts.length) return doc.facts;
  const hw = await setupLoadHW();
  const doctor = setupState.doctor || await setupLoadDoctorReport();
  return setupFactsFromHost(hw, doctor);
}

/** setupFact is one fact: its name, its value and the reason under it. */
function setupFact(fact) {
  const row = factRow(fact.label || fact.id || "", fact.value, fact.why);
  row.classList.add("setup-fact");
  return row;
}

/** setupFactsFromHost reads the facts of the host from the hardware and the
 *  doctor report. */
function setupFactsFromHost(hw, doctor) {
  const facts = [];
  if (hw) {
    const gpus = hw.gpus || [];
    const names = {};
    for (const gpu of gpus) {
      const name = gpu.name || gpu.gfx || "GPU";
      names[name] = (names[name] || 0) + 1;
    }
    const archs = Array.from(new Set(gpus.map((g) => g.gfx).filter(Boolean)));
    const value = gpus.length
      ? Object.keys(names).map((name) => names[name] + " x " + name).join(", ")
      : "–";
    let why = "No AMD GPU is visible to bernie.";
    if (gpus.length === 1 && archs.length === 1) why = "one recipe family fits them all";
    if (gpus.length > 1 && archs.length === 1) why = "every card is " + archs[0] + ", so one recipe family fits them all";
    if (gpus.length && archs.length > 1) why = "the cards use " + archs.length + " architectures";
    facts.push({ id: "gpus", label: "GPUs", value: value, why: why });

    const ram = hw.host && hw.host.ram_bytes;
    facts.push({ id: "memory", label: "Memory", value: ram ? bytes(ram) : "–", why: setupMemoryWhy(hw) });

    const engines = (hw.host && hw.host.engines) || [];
    const usable = engines.filter((e) => e.usable);
    const engine = usable[0] || engines[0];
    facts.push({
      id: "engine",
      label: "Engine",
      value: engine ? (engine.name + (engine.version ? " " + engine.version : "")) : "–",
      why: usable.length ? "the image is used" : "the server runs on the host",
    });
  }
  if (doctor) {
    const rocm = (doctor.checks || []).find((c) => c.category === "rocm");
    facts.push({
      id: "rocm",
      label: "ROCm",
      value: rocm ? rocm.status : "–",
      why: rocm ? (rocm.detail || rocm.title) : "The doctor could not read the host.",
    });
    const virt = (doctor.checks || []).find((c) => c.id === "os.virtualization");
    if (virt) {
      facts.push({
        id: "virtualization",
        label: "Host",
        value: virt.status === "ok" ? "bare metal" : (virt.status || "–"),
        why: virt.detail || virt.title,
      });
    }
  }
  return facts;
}

/** setupMemoryWhy says what the memory of the host is used for. */
function setupMemoryWhy(hw) {
  const gpus = (hw && hw.gpus) || [];
  const vram = gpus.reduce((sum, gpu) => sum + (gpu.vram_bytes || 0), 0);
  if (!vram) return "";
  const perCard = Math.round(vram / gpus.length / (1024 * 1024 * 1024));
  return perCard + " GiB per card, the rest is the KV cache";
}

/** setupPlanSentence names the recipe that fits this host. */
function setupPlanSentence() {
  const recipes = (setupState.recipes && setupState.recipes.recipes) || [];
  const best = recipes.find((r) => r.fit && r.fit.status === "fits");
  if (!best) return "";
  const gpus = (setupState.hw && setupState.hw.gpus) || [];
  const archs = Array.from(new Set(gpus.map((g) => g.gfx).filter(Boolean)));
  let cards = "";
  if (gpus.length && archs.length) cards = gpus.length + " cards of " + archs.join(", ");
  else if (best.gpus) cards = best.gpus + " cards";
  return "The recipe is " + (best.name || best.id) + (cards ? " with " + cards : "") + ".";
}

/** setupLoadHW reads the hardware of the host once. */
async function setupLoadHW() {
  if (setupState.hw) return setupState.hw;
  try {
    setupState.hw = await api("/api/hw");
  } catch (e) {
    setupState.hw = null;
  }
  return setupState.hw;
}

/** setupLoadDoctorReport reads the doctor report once. */
async function setupLoadDoctorReport() {
  if (setupState.doctor) return setupState.doctor;
  try {
    setupState.doctor = await api("/api/doctor");
  } catch (e) {
    setupState.doctor = null;
  }
  return setupState.doctor;
}
