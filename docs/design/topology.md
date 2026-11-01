# Dashboard v3 — the Topology tab

Part of the design of the dashboard. The main file is
[../design-dashboard.md](../design-dashboard.md); the components are in
[components.md](components.md).

The question of the tab: **can the cards copy to each other?**

The topology used to be a section of the Dashboard tab. It is a tab of its
own now: the route is `#topology`, the old route `#dashboard/topology` is
gone, and the files of the tab live in `dashboard/web/views/topology/` with
their own sheet `dashboard/web/views/topology/topology.css`.

Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).

## The order of the page

1. The state headline and the one primary action, `Check readiness`.
2. The section `THE LINK CLASSES`: the matrix of the pairs, the legend and
   the table of every pair.
3. The section `THE PCIE TREE`: one block per PCIe switch.
4. The section `THE READINESS CHECKLIST`: the verdict and the items with
   their fixes.
5. The section `THE BANDWIDTH`: what the host measured, or that it did not.
6. `ADVANCED`: the raw documents and the commands that make them.

## The widgets

| # | Widget | Data source | Refresh | Empty copy | Error copy |
|---|---|---|---|---|---|
| 1 | The state headline | `GET /api/topo` (the cards) and `GET /api/topo/check` (the summary) | on open and on `Check readiness` | `THIS HOST HAS TOO FEW CARDS TO COMPARE.` plus what a topology needs | `THE TOPOLOGY OF THIS HOST IS NOT KNOWN.` plus `Try again` |
| 2 | The matrix of the link classes | `GET /api/topo` → `nodes` and `pairs` | on open and on `Check readiness` | `There is no matrix to show.` plus the command `bernie topo` | the card says the endpoint that failed and offers `Try again` |
| 3 | The legend of the classes | the four classes of spec §4.2 | with the matrix | the legend is hidden when there is no matrix | the legend is hidden |
| 4 | The table of every pair | `GET /api/topo` → `pairs` | with the matrix | `There are no pairs to show.` | the same card as the matrix |
| 5 | The PCIe tree | `GET /api/topo` → `nodes` | with the matrix | `There is no PCIe tree to show.` plus what the tree is | the same card as the matrix |
| 6 | The readiness checklist | `GET /api/topo/check` → `checklist` and `summary` | on open and on `Check readiness` | `Nothing to check on this host.` | the card says the endpoint that failed and offers `Try again` |
| 7 | The fix of an item | the item: `fix`, `doc` | with the checklist | an item without a fix shows the cause only | the same |
| 8 | The bandwidth | `GET /api/topo` → `pairs[].gbps` | with the matrix | `The bandwidth of this host is not measured.` plus the command `bernie topo --bandwidth` | the same card as the matrix |
| 9 | The Advanced panel | `/api/topo`, `/api/topo/check` | on open | the panel holds the empty answer of the endpoint | the panel holds the error answer |

## The matrix and the legend

One cell per pair of cards. The cell carries the class of the path
(`PIX`, `PXB`, `PHB` or `SYS`) and a shape: a filled dot when the pair has a
KFD peer link (the cards copy to each other without the CPU), an open dot
when the copy is staged through the host. The word carries the meaning; the
colour only helps.

The legend names every class in one sentence:

| Class | What it means |
|---|---|
| `PIX` | both cards hang under the same PCIe switch: the fastest path |
| `PXB` | the path crosses more than one PCIe switch, but no root complex |
| `PHB` | the path crosses one root complex: the host bridge of the CPU |
| `SYS` | the path crosses the CPU interconnect: a direct copy is not reachable here |

## The readiness checklist

The checklist of `bernie topo --check` (spec §4.5). Every row carries the
status in words (`ok`, `warn`, `fail`, `skip`), the title, the cause, the fix
with its copy button and the link to the rule in the documentation.

The doctor runs the same checklist under the category `topo.p2p`, so the
badge of the tab in the rail counts the failed items of the doctor report
without another request.

## The check action

`Check readiness` is the one primary button of the tab. It reads
`GET /api/topo` and `GET /api/topo/check` again and repaints the tab. The
server holds the answer of the checklist for fifteen seconds, so a second
press soon after the first one shows the same answer; the tab never claims
to have measured more than it has.

## The honest bandwidth

The dashboard never measures the bandwidth of the cards: `bernie topo
--bandwidth` does, with `rocm-bandwidth-test`. The tab shows the GB/s of a
pair when a measurement found one and says that the bandwidth is not
measured when it did not, with the command that measures it. It never shows
a number it does not have.

## At 390 px

The bottom bar of the shell holds the eight tabs in a grid of four columns
and two rows; every entry is whole (89 × 44 px) and no word is cut off. The
matrix and the tree scroll inside their card, like every other table of the
page.
