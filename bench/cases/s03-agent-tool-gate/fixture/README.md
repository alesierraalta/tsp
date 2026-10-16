# agent-tools

The tool layer of an LLM assistant that reads untrusted documents. Run `node --test`.

**`ToolRunner`** runs the tool calls the model proposes: `{ name, args }`. A read-only tool runs directly.
A tool declared with `sideEffect: true` runs only after `confirm(call)` returned `true` **for that exact
call**: the same tool with the same arguments. An approval never covers a call with different arguments,
and a declined call returns `{ error: "declined" }` without running. An unknown tool returns
`{ error: "unknown tool <name>" }`.

**`fence(text)`** wraps untrusted text (retrieved documents, tool output) before it goes into the prompt:
`<untrusted>` + newline + text + newline + `</untrusted>`. The model treats the fence tags
case-insensitively and ignores whitespace inside them, so the wrapped text must never contain anything
that would read as the closing tag: in the fenced output, the only closing tag is the final one.
