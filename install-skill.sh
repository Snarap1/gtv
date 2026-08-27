#!/usr/bin/env bash
set -euo pipefail

src_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
skill_name="gtv"
src_dir="${src_root}/skills/${skill_name}"

scope="project"
project_dir="$(pwd)"
agents_arg="all"
mode="copy"
uninstall=0
dry_run=0
force=0

usage() {
  cat <<'USAGE'
Usage: ./install-skill.sh [options]

Scope (pick one, default --project):
  --project[=DIR]   install into DIR/.claude, DIR/.opencode, DIR/.agents
                    (default DIR: current directory)
  --user            install into ~/.claude, ~/.config/opencode, ~/.codex

Agents:
  --agent=LIST      comma-separated: claude, opencode, codex, all (default: all)

Other:
  --link            symlink the skill directory instead of copying it
                    (links the whole directory, evals/ included - for editing)
  --force           overwrite an existing skill that is not managed by this script
  --uninstall       remove the installed skill instead of installing it
  --dry-run         print what would happen, change nothing
  -h, --help        this text

Installed paths:
  claude    <project>/.claude/skills/gtv        ~/.claude/skills/gtv
  opencode  <project>/.opencode/skills/gtv      ~/.config/opencode/skills/gtv
  codex     <project>/.agents/skills/gtv        ~/.codex/skills/gtv
USAGE
}

for arg in "$@"; do
  case "${arg}" in
    --user) scope="user" ;;
    --project) scope="project" ;;
    --project=*)
      scope="project"
      project_dir="${arg#--project=}"
      ;;
    --agent=*) agents_arg="${arg#--agent=}" ;;
    --agents=*) agents_arg="${arg#--agents=}" ;;
    --link) mode="link" ;;
    --copy) mode="copy" ;;
    --force) force=1 ;;
    --uninstall) uninstall=1 ;;
    --dry-run) dry_run=1 ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: ${arg}" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ ! -f "${src_dir}/SKILL.md" ]]; then
  echo "missing ${src_dir}/SKILL.md" >&2
  exit 1
fi

if [[ "${scope}" == "project" ]]; then
  if [[ ! -d "${project_dir}" ]]; then
    echo "no such directory: ${project_dir}" >&2
    exit 1
  fi
  project_dir="$(cd "${project_dir}" && pwd)"
fi

target_dir() {
  case "$1:${scope}" in
    claude:project)   echo "${project_dir}/.claude/skills/${skill_name}" ;;
    claude:user)      echo "${HOME}/.claude/skills/${skill_name}" ;;
    opencode:project) echo "${project_dir}/.opencode/skills/${skill_name}" ;;
    opencode:user)    echo "${XDG_CONFIG_HOME:-${HOME}/.config}/opencode/skills/${skill_name}" ;;
    codex:project)    echo "${project_dir}/.agents/skills/${skill_name}" ;;
    codex:user)       echo "${HOME}/.codex/skills/${skill_name}" ;;
    *)                echo "" ;;
  esac
}

marker=".installed-by-gtv"

if [[ "${agents_arg}" == "all" ]]; then
  agents=(claude opencode codex)
else
  IFS=',' read -r -a agents <<<"${agents_arg}"
fi

for agent in "${agents[@]}"; do
  if [[ -z "$(target_dir "${agent}")" ]]; then
    echo "unknown agent: ${agent} (want claude, opencode, codex or all)" >&2
    exit 1
  fi
done

run() {
  if [[ "${dry_run}" -eq 1 ]]; then
    printf '  would: %s\n' "$*"
    return 0
  fi
  "$@"
}

say() {
  if [[ "${dry_run}" -eq 0 ]]; then
    echo "$@"
  fi
}

install_one() {
  local dest="$1"

  if [[ -e "${dest}" || -L "${dest}" ]]; then
    if [[ ! -L "${dest}" && ! -f "${dest}/${marker}" && "${force}" -eq 0 ]]; then
      echo "refusing to overwrite unmanaged ${dest} (use --force)" >&2
      return 1
    fi
    run rm -rf "${dest}"
  fi

  run mkdir -p "$(dirname "${dest}")"
  if [[ "${mode}" == "link" ]]; then
    run ln -s "${src_dir}" "${dest}"
    say "linked ${dest} -> ${src_dir}"
  else
    run cp -R "${src_dir}" "${dest}"
    # evals/ is the skill's own test material; agents have no use for it.
    run rm -rf "${dest}/evals"
    run touch "${dest}/${marker}"
    say "installed ${dest}"
  fi
}

uninstall_one() {
  local dest="$1"

  if [[ ! -e "${dest}" && ! -L "${dest}" ]]; then
    say "not installed at ${dest}"
    return 0
  fi
  if [[ ! -L "${dest}" && ! -f "${dest}/${marker}" && "${force}" -eq 0 ]]; then
    echo "refusing to remove unmanaged ${dest} (use --force)" >&2
    return 1
  fi
  run rm -rf "${dest}"
  say "removed ${dest}"

  local parent
  parent="$(dirname "${dest}")"
  if [[ "${dry_run}" -eq 0 && -d "${parent}" ]]; then
    rmdir "${parent}" 2>/dev/null || true
  fi
}

status=0
for agent in "${agents[@]}"; do
  dest="$(target_dir "${agent}")"
  if [[ "${uninstall}" -eq 1 ]]; then
    uninstall_one "${dest}" || status=1
  else
    install_one "${dest}" || status=1
  fi
done

if [[ "${uninstall}" -eq 0 && "${status}" -eq 0 && "${dry_run}" -eq 0 ]]; then
  echo
  echo "restart the agent (or start a new session) to pick the skill up"
fi

exit "${status}"
