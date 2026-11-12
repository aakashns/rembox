// A minimal JSX runtime that renders straight to HTML strings: no virtual DOM, no diffing, no
// client-side hydration. A component is a plain function from props to Html, and an element is
// rendered the moment it's created, so a page is one pass of string concatenation.
//
// Text and attribute values are escaped. The only way to put markup in a page unescaped is
// raw(), which is kept to HTML the site writes itself.

// Markup that's already safe to put in a page. Strings are escaped; Html isn't.
export class Html {
  constructor(readonly value: string) {}
  toString() {
    return this.value;
  }
}

export type Child =
  Html | string | number | bigint | boolean | null | undefined | Child[];

export type Component<P = {}> = (props: P) => Html;

// Markup to use as it is. Only for HTML the site wrote.
export function raw(html: string): Html {
  return new Html(html);
}

export function Fragment(props: { children?: Child | undefined }): Html {
  return new Html(renderChildren(props.children));
}

// An element's props: its attributes, and what's inside it.
type ElementProps = { children?: Child | undefined } & Record<string, unknown>;

export function jsx<P>(
  type: string | Component<P>,
  props: P & ElementProps,
): Html {
  if (typeof type === "function") return type(props);

  let { children, ...attributes } = props;
  let html = `<${type}${renderAttributes(attributes)}>`;
  if (VOID_ELEMENTS.has(type)) return new Html(html);
  return new Html(`${html}${renderChildren(children)}</${type}>`);
}

export { jsx as jsxs, jsx as jsxDEV };

// Text in an element: &, <, and > are all that can change its meaning.
export function escapeHtml(text: string): string {
  return TEXT_SPECIAL.test(text)
    ? text.replace(TEXT_SPECIAL_ALL, escapeChar)
    : text;
}

// A value between double quotes.
export function escapeAttribute(value: string): string {
  return ATTRIBUTE_SPECIAL.test(value)
    ? value.replace(ATTRIBUTE_SPECIAL_ALL, escapeChar)
    : value;
}

function renderChildren(children: Child): string {
  if (children == null || typeof children === "boolean") return "";
  if (children instanceof Html) return children.value;
  if (Array.isArray(children)) {
    let html = "";
    for (let child of children) html += renderChildren(child);
    return html;
  }
  return escapeHtml(String(children));
}

// `true` writes the attribute alone, and `false`, null, and undefined leave it out. Props are
// written as named, so pages use HTML's names (`class`, `for`, `data-on:click`).
function renderAttributes(attributes: Record<string, unknown>): string {
  let html = "";
  for (let name in attributes) {
    let value = attributes[name];
    if (value == null || value === false) continue;
    html +=
      value === true
        ? ` ${name}`
        : ` ${name}="${escapeAttribute(String(value))}"`;
  }
  return html;
}

const TEXT_SPECIAL = /[&<>]/;
const TEXT_SPECIAL_ALL = /[&<>]/g;
const ATTRIBUTE_SPECIAL = /[&<>"]/;
const ATTRIBUTE_SPECIAL_ALL = /[&<>"]/g;
const ESCAPES: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
};
const escapeChar = (char: string) => ESCAPES[char] ?? char;

const VOID_ELEMENTS = new Set([
  "area",
  "base",
  "br",
  "col",
  "embed",
  "hr",
  "img",
  "input",
  "link",
  "meta",
  "source",
  "track",
  "wbr",
]);

export namespace JSX {
  export type Element = Html;
  export interface ElementChildrenAttribute {
    children: {};
  }
  // Any element with any attributes: pages are checked by their tests and by eye, and a strict
  // DOM typing would add more code than this whole runtime.
  export interface IntrinsicElements {
    [tag: string]: ElementProps;
  }
}
