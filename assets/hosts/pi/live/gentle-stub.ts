// Stands in for gentle-pi in the live proof: registers one tool under a Gentle-owned name, which is all the tsp
// extension reads to observe a Gentle session (observesGentleSession). It does nothing when called.
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

export default function (pi: ExtensionAPI) {
  pi.registerTool({
    name: "gentle_odd_phase",
    label: "gentle_odd_phase",
    description: "Live-proof stand-in for the gentle-pi tool of the same name.",
    parameters: { type: "object", properties: {} } as never,
    execute: async () => ({ content: [{ type: "text", text: "ok" }], details: {} }),
  });
}
