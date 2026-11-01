"use strict";
// Keep blocking results and warnings visible; successful checks are evidence on demand.
function advDoctorReport() {
  const card = h("div.card");
  return {
    el: card,
    update: (report) => {
      const checks = (report && report.checks) || [];
      if (!checks.length) {
        put(card, cardHead("Health checks", "/api/doctor"),
          emptyState("Host health has not been established.",
            "Re-check the host to read its health checks. An unavailable or empty report is not a pass.", "bernie doctor"),
          adv.errors && adv.errors["/api/doctor"] ? h("p.err", adv.errors["/api/doctor"]) : null);
        return;
      }
      const labels = { os: "Operating system", gpu: "Graphics cards", kernel: "System driver",
        "topo.p2p": "Multi-GPU copies", rocm: "GPU runtime", python: "Python environment",
        container: "Container support", resources: "Memory and storage", tuning: "Performance settings" };
      const groups = new Map();
      for (const check of checks) {
        const cat = check.category || "other";
        if (!groups.has(cat)) groups.set(cat, []);
        groups.get(cat).push(check);
      }
      put(card, cardHead("Health checks", "/api/doctor"),
        h("div.row", [["fail", "blocking"], ["warn", "warnings"], ["ok", "passed"], ["skip", "not checked"]]
          .map(([s, label]) => statePill(s, checks.filter((c) => c.status === s).length + " " + label))),
        h("p.small.muted", "Resolve blocking checks first, review warnings, then re-check. Not checked means unknown, not healthy."),
        [...groups.entries()].map(([cat, list]) => {
          const passed = list.filter((c) => c.status === "ok");
          return h("div.adv-doctor-group", h("h3.adv-doctor-cat", labels[cat] || cat),
            list.filter((c) => c.status !== "ok").map(advDoctorCheck),
            passed.length ? h("details.advanced", h("summary", passed.length + " passed checks"),
              h("div.adv-doctor-list", passed.map(advDoctorCheck))) : null);
        }));
    },
  };
}

// Commands are copied, never run as root by the dashboard.
function advToolsCard() {
  const card = h("div.card");
  return {
    el: card,
    update: (report) => {
      const checks = (report && report.checks) || [];
      const power = checks.find((c) => c.id === "tuning.power");
      put(card, cardHead("Memory protection and power tools", "/api/doctor"),
        h("p.small", "These are manual instructions, not actions run by this page. bernie never runs commands as root."),
        !report ? h("p.err", "Health checks are unavailable. Re-check the host before deciding which settings to change.") : null,
        h("div.adv-doctor-group", h("h3.adv-doctor-cat", "GPU memory protection (ECC)"),
          h("p.small", "Changing ECC can reduce memory-error protection and requires kernel changes and reboots. Review the health checks and instructions first."),
          h("details.advanced", h("summary", "Read memory counters and change ECC"),
            cmdBlock("cat /sys/class/drm/card*/device/ras/umc_err_count", "Copy the read command"),
            h("p.small.muted", "Turn ECC off with amdgpu.ras_enable=0 in the kernel command line, then reboot twice; turn it back on with amdgpu.ras_enable=1. bernie never changes the ECC state."),
            h("a.small", { href: "https://blivioniag.github.io/gfx1030-wiki/tuning/ecc.html", target: "_blank", rel: "noopener noreferrer" }, "Read ECC instructions"))),
        h("div.adv-doctor-group", h("h3.adv-doctor-cat", "Power limit commands"),
          h("p.small", "Changing limits affects power draw, heat and performance. Use the Power limits controls for a deliberate change; the commands below may require root."),
          h("details.advanced", h("summary", "Read and apply power limits in a terminal"),
            power && power.fix ? cmdBlock(power.fix, "Copy the recommended command") : null,
            cmdBlock("cat /sys/class/drm/card*/device/hwmon/hwmon*/power1_cap", "Copy the read command"),
            cmdBlock("sudo bernie tune power apply", "Copy the command"),
            h("p.small.muted", "bernie tune power set saves a desired cap without root; applying it needs root. A kernel with the 250 W VBIOS floor refuses a lower cap until the powerfix is applied."),
            h("a.small", { href: "https://github.com/blivioniag/v620_toolbox", target: "_blank", rel: "noopener noreferrer" }, "Read powerfix instructions"))));
    },
  };
}

function advDoctorCheck(check) {
  const status = check.status || "skip";
  const word = { fail: "Blocking", warn: "Warning", ok: "Passed", skip: "Not checked" }[status] || "Unknown";
  return h("div.finding.doctor-finding",
    h("div.doctor-finding-head", statePill(status, word),
      h("span.doctor-finding-id.advanced-only", check.id || "")),
    h("p.doctor-finding-cause", check.title || "Check result"),
    check.detail ? h("p.small.muted", check.detail) : null,
    check.fix ? h("div", h("b.small", "Next step"), cmdBlock(check.fix, "Copy the fix command")) : null,
    check.doc ? h("a.small", { href: check.doc, target: "_blank", rel: "noopener noreferrer" }, "Read the explanation") : null);
}
