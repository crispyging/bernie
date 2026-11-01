# Writing bernie recipes

This document tells you how to write a recipe. The contract is
[spec.md §7](spec.md#7-recipes). If the two disagree, spec.md is correct.

Writing rules for this file: short sentences, one fact or one instruction per
sentence, active voice.

## 1. What a recipe is

A recipe is one YAML file. It describes one vLLM launch: the model, the
hardware it needs, the environment and the `vllm serve` flags.

- The file name stem is the recipe id. `flashnext-mtp2.yaml` has the id
  `flashnext-mtp2`. The directory does not matter. An id is unique across the
  whole tree: two files with the same stem in one tree are a load error that
  names both paths.
- An id uses lowercase letters, digits, `.`, `_` and `-`.
- A file whose id starts with `_` is a fragment. A fragment is a shared part.
  You cannot plan or serve a fragment. Other recipes use it through `extends`.
- bernie turns a recipe into a plan: `bernie recipe plan <id>`. The plan is
  the exact command and environment. `bernie serve <id>` starts the plan.

## 2. Where bernie finds recipes

bernie loads recipes in this order. A later file with the same id replaces
an earlier one.

1. The built-in recipes (embedded from `recipes/` in the source tree).
2. `<config>/recipes/` (default `~/.config/bernie/recipes/`).
3. Each directory in `$BERNIE_RECIPES` (`:`-separated).
4. The files given to `bernie recipe validate FILE...`.

Directories are read recursively: every `*.yaml` and `*.yml` file below them
is a recipe. Group your own recipes in sub-directories as you like. Inside one
directory tree an id may appear once. A file in a later directory replaces an
earlier recipe with the same id.

A file that does not load is listed by `recipe list` and `recipe validate`.
bernie does not drop it silently.

To change a built-in recipe, do not copy it. Write a new file that extends
it (see [§5](#5-extends-and-merge-rules)).

## 3. Schema

```yaml
schema: 1                       # required. Always 1.
extends: [_base, _attn-fa]      # optional. Merged left to right, then this file.
name: Qwen3.8 Flash-Next AWQ + MTP=2
description: >-
  One paragraph. What it is, why, and where it was validated.
status: validated               # validated | experimental | reference
priority: 40                    # higher wins among recipes that fit. Default 0.
tags: [qwen, moe, mtp]
entrypoint: api                 # cli = `vllm serve` (default). api = openai.api_server.

model:
  source: wtdcode/Qwen3.8-Flash-Next-AWQ-W4A16   # HF repo id or absolute path
  served_name: flash-next       # optional. Becomes --served-model-name.
  revision: main                # optional. Becomes --revision.

requires:
  arch: [gfx1030]               # gfx names or prefixes: [gfx103, gfx11] matches RDNA2 and RDNA3.
  gpus: 4                       # GPUs of a matching arch
  vram_gib: 28                  # per GPU
  ram_gib: 48                   # host RAM
  engine: vllm-rdna             # vllm | vllm-rdna (needs the fork's kernels)
  download_gb: 21               # optional. The model download in GB.
  kv_bytes_per_token: 78356     # optional. Measured KV cache of one token over all ranks.

inputs:                         # values the operator gives with --set NAME=VALUE
  PLE_QUANT_DIR:
    description: ples_int4 directory (has META.json)
    required: true
    path: true                  # a host path; mounted into the container
    default: ""

container:                      # omit = host mode only
  image: docker.io/blivioniag/vllm-rdna:v0.31.0-extras
  tunableop_profile: rocm7.14-rocblas5.5
  shm_size: 16g                 # default 16g
  pull: true                    # default true. `serve --no-pull` skips the pull.

env:                            # environment of the vLLM process
  VLLM_RDNA_AR: "1"
  FLASH_ATTENTION_TRITON_AMD_ENABLE: null   # null removes the variable

args:                           # vllm serve flags, without the leading --
  tensor-parallel-size: 4
  max-model-len: 262144
  enable-prefix-caching: true
  compilation-config: {mode: 3, cudagraph_mode: FULL_AND_PIECEWISE}

why:                            # optional; one short reason per setting
  env.VLLM_ROCM_USE_AITER: AITER has no RDNA2 kernels.
  args.max-num-seqs: Eight slots match the validated serving profile.
  tuning.tunableop_profile: The solver rows match this rocBLAS build.
```

### 3.1 Required fields

A recipe that is not a fragment needs these fields after the merge:

- `schema`, `name`, `status`, `model.source`.
- `requires.arch` and `requires.gpus`. `requires.arch` is a list of gfx names
  (`gfx1030`) or prefixes (`gfx103`, `gfx11`). A GPU matches when its gfx name
  starts with one of the entries. A recipe with several entries runs on any of
  those architectures. Validation rejects an entry that is not `gfx` followed
  by digits or letters.
- `container.image` when `container` is set.

`requires.gpus` must equal `tensor-parallel-size` x `pipeline-parallel-size`
x `data-parallel-size` when the recipe sets one of them.

### 3.2 Field rules

- Write environment values as strings: `"1"`, not `1`. Numbers and booleans
  are accepted and written as text.
- Write `args` keys in lowercase with `-`. Do not write `--`.
- Do not put `model`, `served-model-name` or `revision` in `args`. Use the
  `model` block.
- Unknown fields are errors. A typo such as `modle:` fails the load.

### 3.3 Container or host

- A recipe with `container.image` runs in host mode and in container mode.
- A recipe without `container` runs in host mode only. Use this when the
  published image lacks a feature that the recipe needs. `--mode container`
  then fails with a clear message.
- The default mode is the config `mode`. When it is empty, bernie uses
  container mode if the recipe has an image and Docker or Podman is usable.
  Else it uses host mode.
- In host mode, bernie selects the TunableOp profile from the loaded
  `librocblas.so.5` and the architecture of the selected GPUs. In container
  mode, it uses `container.tunableop_profile`.
- In host mode, `VLLM_TUNED_CONFIG_FOLDER` (from `_tuned-moe`) points to the
  embedded tuned-MoE folder of the selected GPUs' architecture. An architecture
  without a folder gets no variable.

### 3.4 Why a setting

Use `why` to explain a setting that an operator may want to change. The key
is the setting path:

```yaml
why:
  env.VLLM_ROCM_USE_AITER: AITER has no RDNA2 kernels.
  args.max-num-seqs: Eight slots match the validated serving profile.
  tuning.tunableop_profile: The solver rows match this rocBLAS build.
```

- `env.NAME` names an environment variable.
- `args.FLAG` names a `vllm serve` flag without `--`.
- `tuning.tunableop_profile` names the TunableOp profile choice.
- Write one short, non-empty line. State the reason, not the value.
- An `env` or `args` reason must name a setting in the merged recipe.
- An override changes the value. The reason stays until a later layer
  replaces it or removes it with `null`.

`recipe plan` prints each reason under its setting. Its JSON adds a
`settings` array with `setting`, `value` and `why`. The existing `env` and
`args` fields do not change. A recipe without `why` keeps the same plan JSON.

## 4. Arguments

bernie renders `args` in key order:

| Value | Flag |
|---|---|
| `true` | `--key` |
| `false` or `null` | nothing |
| a map or a list | `--key '<compact JSON>'` |
| another value | `--key VALUE` |

Example: `speculative-config: {method: mtp, num_speculative_tokens: 2}` gives
`--speculative-config '{"method":"mtp","num_speculative_tokens":2}'`.

## 5. Extends and merge rules

`extends` lists fragments or recipes. bernie merges the parents depth first,
left to right, then the file itself. A cycle or a missing parent is an error.

Merge rules:

- Maps merge key by key. A nested map such as `compilation-config` merges too.
- The `why` map merges key by key. A later layer replaces the reason for the
  same setting path. Use `null` to remove an inherited reason.
- Lists replace. `tags: [a]` in the child replaces the parent's tags.
- Scalars replace.
- `null` deletes the key. Under `env`, `null` also removes the variable from
  the inherited environment (and from the image environment in container
  mode).

Example: run `flashnext` with 4 sequences and no tool parser.

```yaml
# ~/.config/bernie/recipes/flashnext-small.yaml
schema: 1
extends: [flashnext]
name: Flash-Next, 4 sequences, no tools
status: experimental
args:
  max-num-seqs: 4
  enable-auto-tool-choice: null
  tool-call-parser: null
```

## 6. Inputs and interpolation

- `${NAME}` in `model`, `env` and `args` strings is replaced by the value of
  input `NAME`.
- Every `${NAME}` must be a declared input. An unknown name fails validation.
- `$${` writes a literal `${`.
- The value comes from `--set NAME=VALUE`, then from `default`.
- A `required` input without a value stops the plan. The message names the
  `--set` to give.
- A `path` input is a host path. bernie warns when it does not exist. In
  container mode, bernie mounts it at the same path.

## 7. Overrides

`--set KEY=VALUE` changes one value at plan time. It can be repeated.

- A key without a dot sets an input: `--set PLE_QUANT_DIR=/data/ples_int4`.
  The value is a plain string.
- A dotted key sets a field: `--set args.max-model-len=32768`,
  `--set env.GPU_MAX_HW_QUEUES=2`, `--set args.enforce-eager=true`. The
  value is parsed as YAML. `null` deletes the key.
- `set:` in `config.yaml` holds default overrides. bernie applies them before
  the `--set` flags. A later override wins.
- Change the GPUs with `--set env.HIP_VISIBLE_DEVICES=4,5,6,7`. Otherwise
  bernie selects them (spec §7.7): matching arch, one PCIe switch, lowest
  HIP indices.

## 8. Check your recipe

```sh
bernie recipe validate ~/.config/bernie/recipes/flashnext-small.yaml
bernie recipe show flashnext-small        # the merged recipe
bernie recipe plan flashnext-small        # env + command, GPU selection, warnings
bernie recipe plan flashnext-small --json # the plan for scripts
```

## 9. Layout of the built-in tree

```text
recipes/
  _fragments/<concern>/_*.yaml   shared parts: base, attention, comm, ple,
                                 speculative, tuning, arch
  flashnext/                     Qwen3.8 Flash-Next family (+ _flashnext-common)
  qwen-dense/                    Qwen3.8-27B dense family
  deepseek/                      DeepSeek family
```

- A family folder holds the recipes of that family. A family that grows past
  about 15 recipes splits by a real axis (for example `flashnext/pp/`).
- A fragment lives in `_fragments/<concern>/`, or next to its family when only
  that family uses it (`flashnext/_flashnext-common.yaml`).
- `extends` names ids, never paths. Moving a file does not change any
  `extends`.
- A setting that exists only for one GPU architecture goes in
  `_fragments/arch/`. A setting that is true on every architecture stays in
  the concern fragment. The `_rdna2` fragment holds the RDNA2 kernel switches.
  There is no RDNA3 fragment yet: add one only with settings you have
  validated on RDNA3 hardware.
- To add a family, create the directory. No Go code lists the directories.

## 10. Built-in fragments

| Fragment | Path under `recipes/` | Content |
|---|---|---|
| `_base` | `_fragments/base/` | Shared environment and fixed flags of the fork launcher (`rdna_init`): spawn workers, no AITER, no hipBLASLt, FP16, TP4, 8 sequences, 2048 batched tokens, 127.0.0.1:18094. Declares `requires.arch: [gfx1030]`. Sets the `v0.31.0-extras` image and the `rocm7.14-rocblas5.5` TunableOp profile. |
| `_rdna2` | `_fragments/arch/` | RDNA2-only kernel switches: the fork's causal-conv1d forward and update kernels, off. |
| `_attn-fa` | `_fragments/attention/` | FA-RDNA2 attention: `RDNA_ATTN` backend, GQA decode. Removes `FLASH_ATTENTION_TRITON_AMD_ENABLE`. |
| `_attn-triton` | `_fragments/attention/` | Triton attention (`TRITON_ATTN`), FA-RDNA2 off. |
| `_rccl-pcie` | `_fragments/comm/` | RCCL over PCIe: P2P level `pxb`, Simple protocol, no MSCCL, fine-grained PCIe memory. |
| `_rdna-ar` | `_fragments/comm/` | The fork's one-shot all-reduce up to 64 KiB, RCCL above. |
| `_tuned-moe` | `_fragments/tuning/` | `VLLM_TUNED_CONFIG_FOLDER` for the tuned fused-MoE configs. In container mode it is `/src/vllm/tuned-moe` in the image. In host mode bernie writes the embedded folder of the selected GPUs' architecture to `<data>/tuned-moe/<arch>` and uses that directory. |
| `_ple-int4` | `_fragments/ple/` | Flash-Next PLE n-gram table, INT4, in pinned host RAM. Needs input `PLE_QUANT_DIR`. |
| `_ple-int4-disk` | `_fragments/ple/` | The INT4 table memory-mapped from disk through the CPU sidecar (no RAM floor). |
| `_ple-exl3` | `_fragments/ple/` | The PLE table from an EXL3 checkpoint through the CPU sidecar. Removes `PLE_QUANT_DIR`. |
| `_mtp2` | `_fragments/speculative/` | MTP speculative decode with 2 tokens and capture sizes 3, 6, 12, 24. |
| `_flashnext-common` | `flashnext/` | The shared Flash-Next AWQ-W4A16 profile on 4x gfx1030 (TP4/EP4). |

## 11. Built-in recipes

The 11 fork recipes come from `tools/rdna/recipes/*.env` of the
`opengfx1030/vllm-rdna` fork. The single-GPU Qwen profile is derived from the
`27b-awq` fork recipe. The fork comments are kept in the files. All 12
declare `requires.arch: [gfx1030]`: the fork kernels are validated on
gfx1030 only. `bernie recipe plan` on another architecture refuses with the
arch it needs.

| Recipe | Path under `recipes/` | GPUs | Modes | What it is |
|---|---|---|---|---|
| `flashnext-mtp2` | `flashnext/` | 4 | host, container | Flash-Next AWQ-W4A16, MTP=2, TP4/EP4. Wins at 8x16k/1k c=8. Validated. |
| `flashnext-mtp0` | `flashnext/` | 4 | host, container | Flash-Next AWQ-W4A16, plain decode. Wins at 8x1k/512 c=8. Validated. |
| `flashnext` | `flashnext/` | 4 | host, container | Flash-Next production agentic serve: tool calling, reasoning parser, 6 sequences. Validated. |
| `flashnext-pp3` | `flashnext/` | 3 | host | Flash-Next over 3 cards, TP1/PP3, layers 16,16,16, dense INT8 only, text only. Validated. |
| `flashnext-pp3-mtp2` | `flashnext/` | 3 | host | `flashnext-pp3` with a W4A16 MTP drafter (input `MODEL`), layers 17,17,14. Experimental. |
| `flashnext-dflash2` | `flashnext/` | 4 | host | Flash-Next with the incoai DFlash2 drafter, 3 speculative tokens. Experimental. |
| `flashnext-exl3` | `flashnext/` | 4 | host | Flash-Next EXL3 3.05 bpw with the PLE table from the checkpoint. Experimental. |
| `27b-awq` | `qwen-dense/` | 4 | host, container | Qwen3.8-27B AWQ-INT4 dense, W4A8 prefill, TP4. Validated. |
| `27b-awq-1gpu` | `qwen-dense/` | 1 | host, container | Qwen3.8-27B AWQ-INT4 dense, W4A16, 32K context on one V620. Validated. |
| `27b-exl3` | `qwen-dense/` | 4 | host, container | Qwen3.8-27B EXL3 3.00 bpw, TP4, 20K context. Validated. |
| `full` | `qwen-dense/` | 2 | host, container | Qwen3.8-27B AWQ-INT4, full HIP path, TP2. Validated. |
| `dsv4-flash` | `deepseek/` | 4 | host, container | DeepSeek-V4-Flash K128 (fp8 + MXFP4 experts), TP4. Needs input `MODEL`. Experimental. |

`bernie recipe list` shows the fit of each recipe on this host: `fits`,
`tight: <reason>`, `no: <reason>` or `unknown`. A `tight` recipe runs after
one change: free disk for the download, or a shorter `max-model-len`.

To measure `kv_bytes_per_token`, start the recipe once with a fixed
`kv-cache-memory-bytes`. vLLM logs the tokens the KV cache holds. Divide the
reserve times the ranks of a replica by that number. Write down the date,
the host and the numbers in a comment next to the value. Leave the field
out when you have not measured it.

## 12. Differences from the fork launcher

`bernie recipe plan` gives the same vLLM flags and kernel environment as
`PRINT=1 bash tools/rdna/serve_rdna.sh RECIPE=<id>`. These differences are on
purpose:

- GPUs: bernie selects GPUs of the recipe's arch on one PCIe switch. The
  fork uses `0,1,2,3` by default.
- Caches: bernie uses `<cache>/runs/<name>/` per run. The fork uses
  `<tree>/cache/`.
- `PYTHONPATH`: bernie does not set it. The venv has vLLM installed. The run
  directory is the working directory.
- `HF_HUB_OFFLINE` and `TRANSFORMERS_OFFLINE`: `0` for a repo id (downloads
  allowed), `1` for a local path. The fork always sets `1`. Set
  `env.HF_HUB_OFFLINE` in a recipe or with `--set` to force a value.
- TunableOp: the plan shows the `PYTORCH_TUNABLEOP_*` variables. The fork
  sets the same variables when it starts, but its `PRINT=1` output omits them.
- `VLLM_PP_LAYER_PARTITION`: the plan shows it. The fork exports it but its
  `PRINT=1` output omits it.
- `VLLM_TUNED_CONFIG_FOLDER`: in host mode it points to bernie's copy of the
  same files for the selected GPUs' architecture, not to the fork tree.
- `ROCM_HOME`, `ROCM_PATH`, `HIP_PATH`, `HIP_ROOT_DIR`: bernie uses the
  ROCm install the host runs, as a resolved path: the tree whose tools win
  on `PATH`, else the newest install that has a `hipcc`. A tree nested in
  another one counts (a TheRock tree inside a package install is the one the
  operator runs).

## 13. Recipes for another GPU architecture

A recipe runs on the architectures in `requires.arch`. To run a recipe on a
new architecture:

1. Add the architecture to the device tables. See
   [extending.md](extending.md#2-how-to-add-a-new-gpu-generation).
2. Write a new recipe that extends the old one. Set its own `requires.arch`.
   Do not edit the validated recipe.
3. Put settings that hold only on that architecture in a fragment under
   `recipes/_fragments/arch/`.
4. Use `status: experimental`. Change it to `validated` only with a
   `bernie bench --json` result from real hardware.

`bernie doctor` reads the validated architectures from the recipes
(`gpu.supported.<gfx>`). A new `validated` recipe makes the architecture
`ok`. No other file lists it.

## 14. The visible GPUs

`HIP_VISIBLE_DEVICES` and `ROCR_VISIBLE_DEVICES` limit the GPUs bernie
uses. When one of them is set in the environment, the fit of a recipe, the
GPU selection and the plan see only the GPUs it names (spec §7.7). A recipe
that needs four GPUs does not fit a host where the environment leaves one
GPU visible, and the plan of a one-GPU recipe picks the visible GPU.

The value is a comma list of HIP indices. A UUID is not a list of indices:
bernie ignores it and says so.
