// A scripted model for the live proof: Pi's real runtime, the real tsp extension, no network and no cost.
// Each model request consumes the next step of PI_LIVE_SCRIPT (a JSON array):
//   { "text": "..." }                      answer with text and stop
//   { "tool": "write", "args": { ... } }   ask Pi to run a tool, then continue with the next step
//   { "hang": true }                       never answer; ends as aborted when Pi aborts the run
// A script that runs out answers "script exhausted", so a run the script did not plan is visible.
// When PI_LIVE_SEEN names a file, each request appends what reached the model last ({ role, text }): the only way to see a
// message an extension added at agent_before_settle, which Pi does not announce as a message event.
import { createAssistantMessageEventStream } from "@earendil-works/pi-ai";
import { appendFileSync } from "node:fs";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

type Step = { text: string } | { tool: string; args: Record<string, unknown> } | { hang: true };

export default function (pi: ExtensionAPI) {
  let steps: Step[];
  try {
    steps = JSON.parse(process.env.PI_LIVE_SCRIPT ?? "[]");
  } catch (error) {
    throw new Error(`PI_LIVE_SCRIPT is not valid JSON: ${error instanceof Error ? error.message : String(error)}`);
  }
  let next = 0;
  pi.registerProvider("scripted", {
    baseUrl: "http://127.0.0.1:1",
    apiKey: "scripted",
    api: "scripted-api",
    models: [{ id: "script", name: "script", reasoning: false, input: ["text"], cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }, contextWindow: 100000, maxTokens: 4096 }],
    streamSimple(model, context, options) {
      if (process.env.PI_LIVE_SEEN) {
        const last: any = context.messages.at(-1);
        const text = typeof last?.content === "string" ? last.content : String(last?.content?.[0]?.text ?? "");
        appendFileSync(process.env.PI_LIVE_SEEN, JSON.stringify({ role: last?.role ?? "", text: text.slice(0, 120) }) + "\n");
      }
      const stream = createAssistantMessageEventStream();
      const step: Step = steps[next++] ?? { text: "script exhausted" };
      const zero = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 };
      const output: any = {
        role: "assistant", content: [], api: model.api, provider: model.provider, model: model.id,
        usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: zero },
        stopReason: "stop", timestamp: Date.now(),
      };
      const finish = (reason: "stop" | "toolUse") => {
        output.stopReason = reason;
        stream.push({ type: "done", reason, message: output });
        stream.end();
      };
      queueMicrotask(() => {
        stream.push({ type: "start", partial: output });
        if ("hang" in step) {
          options?.signal?.addEventListener("abort", () => {
            output.stopReason = "aborted";
            output.errorMessage = "aborted";
            stream.push({ type: "error", reason: "aborted", error: output });
            stream.end();
          });
        } else if ("text" in step) {
          output.content.push({ type: "text", text: step.text });
          stream.push({ type: "text_start", contentIndex: 0, partial: output });
          stream.push({ type: "text_delta", contentIndex: 0, delta: step.text, partial: output });
          stream.push({ type: "text_end", contentIndex: 0, content: step.text, partial: output });
          finish("stop");
        } else {
          const block = { type: "toolCall", id: `call-${next}`, name: step.tool, arguments: step.args };
          output.content.push(block);
          stream.push({ type: "toolcall_start", contentIndex: 0, partial: output });
          stream.push({ type: "toolcall_end", contentIndex: 0, toolCall: block, partial: output });
          finish("toolUse");
        }
      });
      return stream;
    },
  });
}
