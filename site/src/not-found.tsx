import { Document } from "./layout.tsx";

// Cloudflare serves it, with a 404, for any path without a file.
export function NotFound() {
  return (
    <Document
      title="Page not found — rembox"
      head={<meta name="robots" content="noindex" />}
    >
      <main id="main">
        <h1>Page not found</h1>
        <p class="lede">
          This page doesn’t exist. <a href="/">Go to the home page</a>.
        </p>
      </main>
    </Document>
  );
}
