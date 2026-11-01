# Third-party material and licenses

bernie copies text only from Apache-2.0 sources. Everything else is linked.

| Source | License | Use in bernie |
|---|---|---|
| [opengfx1030/vllm-rdna](https://github.com/opengfx1030/vllm-rdna) (recipes, `tunableop/` rows) | Apache-2.0 | Recipes and TunableOp rows are ported and embedded. Keep the SPDX notice. |
| [opengfx1030/vllm-rdna-docker](https://github.com/opengfx1030/vllm-rdna-docker) | GPL-3.0 | Link only. bernie references the published images, not the Dockerfiles. |
| [BlivionIaG/v620_toolbox](https://github.com/BlivionIaG/v620_toolbox) | GPL-3.0 | Link only. bernie reads the public hwmon power range; it does not apply firmware or kernel changes. |
| [BlivionIaG/gfx1030-wiki](https://blivioniag.github.io/gfx1030-wiki/) | none stated | Link only, through the `doc` field of checks. No prose is copied. |
| [crispyging/vllm-rdna-dash](https://github.com/crispyging/vllm-rdna-dash) | Apache-2.0 | Web UI design and parts of the markup and CSS. The Dashboard tab follows its operational reading order — the serving hero, six plain-language tiles, request-rate charts, GPU cards and connect block — while the Advanced tab preserves bernie's first dashboard for comparison. The rail, tab bar, pills, command block, toast, tooltip and step timeline are adapted to the bernie tokens, API and accessibility rules. Markup, CSS and copy may be taken from it. Keep the notice in the header of each adapted file. |
| [Montserrat](https://fonts.google.com/specimen/Montserrat), [Inter](https://fonts.google.com/specimen/Inter), [JetBrains Mono](https://fonts.google.com/specimen/JetBrains+Mono) | SIL Open Font License 1.1 | The self-hosted fonts of the dashboard: the Latin subsets under `dashboard/web/fonts/` (128 KB, no CDN). The licence text of each family is next to the file: `OFL-montserrat.txt`, `OFL-inter.txt`, `OFL-jetbrainsmono.txt`. |
| [ashhart/TensorFold](https://github.com/ashhart/TensorFold) | Apache-2.0 | Ideas only: the suite as published data with a digest, the repetitions with medians, the noise band and the steady rule, the base/cand/cand/base order, and the shape of a pull request receipt. No code and no prose is copied. |
| [GeorgeMA-Strong/llm-context-bench](https://github.com/GeorgeMA-Strong/llm-context-bench) | MIT | Ideas only: the context-length ladder, the measurement gates that name why a number may not be a speed number, and the per-tier result table. No code, no prose and no bundled fixture text is copied. |
| [BlivionIaG/benchmark_suite](https://github.com/BlivionIaG/benchmark_suite) | none (no license file) | Ideas only: the recipe-driven shape of a benchmark run, the fixed manifest of a suite and the compare direction table. No code and no prose is copied. |
| [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) v1.8.0 | MIT and Apache-2.0 (the SDK's own license file) | Linked. The MCP server (`internal/mcp`) uses it. Modules it brings into the binary: `google/jsonschema-go` (MIT), `segmentio/encoding` and `segmentio/asm` (MIT), `yosida95/uritemplate` (BSD-3-Clause), `golang.org/x/oauth2`, `x/sync`, `x/sys` and `x/time` (BSD-3-Clause). |
| [vllm-project/vllm](https://github.com/vllm-project/vllm) | Apache-2.0 | Target engine. bernie runs it; it does not vendor it. |
| PCI id database, Linux kernel amdgpu/KFD sources, LLVM AMDGPU docs, ROCm docs | various | Facts only: PCI ids, product names, die-to-gfx maps, wave size. bernie states the facts in its own tables (`internal/machine/gpu/devices/*.yaml`). The header of each file links its sources. No text is copied. |

Rule for contributors: do not paste code or prose from a source marked
"link only". Re-express facts in your own words and link the source.
