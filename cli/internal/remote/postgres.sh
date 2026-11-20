# postgres.sh: has apt install PostgreSQL from PostgreSQL's apt repo, and adds
# a PostgreSQL superuser for your user, so it can run psql as itself.

# PostgreSQL's script, from postgresql-common, that adds its apt repo, as
# PostgreSQL's docs, at postgresql.org/download/linux/debian, do. It writes
# pgdg.sources, or pgdg.list in older versions.
PG_REPO_SCRIPT=/usr/share/postgresql-common/pgdg/apt.postgresql.org.sh
PG_SOURCES=(/etc/apt/sources.list.d/pgdg.sources /etc/apt/sources.list.d/pgdg.list)

# add_postgres_repo: has apt install PostgreSQL from PostgreSQL's apt repo, as
# the distros' lag behind (15 on Debian 12, 16 on Ubuntu 24.04), with
# PostgreSQL's own script, unless it's added already. Then update upgrades
# postgresql-common, and so the repo's key, from it. It goes before update,
# and install_basics installs postgresql from it.
add_postgres_repo() {
  step "Install PostgreSQL from PostgreSQL's apt repo, which has its latest version"
  if [[ -e ${PG_SOURCES[0]} || -e ${PG_SOURCES[1]} ]]; then
    ok "apt installs PostgreSQL from PostgreSQL's apt repo already."
    return
  fi
  if [[ ! -x $PG_REPO_SCRIPT ]]; then
    run "${APT[@]}" update
    install_packages postgresql-common
  fi
  # -y: doesn't ask first. It checks the repo has this release, and runs
  # apt-get update.
  if ! run "$PG_REPO_SCRIPT" -y; then
    warn "PostgreSQL's script couldn't add its apt repo, as it says above, so PostgreSQL comes from the distro, or stays as it is."
    return
  fi
  ok "apt installs PostgreSQL from PostgreSQL's apt repo, and update upgrades it."
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
