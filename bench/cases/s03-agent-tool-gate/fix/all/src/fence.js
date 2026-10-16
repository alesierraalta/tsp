export function fence(text) {
  const safe = text.replace(/<\s*\/\s*untrusted\s*>/gi, "<\\/untrusted>");
  return `<untrusted>\n${safe}\n</untrusted>`;
}
