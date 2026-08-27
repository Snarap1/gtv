#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

install=0
uninstall=0
prefix="${HOME}/.local/bin"

usage() {
  cat <<'EOF'
Usage: ./build.sh [--install|--uninstall] [--prefix=DIR]

  (default)     cross-compile linux/darwin/windows into dist/
  --install     build for this host and install to PREFIX/gtv
  --uninstall   remove PREFIX/gtv and the gtv cache directory
  --prefix=DIR  install directory (default: ~/.local/bin)
EOF
}

for arg in "$@"; do
  case "${arg}" in
    --install) install=1 ;;
    --uninstall) uninstall=1 ;;
    --prefix=*) prefix="${arg#--prefix=}" ;;
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

if [[ "${install}" -eq 1 && "${uninstall}" -eq 1 ]]; then
  echo "use either --install or --uninstall, not both" >&2
  exit 1
fi

if [[ "${uninstall}" -eq 1 ]]; then
  dest="${prefix}/gtv"
  if [[ -e "${dest}" || -L "${dest}" ]]; then
    rm -f "${dest}"
    echo "removed ${dest}"
  else
    echo "not installed at ${dest}"
  fi
  if [[ -e "${dest}.exe" || -L "${dest}.exe" ]]; then
    rm -f "${dest}.exe"
    echo "removed ${dest}.exe"
  fi

  cache_dir="${XDG_CACHE_HOME:-${HOME}/.cache}/gtv"
  if [[ -d "${cache_dir}" ]]; then
    rm -rf "${cache_dir}"
    echo "removed ${cache_dir}"
  fi
  exit 0
fi

version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
ldflags="-s -w -X main.version=${version}"

build_one() {
  local goos="$1" goarch="$2" out="$3"
  echo "building ${out}"
  GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 \
    go build -trimpath -ldflags "${ldflags}" -o "${out}" ./cmd/gtv
}

if [[ "${install}" -eq 1 ]]; then
  goos="$(go env GOOS)"
  goarch="$(go env GOARCH)"
  mkdir -p dist "${prefix}"
  out="dist/gtv-${goos}-${goarch}"
  if [[ "${goos}" == "windows" ]]; then
    out+=".exe"
  fi
  build_one "${goos}" "${goarch}" "${out}"

  dest="${prefix}/gtv"
  if [[ "${goos}" == "windows" ]]; then
    dest+=".exe"
  fi
  install -m 755 "${out}" "${dest}"
  echo "installed ${dest} (version ${version})"

  case ":${PATH}:" in
    *":${prefix}:"*) ;;
    *)
      echo "note: ${prefix} is not on PATH; add to your shell rc:" >&2
      echo "  export PATH=\"${prefix}:\$PATH\"" >&2
      ;;
  esac
  exit 0
fi

targets=(
  "linux amd64"
  "darwin arm64"
  "windows amd64"
)

rm -rf dist
mkdir -p dist

for target in "${targets[@]}"; do
  read -r goos goarch <<<"${target}"
  out="dist/gtv-${goos}-${goarch}"
  if [[ "${goos}" == "windows" ]]; then
    out+=".exe"
  fi
  build_one "${goos}" "${goarch}" "${out}"
done

echo "built version ${version}"
