// Shared command block and copy button.
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
// Every command of the page is text: it is never markup.
"use strict";

/** copyBtn is a copy button for a command that is shown inline: the fix of a
 *  finding, a flag, a URL. */
function copyBtn(text, label) {
  return h("button.small.copy", {
    type: "button",
    "data-copy": text,
    "aria-label": label || "Copy the command",
  }, "Copy");
}

/** cmdBlock shows a command with its copy button. The text is the command as
 *  it is run. */
function cmdBlock(text, label) {
  if (!text) return null;
  return h("div.cmd",
    h("code", text),
    h("button.copy", {
      type: "button",
      "data-copy": text,
      "aria-label": label || "Copy the command",
    }, "Copy"));
}

/** connectEndpoint is the one source of the address and the model of a run.
 * The Dashboard hero, its connect block and the Advanced run card read it. */
function connectEndpoint(run) {
  if (!run) return { base: "", url: "", model: "" };
  let raw = run.url || "";
  if (!raw && run.port) raw = runURL(run);
  raw = raw.replace(/\/v1\/?$/, "").replace(/\/$/, "");
  raw = raw.replace("//0.0.0.0:", "//127.0.0.1:").replace("//[::]:", "//127.0.0.1:");
  const plan = run.plan || {};
  const model = plan.served_name || plan.model || run.recipe || run.name || "";
  return { base: raw, url: raw ? raw + "/v1" : "", model: model };
}

/** connectSnippet is the copyable request for an OpenAI-compatible client. */
function connectSnippet(kind, endpoint) {
  if (!endpoint.base) return "Start a run to get an address.";
  if (kind === "python") {
    return "from openai import OpenAI\n" +
      "client = OpenAI(base_url=\"" + endpoint.base + "/v1\", api_key=\"not-used\")\n" +
      "r = client.chat.completions.create(\n" +
      "    model=\"" + endpoint.model + "\",\n" +
      "    messages=[{\"role\": \"user\", \"content\": \"Hello!\"}]\n" +
      ")\n" +
      "print(r.choices[0].message.content)";
  }
  return "curl " + endpoint.base + "/v1/chat/completions \\\n" +
    "  -H 'Content-Type: application/json' \\\n" +
    "  -d '{\"model\":\"" + endpoint.model + "\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello!\"}]}'";
}
