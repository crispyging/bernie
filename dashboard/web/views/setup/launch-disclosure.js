// Exact launch command and optional compose file; available in either mode.
"use strict";

function setupPlanLaunchDisclosure(plan) {
  const launch = h("div.recipe-plan-launch",
    h("h4", "The launch command"),
    cmdBlock(setupPlanCommand(plan), "Copy the launch command"));
  if (plan.mode !== "container" || !plan.compose) {
    return h("details.advanced.recipe-plan-details",
      h("summary", "Exact launch command"),
      launch);
  }
  const composeBox = h("div.recipe-plan-compose",
    h("h4", "The generated compose.yml"),
    cmdBlock(plan.compose, "Copy the compose file"),
    h("p.small.muted",
      "Save the file, then start it with the engine of the host: ",
      h("code", "docker compose -f FILE up -d"),
      " or ",
      h("code", "podman compose -f FILE up -d"),
      "."));
  return h("details.advanced.recipe-plan-details",
    h("summary", "Exact launch command and compose file"),
    launch,
    composeBox);
}
