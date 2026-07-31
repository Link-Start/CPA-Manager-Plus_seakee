#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
version="${VERSION:-dev}"
out_dir="${OUT_DIR:-"${repo_root}/dist/native"}"
web_html="${WEB_HTML:-"${repo_root}/apps/web/dist/index.html"}"
binary_name="cpa-manager-plus"
server_src="${repo_root}/apps/manager-server"
native_script_src="${repo_root}/bin/native"
release_public_key="${RELEASE_PUBLIC_KEY:-}"
cpa_version="${CPA_VERSION:-}"
cpa_asset_dir="${CPA_ASSET_DIR:-}"
require_full_packages="${REQUIRE_FULL_PACKAGES:-false}"
output_marker_name=".cpamp-native-packaging-output"

prepare_output_directory() {
  local requested="$1"
  local resolved=""
  local unexpected=""

  case "${requested}" in
    ""|/|.|..) echo "refusing unsafe OUT_DIR: ${requested:-<empty>}" >&2; exit 1 ;;
  esac
  if [ -L "${requested}" ]; then
    echo "refusing symlink OUT_DIR: ${requested}" >&2
    exit 1
  fi
  mkdir -p "${requested}"
  resolved="$(cd "${requested}" && pwd -P)"
  if [ "${resolved}" = "/" ] || [ "${resolved}" = "${repo_root}" ]; then
    echo "refusing unsafe OUT_DIR: ${resolved}" >&2
    exit 1
  fi
  case "${repo_root}/" in
    "${resolved}/"*)
      echo "refusing OUT_DIR that contains the repository: ${resolved}" >&2
      exit 1
      ;;
  esac
  case "${resolved}" in
    "${repo_root}/dist/"*|"${repo_root}/bin/tmp/"*) ;;
    "${repo_root}/"*)
      echo "OUT_DIR inside the repository must be under dist/ or bin/tmp/: ${resolved}" >&2
      exit 1
      ;;
    *)
      if [ -L "${resolved}/${output_marker_name}" ]; then
        echo "refusing OUT_DIR with a symlink ownership marker: ${resolved}" >&2
        exit 1
      fi
      unexpected="$(find "${resolved}" -mindepth 1 -maxdepth 1 ! -name "${output_marker_name}" -print -quit)"
      if [ -n "${unexpected}" ] && [ ! -f "${resolved}/${output_marker_name}" ]; then
        echo "refusing to clear non-empty OUT_DIR without ${output_marker_name}: ${resolved}" >&2
        exit 1
      fi
      ;;
  esac
  out_dir="${resolved}"
}

if [[ ! "${version}" =~ ^[vV]?[0-9A-Za-z][0-9A-Za-z._+-]*$ ]] && [ "${version}" != "dev" ]; then
  echo "invalid VERSION: ${version}" >&2
  exit 1
fi
if [ -n "${release_public_key}" ] && [[ ! "${release_public_key}" =~ ^[A-Za-z0-9+/=_-]+$ ]]; then
  echo "RELEASE_PUBLIC_KEY contains unsupported characters" >&2
  exit 1
fi
if [ ! -f "${web_html}" ]; then
  echo "missing ${web_html}; run npm run build first" >&2
  exit 1
fi
if { [ -n "${cpa_version}" ] && [ -z "${cpa_asset_dir}" ]; } || { [ -z "${cpa_version}" ] && [ -n "${cpa_asset_dir}" ]; }; then
  echo "CPA_VERSION and CPA_ASSET_DIR must be configured together" >&2
  exit 1
fi
if [ -n "${cpa_version}" ] && [[ ! "${cpa_version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.+][0-9A-Za-z.-]+)?$ ]]; then
  echo "invalid CPA_VERSION: ${cpa_version}" >&2
  exit 1
fi
if [ "${require_full_packages}" = "true" ] && { [ -z "${cpa_version}" ] || [ -z "${cpa_asset_dir}" ]; }; then
  echo "Full native packages require CPA_VERSION and CPA_ASSET_DIR" >&2
  exit 1
fi
if [ -n "${cpa_asset_dir}" ]; then
  checksum_file="${cpa_asset_dir}/checksums.txt"
  if [ ! -f "${checksum_file}" ]; then
    if [ "${require_full_packages}" = "true" ]; then
      echo "Full native packages require ${checksum_file}" >&2
      exit 1
    fi
  else
    (
      cd "${cpa_asset_dir}"
      if command -v sha256sum >/dev/null 2>&1; then
        sha256sum -c checksums.txt
      else
        shasum -a 256 -c checksums.txt
      fi
    )
  fi
fi

prepare_output_directory "${out_dir}"
mkdir -p "${repo_root}/bin/tmp/release"
work_dir="$(mktemp -d "${repo_root}/bin/tmp/release/native.XXXXXX")"
trap 'rm -rf "${work_dir}"' EXIT

rm -rf "${out_dir}"
mkdir -p "${out_dir}"
printf 'CPA Manager Plus native packaging output.\n' >"${out_dir}/${output_marker_name}"

cp -R "${server_src}" "${work_dir}/manager-server"
cp "${web_html}" "${work_dir}/manager-server/internal/httpapi/web/management.html"

targets=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
  "windows arm64"
)

archive_package() {
  local package_name="$1"
  local goos="$2"

  if [ "${goos}" = "windows" ]; then
    (
      cd "${work_dir}"
      zip -qr "${out_dir}/${package_name}.zip" "${package_name}"
    )
    return
  fi

  (
    cd "${work_dir}"
    tar -czf "${out_dir}/${package_name}.tar.gz" "${package_name}"
  )
}

copy_package_files() {
  local package_dir="$1"
  local goos="$2"
  local runtime_mode="$3"
  local built_binary="$4"
  local exe_name="$5"

  mkdir -p "${package_dir}"
  cp "${built_binary}" "${package_dir}/${exe_name}"
  printf '%s\n' "${runtime_mode}" >"${package_dir}/.cpamp-runtime-mode"
  cp "${repo_root}/README.md" "${package_dir}/README.md"
  cp "${repo_root}/README_CN.md" "${package_dir}/README_CN.md"
  cp -R "${repo_root}/docs" "${package_dir}/docs"
  cp "${repo_root}/LICENSE" "${package_dir}/LICENSE"
  if [ "${goos}" = "windows" ]; then
    cp "${native_script_src}/cpa-manager-plusctl.ps1" "${package_dir}/cpa-manager-plusctl.ps1"
  else
    cp "${native_script_src}/cpa-manager-plusctl.sh" "${package_dir}/cpa-manager-plusctl"
    chmod 0755 "${package_dir}/cpa-manager-plusctl"
  fi
}

find_cpa_asset() {
  local goos="$1"
  local goarch="$2"
  local asset_arch="${goarch}"
  local release_version="${cpa_version#[vV]}"
  local extension="tar.gz"
  local asset_suffix=""

  if [ "${goarch}" = "arm64" ]; then
    asset_arch="aarch64"
  fi
  if [ "${goos}" = "windows" ]; then
    extension="zip"
  fi
  if [ "${goos}" = "linux" ]; then
    asset_suffix="_no-plugin"
  fi

  local candidate="${cpa_asset_dir}/CLIProxyAPI_${release_version}_${goos}_${asset_arch}${asset_suffix}.${extension}"
  if [ ! -f "${candidate}" ]; then
    echo "missing CPA asset ${candidate}" >&2
    return 1
  fi
  printf '%s\n' "${candidate}"
}

extract_cpa_binary() {
  local archive="$1"
  local goos="$2"
  local destination="$3"
  local cpa_exe="cli-proxy-api"

  if [ "${goos}" = "windows" ]; then
    cpa_exe="cli-proxy-api.exe"
    unzip -p "${archive}" "${cpa_exe}" >"${destination}/${cpa_exe}"
  else
    tar -xOzf "${archive}" "${cpa_exe}" >"${destination}/${cpa_exe}"
    chmod 0755 "${destination}/${cpa_exe}"
  fi

  if [ ! -s "${destination}/${cpa_exe}" ]; then
    echo "CPA archive ${archive} did not contain ${cpa_exe}" >&2
    exit 1
  fi
}

for target in "${targets[@]}"; do
  read -r goos goarch <<<"${target}"
  exe_name="${binary_name}"
  if [ "${goos}" = "windows" ]; then
    exe_name="${binary_name}.exe"
  fi

  build_dir="${work_dir}/build-${goos}-${goarch}"
  mkdir -p "${build_dir}"
  ldflags="-s -w -X github.com/seakee/cpa-manager-plus/apps/manager-server/internal/managedruntime.BuildVersion=${version}"
  ldflags+=" -X github.com/seakee/cpa-manager-plus/apps/manager-server/internal/managedruntime.EmbeddedReleasePublicKey=${release_public_key}"
  if [ -n "${cpa_version}" ]; then
    ldflags+=" -X github.com/seakee/cpa-manager-plus/apps/manager-server/internal/managedruntime.EmbeddedCPAVersion=${cpa_version}"
  fi
  (
    cd "${work_dir}/manager-server"
    CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" go build \
      -trimpath \
      -ldflags "${ldflags}" \
      -o "${build_dir}/${exe_name}" \
      ./cmd/cpa-manager-plus
  )

  slim_name="${binary_name}_${version}_${goos}_${goarch}_slim"
  slim_dir="${work_dir}/${slim_name}"
  copy_package_files "${slim_dir}" "${goos}" "slim" "${build_dir}/${exe_name}" "${exe_name}"
  archive_package "${slim_name}" "${goos}"

  compatibility_name="${binary_name}_${version}_${goos}_${goarch}"
  compatibility_dir="${work_dir}/${compatibility_name}"
  copy_package_files "${compatibility_dir}" "${goos}" "slim" "${build_dir}/${exe_name}" "${exe_name}"
  archive_package "${compatibility_name}" "${goos}"

  if [ -n "${cpa_version}" ]; then
    full_name="${binary_name}_${version}_${goos}_${goarch}_full"
    full_dir="${work_dir}/${full_name}"
    copy_package_files "${full_dir}" "${goos}" "integrated" "${build_dir}/${exe_name}" "${exe_name}"
    cpa_asset="$(find_cpa_asset "${goos}" "${goarch}")"
    extract_cpa_binary "${cpa_asset}" "${goos}" "${full_dir}"
    archive_package "${full_name}" "${goos}"
  fi
done

(
  cd "${out_dir}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum ./* >checksums.txt
  else
    shasum -a 256 ./* >checksums.txt
  fi
)
