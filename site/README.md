# rembox site

[rembox.aakashns.com](https://rembox.aakashns.com): plain HTML in `public/`, with no build step,
and `public/install.sh`, the installer. [`Caddyfile`](Caddyfile) serves it.

To preview it locally, with live reload: `npx live-server public --port=8788`

`images/render.sh` renders `public/social.png` (the link preview, from `images/social.html`),
`favicon.ico` and `apple-touch-icon.png` (from `icon.svg`). Re-run it when you change those.

## Deploying

The server serves the repo's `site/public` from `/srv/rembox`, which the `caddy` user can read
(it can't read anything under `/root`). To set it up, as root, with a working copy in `/root/work`:

```sh
mv /root/work/rembox /srv/rembox && ln -s /srv/rembox /root/work/rembox
chmod -R a+rX /srv/rembox/site/public
sudo -u caddy cat /srv/rembox/site/public/index.html > /dev/null && echo readable
```

Then add [`Caddyfile`](Caddyfile)'s site block to `/etc/caddy/Caddyfile` and run
`systemctl reload caddy`. To deploy changes, `git -C /srv/rembox pull`.
