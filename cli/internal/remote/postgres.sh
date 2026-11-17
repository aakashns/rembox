# postgres.sh: has apt install PostgreSQL from PostgreSQL's apt repo, and adds
# a PostgreSQL superuser for your user, so it can run psql as itself.

# Where PostgreSQL's docs, at postgresql.org/download/linux/debian, put them.
PG_KEY=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc
PG_SOURCES=/etc/apt/sources.list.d/pgdg.list

# add_postgres_repo: has apt install PostgreSQL from PostgreSQL's apt repo, as
# the distros' lag behind (15 on Debian 12, 16 on Ubuntu 24.04). It goes
# before update, and install_basics installs postgresql from it.
add_postgres_repo() {
  step "Install PostgreSQL from PostgreSQL's apt repo, which has its latest version"
  local codename dir changed=no content
  # shellcheck disable=SC1091
  codename=$(. /etc/os-release && echo "$VERSION_CODENAME")
  # A release the repo doesn't have yet would make apt update fail.
  if ! curl -fsSI "https://apt.postgresql.org/pub/repos/apt/dists/$codename-pgdg/Release" >/dev/null; then
    warn "PostgreSQL's apt repo doesn't have $codename, or couldn't be reached, so PostgreSQL comes from the distro, or stays as it is."
    return
  fi
  dir=$(mktemp -d)
  # The key may change, so it's downloaded again each time.
  if ! run curl -fsSL -o "$dir/key.asc" https://www.postgresql.org/media/keys/ACCC4CF8.asc; then
    rm -rf "$dir"
    warn "Couldn't download the key for PostgreSQL's apt repo, so PostgreSQL comes from the distro, or stays as it is. Run rembox setup again later."
    return
  fi
  if ! cmp -s "$dir/key.asc" "$PG_KEY"; then
    [[ -d ${PG_KEY%/*} ]] || run install -d -m 755 "${PG_KEY%/*}"
    run install -m 644 "$dir/key.asc" "$PG_KEY"
    changed=yes
  fi
  rm -rf "$dir"
  content="deb [signed-by=$PG_KEY] https://apt.postgresql.org/pub/repos/apt $codename-pgdg main"
  if ! same "$PG_SOURCES" "$content"; then
    put "$PG_SOURCES" 644 "$content"
    changed=yes
  fi
  if [[ $changed == yes ]]; then
    ok "apt installs PostgreSQL from PostgreSQL's apt repo, and update upgrades it."
  else
    ok "apt installs PostgreSQL from PostgreSQL's apt repo already."
  fi
}

# add_postgres_role USER: adds a PostgreSQL superuser role named USER, and a
# database of the same name, so USER can just run psql: PostgreSQL lets in a
# Linux user as the role of the same name, without a password. Leaves a role
# or database that's there already as it is.
add_postgres_role() {
  local user=$1 version
  step "Let $user run psql as a PostgreSQL superuser"
  # From /, which the postgres user can read, so psql doesn't warn it can't
  # change to root's home.
  cd /
  if [[ $(runuser -u postgres -- psql -tAc "SELECT 1 FROM pg_roles WHERE rolname = '$user'") != 1 ]]; then
    run runuser -u postgres -- createuser --superuser "$user"
  fi
  if [[ $(runuser -u postgres -- psql -tAc "SELECT 1 FROM pg_database WHERE datname = '$user'") != 1 ]]; then
    run runuser -u postgres -- createdb --owner "$user" "$user"
  fi
  version=$(runuser -u postgres -- psql -tAc 'SHOW server_version' | cut -d ' ' -f 1)
  cd - >/dev/null
  ok "$user can run psql as itself, on PostgreSQL $version."
  checklist ok "PostgreSQL $version, with psql as $user"
}
