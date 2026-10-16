# chat-render

Turns the assistant's markdown reply into HTML for the chat window. Run `node --test`.

`render(markdown)` supports `**bold**`, links `[text](url)`, images `![alt](url)` and line breaks; any other
text is shown as typed. The reply is model output and may repeat anything the model read.

**Promise:** the HTML is safe to insert into the chat window: nothing in the reply can run script, and
nothing makes the browser contact another site on its own, before the user clicks something.
