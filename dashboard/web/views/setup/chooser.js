// Alternative recipes, loaded when the chooser is opened.
"use strict";

function setupRecipeCard(recipe) {
  const input = h("input", {
    type: "radio",
    name: "setup-recipe",
    value: recipe.id,
    checked: recipe.id === setupState.recipeID,
    onchange: () => setupSelectRecipe(recipe.id),
  });
  const recommended = setupRecommendedRecipe() && setupRecommendedRecipe().id === recipe.id;
  return h("label.recipe-card", { class: "recipe-card" + (recipe.id === setupState.recipeID ? " picked" : "") },
    input,
    h("h3", recipe.name || recipe.id, recommended ? h("span.pill", "recommended") : null),
    recipe.description ? h("p.recipe-what", recipe.description) : null,
    h("div.recipe-meta.advanced-only",
      h("span.recipe-status", recipe.status || ""),
      h("span.recipe-need", recipe.gpus ? recipe.gpus + " GPUs" : "any number of GPUs"),
      h("span.recipe-arch", (recipe.arch || []).join(", ") || "any architecture")),
    h("p.recipe-fit", { class: "recipe-fit fit-" + setupFitStatus(recipe) }, setupFitLine(recipe)),
    recommended ? h("p.small.muted", "recommended: " + setupRecommendationWhy(recipe)) : null);
}
/** Keep the chosen radio and its card in sync, including the recommendation. */
function setupPaintRecipeList(box, recipes) {
  put(box, h("div.recipe-chooser", recipes.map(setupRecipeCard)));
  setupPaintRecipeSelection();
}

async function setupSelectRecipe(id) {
  setupState.recipeID = id;
  const recipe = (setupState.recipes && setupState.recipes.recipes || []).find((r) => r.id === id);
  const modes = (recipe && recipe.modes) || ["host"];
  setupState.mode = modes.includes(setupState.mode) ? setupState.mode : modes[0];
  setupState.sets = [];
  setupPaintRecipeSelection();
  await loadSetupPlan();
}
function setupPaintRecipeSelection() {
  const cards = setupState.recipeBox ? setupState.recipeBox.querySelectorAll(".recipe-card") : [];
  cards.forEach((card) => {
    const input = card.querySelector("input");
    const picked = input && input.value === setupState.recipeID;
    card.classList.toggle("picked", picked);
    if (input) input.checked = picked;
  });
}
