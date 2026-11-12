import { Document, Wordmark } from "./layout.tsx";

const github = "https://github.com/aakashns/rembox";
const url = "https://rembox.org/";
const title = "rembox — Set up and manage remote development machines";
const description =
  "Turn a fresh Ubuntu server into a secure remote dev box, from your Mac.";
const install = `curl -fsSL ${url}install.sh | sh`;

// What `rembox help` lists, as the docs for each command.
const commands = [
  {
    usage: "rembox help [command]",
    about: "Lists the commands, or explains one of them.",
  },
  {
    usage: "rembox version",
    about: "Prints the version, and the commit it was built from.",
  },
  {
    usage: "rembox upgrade [--yes]",
    about: (
      <>
        Replaces rembox with the latest release, after showing both versions and
        asking first. Pass <code>--yes</code> to skip the question, e.g. in a
        script.
      </>
    ),
  },
];

// A shell command, shown as its own block.
function Command(props: { children: string }) {
  return (
    <pre class="block">
      <code>{props.children}</code>
    </pre>
  );
}

export function Home() {
  return (
    <Document
      title={title}
      head={
        <>
          <meta name="description" content={description} />
          <link rel="canonical" href={url} />
          <meta property="og:type" content="website" />
          <meta property="og:title" content={title} />
          <meta property="og:description" content={description} />
          <meta property="og:url" content={url} />
          <script src="/site.js" defer></script>
        </>
      }
    >
      <a class="skip-link" href="#main">
        Skip to content
      </a>
      <header class="site">
        <a class="name" href="/" aria-label="rembox home">
          <Wordmark />
        </a>
        <nav aria-label="Main navigation">
          <a href={github} target="_blank">
            GitHub
          </a>
          <a href={`${github}/releases`} target="_blank">
            Changelog
          </a>
        </nav>
      </header>
      <main id="main">
        <h1>
          Remote dev boxes, <br class="mobile" />
          set up securely
        </h1>
        <p class="lede">
          rembox turns a fresh Ubuntu server into a secure remote development
          machine, and helps you manage it from your Mac.
        </p>

        <p>Run this on your Mac (or a Linux machine) to install rembox:</p>
        <div class="command">
          <pre>
            <code id="install-command">{install}</code>
          </pre>
          <button
            type="button"
            id="copy"
            hidden
            aria-label="Copy install command"
          >
            Copy
          </button>
        </div>
        <p id="copy-status" class="sr-only" role="status"></p>
        <p class="note">
          It checks the download against the release’s checksums and installs it
          at <code>/usr/local/bin/rembox</code>, using sudo only if that folder
          needs it. <a href="/install.sh">Read the script</a>.
        </p>

        <p class="callout">
          <strong>Early days.</strong> So far rembox installs and upgrades
          itself. Setting up servers comes next, and this page will document
          each step as it lands.
        </p>

        <section aria-labelledby="commands">
          <h2 id="commands">Commands</h2>
          <dl class="commands">
            {commands.map((command) => (
              <>
                <dt>
                  <code>{command.usage}</code>
                </dt>
                <dd>{command.about}</dd>
              </>
            ))}
          </dl>
        </section>

        <section aria-labelledby="installing">
          <h2 id="installing">Installing</h2>
          <p>
            Run the install command again to reinstall or upgrade. Two
            environment variables change what it does:
          </p>
          <dl class="commands">
            <dt>
              <code>REMBOX_VERSION</code>
            </dt>
            <dd>Installs this version, like 0.1.0, instead of the latest.</dd>
            <dt>
              <code>REMBOX_INSTALL_DIR</code>
            </dt>
            <dd>
              Installs in this folder instead of <code>/usr/local/bin</code>.
            </dd>
          </dl>
          <Command>{`curl -fsSL ${url}install.sh | REMBOX_VERSION=0.1.0 sh`}</Command>
          <p>
            Releases have a binary for macOS and Linux, on both Apple
            Silicon/ARM and Intel/AMD processors.
          </p>
        </section>

        <section aria-labelledby="upgrading">
          <h2 id="upgrading">Upgrading and uninstalling</h2>
          <p>
            <code>rembox upgrade</code> installs the latest release in place of
            the one you ran. If it’s in a folder only root can write to, as{" "}
            <code>/usr/local/bin</code> often is, run it with sudo. To
            uninstall, delete the binary:
          </p>
          <Command>{`sudo rembox upgrade
sudo rm /usr/local/bin/rembox`}</Command>
        </section>

        <section aria-labelledby="security">
          <h2 id="security">Verifying a release</h2>
          <p>
            Every release is built by{" "}
            <a href={`${github}/actions`} target="_blank">
              GitHub Actions
            </a>{" "}
            from the main branch, with a <code>SHA256SUMS</code> file that both
            the install script and <code>rembox upgrade</code> check downloads
            against. Each binary also has a signed build attestation, which you
            can check with the GitHub CLI:
          </p>
          <Command>{`gh attestation verify "$(command -v rembox)" --repo aakashns/rembox`}</Command>
        </section>
      </main>
      <footer class="site">
        <span>
          Free and{" "}
          <a href={github} target="_blank">
            open source
          </a>
          <span aria-hidden="true"> · </span>
          Built by{" "}
          <a href="https://aakashns.com" target="_blank">
            aakashns
          </a>
        </span>
      </footer>
    </Document>
  );
}
