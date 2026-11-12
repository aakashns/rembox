import type { Child } from "#jsx/jsx-runtime";

// The page every other is in: what goes in every <head>, then the page's own.
export function Document(props: {
  title: string;
  head?: Child;
  children?: Child;
}) {
  return (
    <html lang="en">
      <head>
        <meta charset="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <title>{props.title}</title>
        <meta name="theme-color" content="#161616" />
        <link rel="icon" href="/icon.svg" type="image/svg+xml" />
        <link rel="stylesheet" href="/style.css" />
        {props.head}
      </head>
      <body>{props.children}</body>
    </html>
  );
}

// "rembox" set in the monospace font, with a prompt before it.
export function Wordmark() {
  return (
    <span class="wordmark">
      <span aria-hidden="true">&gt;_</span>rembox
    </span>
  );
}
