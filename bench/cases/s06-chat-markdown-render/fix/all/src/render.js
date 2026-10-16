const escape = (s) =>
  s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

export function render(markdown) {
  let html = escape(markdown);
  html = html.replace(/!\[([^\]]*)\]\(([^)\s]+)\)/g, (_, alt, url) =>
    url.startsWith("/") && !url.startsWith("//") ? `<img alt="${alt}" src="${url}">` : alt);
  html = html.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (_, text, url) =>
    /^(https?:\/\/|\/(?!\/))/i.test(url) ? `<a href="${url}" rel="noopener">${text}</a>` : text);
  html = html.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  return html.replace(/\n/g, "<br>");
}
