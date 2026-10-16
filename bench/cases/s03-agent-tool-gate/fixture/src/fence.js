export function fence(text) {
  const safe = text.replaceAll("</untrusted>", "<\\/untrusted>");
  return `<untrusted>\n${safe}\n</untrusted>`;
}
