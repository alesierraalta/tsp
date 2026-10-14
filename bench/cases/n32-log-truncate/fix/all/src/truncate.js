function stripOneTrailingLineFeed(text) {
  return text.endsWith("\n") ? text.slice(0, -1) : text;
}

export function truncate(text, maxCharacters) {
  if (typeof text !== "string") throw new TypeError("text must be a string");
  if (!Number.isInteger(maxCharacters) || maxCharacters < 0) {
    throw new RangeError("maxCharacters must be a non-negative integer");
  }

  const content = stripOneTrailingLineFeed(text);
  if (maxCharacters === 0) return "";

  return Array.from(content).slice(0, maxCharacters).join("");
}
