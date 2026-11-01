// Setup host summary. Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

async function paintSetupFacts(doc) {
  const facts = await setupFactList(doc);
  if (!setupState.hero || setupState.doc !== doc) return;
  const summary = facts.filter((f) => ["gpus", "memory"].includes(f.id))
    .map((f) => (f.label || f.id) + ": " + (f.value || "not reported")).join(" · ");
  put(setupState.hero,
    h("div.setup-plan", h("p", setupPlanSentence()),
      summary ? h("p.small.muted", summary) : null,
      h("p.small.muted", "Administrator commands need a terminal. Other setup steps run here, only when you start them.")),
    facts.length ? h("details.advanced.setup-host-details", h("summary", "Host details and prerequisites"), h("dl.facts", facts.map(setupFact))) : null);
}

/** Prefer onboarding's facts; probe hardware only when they are absent. */
async function setupFactList(doc) {
  if (Array.isArray(doc.facts) && doc.facts.length) return doc.facts;
  const hw = await setupLoadHW();
  const doctor = setupState.doctor || await setupLoadDoctorReport();
  return setupFactsFromHost(hw, doctor);
}

function setupFact(fact) {
  const row = factRow(fact.label || fact.id || "", fact.value, fact.why);
  row.classList.add("setup-fact");
  return row;
}

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
    const why = gpus.length ? "GPU architectures: " + (archs.join(", ") || "not reported") : "No AMD GPU is visible to bernie.";
    facts.push({ id: "gpus", label: "GPUs", value: value, why: why });

    const ram = hw.host && hw.host.ram_bytes;
    facts.push({ id: "memory", label: "Host RAM", value: ram ? bytes(ram) : "–", why: setupMemoryWhy(hw) });

    const engines = (hw.host && hw.host.engines) || [];
    const usable = engines.filter((e) => e.usable);
    const engine = usable[0] || engines[0];
    facts.push({
      id: "engine",
      label: "Engine",
      value: engine ? (engine.name + (engine.version ? " " + engine.version : "")) : "–",
      why: setupState.mode === "container" ? (usable.length ? "Available for container mode." : "No usable container engine was reported.") : "Host mode runs directly on this machine; a container engine is optional.",
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

function setupMemoryWhy(hw) {
  const gpus = (hw && hw.gpus) || [];
  if (!gpus.length) return "GPU capacity not reported; host RAM is separate from GPU VRAM.";
  const capacities = gpus.map((g) => g.vram_bytes).filter((n) => Number.isFinite(n) && n > 0);
  const low = Math.min(...capacities), high = Math.max(...capacities);
  return "GPU VRAM: " + (capacities.length ? bytes(low) + (low === high ? "" : "–" + bytes(high)) + " per GPU" : "not reported")
    + (capacities.length < gpus.length ? "; " + (gpus.length - capacities.length) + " GPU capacities unknown" : "")
    + ". Separate from host RAM, not pooled.";
}

function setupPlanSentence() {
  const recipes = (setupState.recipes && setupState.recipes.recipes) || [];
  const selected = recipes.find((r) => r.id === setupState.recipeID);
  if (!selected) return "Choose a recipe below to select the model and its serving settings.";
  return "Selected recipe: " + (selected.name || selected.id) + ". "
    + (setupState.mode === "container" ? "Run in a container on this host." : "Run directly on this host.");
}

async function setupLoadHW() {
  if (setupState.hw) return setupState.hw;
  try {
    setupState.hw = await api("/api/hw");
  } catch (e) {
    setupState.hw = null;
  }
  return setupState.hw;
}

async function setupLoadDoctorReport() {
  if (setupState.doctor) return setupState.doctor;
  try {
    setupState.doctor = await api("/api/doctor");
  } catch (e) {
    setupState.doctor = null;
  }
  return setupState.doctor;
}
