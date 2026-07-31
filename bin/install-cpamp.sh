#!/usr/bin/env bash
set -euo pipefail

repo="seakee/CPA-Manager-Plus"
default_cpamp_image="seakee/cpa-manager-plus:latest"
default_cpa_image="eceasy/cli-proxy-api:latest"
default_install_dir="${HOME:-.}/cpa-manager-plus"
native_graceful_stop_timeout_seconds=45
docker_stop_timeout_seconds=45

dry_run="${CPAMP_DRY_RUN:-0}"
non_interactive="${CPAMP_NON_INTERACTIVE:-0}"
skip_execute="${CPAMP_SKIP_EXECUTE:-0}"
lang_code="${CPAMP_LANG:-}"
operation="${CPAMP_OPERATION:-}"

os_name="unknown"
arch_name="unknown"
normalized_os="unknown"
normalized_arch="unknown"
is_wsl="false"

install_mode=""
deploy_method=""
install_dir=""
cpamp_port=""
cpamp_api_port=""
cpamp_panel_port=""
cpa_port=""
cpamp_image=""
cpa_image=""
cpamp_version=""
cpa_connection_mode=""
cpa_url=""
cpa_management_key=""
admin_key=""
demo_client_key=""
deployment_mode=""
detected_cpa_url=""
cpa_validation_url=""
bootstrap_token=""
bootstrap_required=""
usage_db_path=""
generated_admin_key=""
generated_cpa_management_key=""
generated_demo_client_key=""
compose_project_name="${CPAMP_PROJECT_NAME:-cpamp}"
existing_install_state="fresh"
existing_volume_name=""
existing_native_health_port="18317"
generated_backup_dir=""
auth_validation_status="pending"
admin_secret_missing="0"
active_cpamp_internal_port=""
existing_docker_panel_port_configured="0"
previous_cpamp_image_id=""
previous_cpa_image_id=""
previous_cpamp_image_ref=""
previous_cpa_image_ref=""
existing_native_binary_dir=""
target_native_binary_dir=""
existing_deploy_method=""
existing_install_mode=""
existing_cpa_connection_mode=""
native_running_pid=""
native_running_start=""
native_record_format=""
native_record_pid=""
native_record_start=""

die() {
  printf '%s\n' "$*" >&2
  exit 1
}

text() {
  case "${lang_code:-zh-CN}:$1" in
    zh-CN:env_header) printf '检测到当前环境' ;;
    en-US:env_header) printf 'Detected environment' ;;
    zh-CN:os) printf '系统' ;;
    en-US:os) printf 'OS' ;;
    zh-CN:arch) printf '架构' ;;
    en-US:arch) printf 'Architecture' ;;
    zh-CN:wsl) printf 'WSL' ;;
    en-US:wsl) printf 'WSL' ;;
    zh-CN:continue) printf '继续使用这个环境安装吗? 输入 yes/no' ;;
    en-US:continue) printf 'Continue with this environment? Enter yes/no' ;;
    zh-CN:select_mode) printf '选择安装范围: 1) CPA + CPAMP 完整安装  2) 仅安装 CPAMP' ;;
    en-US:select_mode) printf 'Select install scope: 1) CPA + CPAMP stack  2) CPAMP only' ;;
    zh-CN:select_method) printf '选择部署方式: 1) Docker  2) 二进制/native' ;;
    en-US:select_method) printf 'Select deployment method: 1) Docker  2) Native binary' ;;
    zh-CN:install_dir) printf '安装目录' ;;
    en-US:install_dir) printf 'Install directory' ;;
    zh-CN:cpamp_port) printf 'CPAMP 兼容端口' ;;
    en-US:cpamp_port) printf 'CPAMP compatibility port' ;;
    zh-CN:cpamp_api_port) printf 'CPAMP 网关端口' ;;
    en-US:cpamp_api_port) printf 'CPAMP gateway port' ;;
    zh-CN:cpamp_panel_port) printf 'CPAMP 管理入口端口' ;;
    en-US:cpamp_panel_port) printf 'CPAMP management entry port' ;;
    zh-CN:cpa_port) printf 'CPA 对外端口' ;;
    en-US:cpa_port) printf 'Public CPA port' ;;
    zh-CN:cpamp_image) printf 'CPAMP Docker 镜像' ;;
    en-US:cpamp_image) printf 'CPAMP Docker image' ;;
    zh-CN:cpa_image) printf 'CPA Docker 镜像' ;;
    en-US:cpa_image) printf 'CPA Docker image' ;;
    zh-CN:version) printf 'CPAMP 版本' ;;
    en-US:version) printf 'CPAMP version' ;;
    zh-CN:usage_db_path) printf '使用数据库路径' ;;
    en-US:usage_db_path) printf 'Usage database path' ;;
    zh-CN:cpa_conn_mode) printf 'CPA 来源: 1) 现在连接已有 CPA  2) 首次打开面板时选择下载最新版或连接已有 CPA' ;;
    en-US:cpa_conn_mode) printf 'CPA source: 1) connect an existing CPA now  2) choose latest or existing CPA in the setup UI' ;;
    zh-CN:cpa_conn_mode_preserve) printf 'CPA 来源: 1) 现在连接已有 CPA  2) 首次打开面板时选择下载最新版或连接已有 CPA  3) 沿用现有 CPA 连接' ;;
    en-US:cpa_conn_mode_preserve) printf 'CPA source: 1) connect an existing CPA now  2) choose latest or existing CPA in the setup UI  3) preserve the current CPA connection' ;;
    zh-CN:cpa_url) printf 'CPA 地址' ;;
    en-US:cpa_url) printf 'CPA URL' ;;
    zh-CN:cpa_key) printf 'CPA Management Key' ;;
    en-US:cpa_key) printf 'CPA Management Key' ;;
    zh-CN:summary) printf '安装摘要' ;;
    en-US:summary) printf 'Install summary' ;;
    zh-CN:install_mode_label) printf '安装范围' ;;
    en-US:install_mode_label) printf 'Install scope' ;;
    zh-CN:deploy_method_label) printf '部署方式' ;;
    en-US:deploy_method_label) printf 'Deployment' ;;
    zh-CN:directory_label) printf '安装目录' ;;
    en-US:directory_label) printf 'Directory' ;;
    zh-CN:stack_mode) printf 'CPA + CPAMP 完整安装' ;;
    en-US:stack_mode) printf 'CPA + CPAMP stack' ;;
    zh-CN:cpamp_mode) printf '仅安装 CPAMP' ;;
    en-US:cpamp_mode) printf 'CPAMP only' ;;
    zh-CN:docker_method) printf 'Docker' ;;
    en-US:docker_method) printf 'Docker' ;;
    zh-CN:native_method) printf '二进制/native' ;;
    en-US:native_method) printf 'Native binary' ;;
    zh-CN:cpa_connection_label) printf 'CPA 连接配置' ;;
    en-US:cpa_connection_label) printf 'CPA connection' ;;
    zh-CN:cpa_connection_setup) printf '由 Slim 初始化向导选择' ;;
    en-US:cpa_connection_setup) printf 'choose in the Slim setup wizard' ;;
    zh-CN:cpa_connection_env) printf '使用安装器验证并管理的已有 CPA' ;;
    en-US:cpa_connection_env) printf 'use an installer-validated existing CPA' ;;
    zh-CN:cpa_connection_preserve) printf '保留现有连接并由新版自动迁移' ;;
    en-US:cpa_connection_preserve) printf 'preserve the existing connection for automatic migration' ;;
    zh-CN:cpa_url_for_cpamp) printf 'CPAMP 使用的 CPA 地址' ;;
    en-US:cpa_url_for_cpamp) printf 'CPA URL for CPAMP' ;;
    zh-CN:confirm) printf '确认执行? 输入 confirm 执行，modify 修改，abort 退出' ;;
    en-US:confirm) printf 'Proceed? Enter confirm to install, modify to change, abort to exit' ;;
    zh-CN:dry_run) printf 'Dry-run：不会写入文件或执行安装命令。' ;;
    en-US:dry_run) printf 'Dry run: no files will be written and no install commands will run.' ;;
    zh-CN:write_file) printf '将写入文件' ;;
    en-US:write_file) printf 'Will write file' ;;
    zh-CN:run_command) printf '将执行命令' ;;
    en-US:run_command) printf 'Will run command' ;;
    zh-CN:done) printf '安装步骤已完成' ;;
    en-US:done) printf 'Install steps completed' ;;
    zh-CN:dry_run_done) printf 'Dry-run 计划预览完成，未写入文件或启动服务' ;;
    en-US:dry_run_done) printf 'Dry-run plan completed; no files were written and no services were started' ;;
    zh-CN:config_done) printf '部署配置已生成，服务尚未启动' ;;
    en-US:config_done) printf 'Deployment config generated; services have not been started' ;;
    zh-CN:operation_skipped) printf '已保留现有部署，按要求跳过升级或修复命令' ;;
    en-US:operation_skipped) printf 'Existing deployment preserved; upgrade or repair commands were skipped' ;;
    zh-CN:open_panel) printf '打开面板' ;;
    en-US:open_panel) printf 'Open panel' ;;
    zh-CN:admin_key) printf 'CPAMP 管理员密钥' ;;
    en-US:admin_key) printf 'CPAMP Admin Key' ;;
    zh-CN:admin_key_file) printf '管理员密钥文件' ;;
    en-US:admin_key_file) printf 'Admin key file' ;;
    zh-CN:cpa_key_file) printf 'CPA Management Key 文件' ;;
    en-US:cpa_key_file) printf 'CPA Management Key file' ;;
    zh-CN:demo_client_key_file) printf '演示客户端 API Key 文件' ;;
    en-US:demo_client_key_file) printf 'Demo client API key file' ;;
    zh-CN:systemd_file) printf 'systemd service 文件' ;;
    en-US:systemd_file) printf 'systemd service file' ;;
    zh-CN:next_setup) printf '首次打开面板后，选择下载最新版 CPA 或连接已有 CPA，然后在 UI 中设置 CPAMP 管理密钥。' ;;
    en-US:next_setup) printf 'Open the panel, choose the latest CPA or an existing CPA, then set the CPAMP Admin Key in the UI.' ;;
    zh-CN:next_full_stack) printf 'CPA 连接已由安装器配置并验证。首次打开面板时只需在 UI 中设置 CPAMP 管理密钥。' ;;
    en-US:next_full_stack) printf 'The installer configured the CPA connection. On first open, only set the CPAMP Admin Key in the UI.' ;;
    zh-CN:next_env_managed) printf '已有 CPA 连接已验证并由安装目录管理。首次打开面板时只需在 UI 中设置 CPAMP 管理密钥。' ;;
    en-US:next_env_managed) printf 'The existing CPA connection was validated and is managed by the install directory. On first open, only set the CPAMP Admin Key in the UI.' ;;
    zh-CN:next_native_preserved) printf '旧版数据、管理员凭证和 CPA 连接均已保留；新版会在首次启动时自动完成运行时迁移。' ;;
    en-US:next_native_preserved) printf 'Existing data, admin credentials, and the CPA connection were preserved; the new runtime migrates them on first start.' ;;
    zh-CN:detected_cpa) printf '检测到可用的 CPA 服务' ;;
    en-US:detected_cpa) printf 'Detected an available CPA service' ;;
    zh-CN:use_detected_cpa) printf '是否使用检测到的 CPA 作为服务端? 输入 yes/no' ;;
    en-US:use_detected_cpa) printf 'Use the detected CPA as the backend? Enter yes/no' ;;
    zh-CN:cpa_key_valid) printf 'CPA Management Key 验证通过' ;;
    en-US:cpa_key_valid) printf 'CPA Management Key validation passed' ;;
    zh-CN:cpa_key_invalid) printf 'CPA Management Key 验证失败（401/403）。请确认远程管理密钥。' ;;
    en-US:cpa_key_invalid) printf 'CPA Management Key validation failed (401/403). Check the remote management key.' ;;
    zh-CN:cpa_unreachable) printf '无法访问 CPA Management API。请确认地址、端口和远程管理设置。' ;;
    en-US:cpa_unreachable) printf 'CPA Management API is unreachable. Check the address, port, and remote-management settings.' ;;
    zh-CN:bootstrap_token) printf '一次性初始化令牌' ;;
    en-US:bootstrap_token) printf 'One-time bootstrap token' ;;
    zh-CN:bootstrap_hint) printf '首次初始化需要此令牌；请勿分享包含它的日志或截图。' ;;
    en-US:bootstrap_hint) printf 'First-time setup requires this token. Do not share logs or screenshots containing it.' ;;
    zh-CN:bootstrap_command) printf '如果未显示令牌，请从服务日志中查找 "one-time bootstrap token"。' ;;
    en-US:bootstrap_command) printf 'If no token is shown, find "one-time bootstrap token" in the service log.' ;;
    zh-CN:skip_execute) printf '已生成配置，但按要求跳过启动命令。' ;;
    en-US:skip_execute) printf 'Configuration was generated, but start commands were skipped as requested.' ;;
    zh-CN:port_busy) printf '端口可能已被占用' ;;
    en-US:port_busy) printf 'Port may already be in use' ;;
    zh-CN:missing_command) printf '缺少命令' ;;
    en-US:missing_command) printf 'Missing command' ;;
    zh-CN:existing_install) printf '检测到已有 CPA Manager Plus 部署' ;;
    en-US:existing_install) printf 'Existing CPA Manager Plus deployment detected' ;;
    zh-CN:existing_volume) printf '检测到已有 Docker 数据卷' ;;
    en-US:existing_volume) printf 'Existing Docker data volume detected' ;;
    zh-CN:select_existing_action) printf '选择操作: 1) 升级现有部署  2) 修复管理员登录  3) 重新生成配置  4) 退出' ;;
    en-US:select_existing_action) printf 'Select action: 1) upgrade existing deployment  2) repair admin login  3) regenerate config  4) exit' ;;
    zh-CN:select_native_action) printf '检测到已有原生二进制部署。选择操作: 1) 原位升级并保留数据  2) 重新生成配置  3) 退出' ;;
    en-US:select_native_action) printf 'Existing native deployment detected. Select action: 1) in-place upgrade and keep data  2) regenerate config  3) exit' ;;
    zh-CN:select_partial_action) printf '检测到不完整的部署配置。选择操作: 1) 备份并重新生成配置  2) 退出' ;;
    en-US:select_partial_action) printf 'Incomplete deployment config detected. Select action: 1) back up and regenerate config  2) exit' ;;
    zh-CN:select_orphan_action) printf '安装目录缺少配置，但发现旧数据卷。选择操作: 1) 修复并继续使用旧数据  2) 使用新项目名全新安装（旧服务仍运行时请改端口）  3) 退出' ;;
    en-US:select_orphan_action) printf 'The install directory has no config, but an old data volume exists. Select action: 1) repair and keep old data  2) fresh install with a new project name (choose different ports if the old service is still running)  3) exit' ;;
    zh-CN:operation_upgrade) printf '升级现有部署' ;;
    en-US:operation_upgrade) printf 'Upgrade existing deployment' ;;
    zh-CN:operation_repair) printf '修复管理员登录' ;;
    en-US:operation_repair) printf 'Repair admin login' ;;
    zh-CN:operation_regenerate) printf '重新生成部署配置' ;;
    en-US:operation_regenerate) printf 'Regenerate deployment config' ;;
    zh-CN:operation_install) printf '首次安装' ;;
    en-US:operation_install) printf 'Fresh install' ;;
    zh-CN:operation_label) printf '执行操作' ;;
    en-US:operation_label) printf 'Operation' ;;
    zh-CN:noninteractive_existing) printf '检测到已有部署。非交互模式必须设置 CPAMP_OPERATION=upgrade、repair 或 regenerate。' ;;
    en-US:noninteractive_existing) printf 'An existing deployment was detected. Non-interactive mode requires CPAMP_OPERATION=upgrade, repair, or regenerate.' ;;
    zh-CN:noninteractive_native) printf '检测到已有原生二进制部署。非交互模式必须设置 CPAMP_OPERATION=upgrade 或 regenerate。' ;;
    en-US:noninteractive_native) printf 'An existing native deployment was detected. Non-interactive mode requires CPAMP_OPERATION=upgrade or regenerate.' ;;
    zh-CN:topology_conversion) printf '已有部署的升级和配置重新生成必须保持原部署方式、安装范围和 CPA 来源。如需转换拓扑，请使用新的安装目录。' ;;
    en-US:topology_conversion) printf 'Existing deployment upgrades and regeneration must keep the deployment method, install scope, and CPA source. Use a new install directory for topology conversion.' ;;
    zh-CN:orphan_noninteractive) printf '检测到旧 Docker 数据卷但安装目录缺少配置。请设置 CPAMP_OPERATION=repair，或设置新的 CPAMP_PROJECT_NAME 后使用 CPAMP_OPERATION=install。' ;;
    en-US:orphan_noninteractive) printf 'An old Docker data volume exists but the install directory has no config. Set CPAMP_OPERATION=repair, or choose a new CPAMP_PROJECT_NAME with CPAMP_OPERATION=install.' ;;
    zh-CN:orphan_mode_required) printf '非交互修复旧数据卷时必须设置 CPAMP_INSTALL_MODE=stack 或 cpamp，避免创建错误的服务组合。' ;;
    en-US:orphan_mode_required) printf 'Non-interactive orphan-volume repair requires CPAMP_INSTALL_MODE=stack or cpamp to avoid creating the wrong service combination.' ;;
    zh-CN:repair_skip_execute) printf '旧数据卷修复必须执行数据库同步，不能使用 CPAMP_SKIP_EXECUTE=1；如只想预览，请使用 CPAMP_DRY_RUN=1。' ;;
    en-US:repair_skip_execute) printf 'Orphan-volume repair must execute the database sync and cannot use CPAMP_SKIP_EXECUTE=1; use CPAMP_DRY_RUN=1 for a preview.' ;;
    zh-CN:repairing_admin) printf '正在把数据库管理员凭证同步为安装目录中的管理员密钥' ;;
    en-US:repairing_admin) printf 'Synchronizing the database admin credential with the install-directory admin key' ;;
    zh-CN:auth_verified) printf '管理员密钥验证通过' ;;
    en-US:auth_verified) printf 'Admin key verification passed' ;;
    zh-CN:auth_failed) printf 'CPAMP 已启动，但管理员密钥验证失败。数据库凭证可能与安装目录中的密钥不一致。' ;;
    en-US:auth_failed) printf 'CPAMP started, but admin key verification failed. The database credential may not match the install-directory key.' ;;
    zh-CN:auth_repair_prompt) printf '是否停止 CPAMP 并自动修复管理员登录? 输入 yes/no' ;;
    en-US:auth_repair_prompt) printf 'Stop CPAMP and repair the admin login automatically? Enter yes/no' ;;
    zh-CN:health_failed) printf 'CPAMP 容器未能在规定时间内通过健康检查。' ;;
    en-US:health_failed) printf 'The CPAMP container did not become healthy in time.' ;;
    zh-CN:key_saved) printf '管理员密钥已保存' ;;
    en-US:key_saved) printf 'Admin key saved' ;;
    zh-CN:key_view_command) printf '查看管理员密钥' ;;
    en-US:key_view_command) printf 'View admin key' ;;
    zh-CN:key_reveal_prompt) printf '现在在终端显示完整管理员密钥吗? 请勿分享包含密钥的截图。输入 yes/no' ;;
    en-US:key_reveal_prompt) printf 'Show the full admin key in the terminal now? Do not share screenshots containing it. Enter yes/no' ;;
    zh-CN:config_backup) printf '旧配置已备份到' ;;
    en-US:config_backup) printf 'Previous config backed up to' ;;
    zh-CN:project_name) printf 'Docker Compose 项目名' ;;
    en-US:project_name) printf 'Docker Compose project name' ;;
    zh-CN:project_name_empty) printf 'Docker Compose 项目名不能为空。' ;;
    en-US:project_name_empty) printf 'Docker Compose project name must not be empty.' ;;
    zh-CN:docker_unavailable) printf 'Docker daemon 不可用。请启动 Docker 后重新运行安装器。' ;;
    en-US:docker_unavailable) printf 'Docker daemon is not available. Start Docker and run the installer again.' ;;
    zh-CN:missing_config) printf '已有安装配置不完整，请选择重新生成配置。' ;;
    en-US:missing_config) printf 'The existing install directory is incomplete. Choose config regeneration.' ;;
    zh-CN:repair_failed) printf '管理员密钥修复失败，CPAMP 已使用原数据库凭证重新启动。' ;;
    en-US:repair_failed) printf 'Admin key repair failed. CPAMP was restarted with the previous database credential.' ;;
    zh-CN:repair_restart_failed) printf '管理员密钥已重置，但 CPAMP 重启失败。请在安装目录执行 docker compose up -d。' ;;
    en-US:repair_restart_failed) printf 'Admin key reset succeeded, but CPAMP failed to restart. Run docker compose up -d from the install directory.' ;;
    zh-CN:repair_verify_failed) printf '管理员密钥修复后验证仍失败，请确认面板和修复命令使用同一个 Docker 数据卷。' ;;
    en-US:repair_verify_failed) printf 'Admin key repair completed, but verification still failed. Confirm that the panel and repair command use the same Docker volume.' ;;
    zh-CN:native_upgrade_rolled_back) printf '新版本未能健康启动，已恢复旧版启动脚本并重新启动旧版本。' ;;
    en-US:native_upgrade_rolled_back) printf 'The new version did not become healthy. The previous run script was restored and the old version was restarted.' ;;
    zh-CN:native_upgrade_manual_stop) printf '检测到安装器之外的进程正在占用旧端口。请先停止 systemd 或其他进程管理器中的 CPAMP，再重新执行升级。' ;;
    en-US:native_upgrade_manual_stop) printf 'A process outside the installer is using the previous port. Stop CPAMP in systemd or the other process manager, then rerun the upgrade.' ;;
    zh-CN:docker_upgrade_rolled_back) printf '新版 Docker 服务未能健康启动，已恢复旧镜像并重新通过健康检查。' ;;
    en-US:docker_upgrade_rolled_back) printf 'The upgraded Docker service did not become healthy. The previous image was restored and passed the health check.' ;;
    zh-CN:docker_upgrade_rollback_failed) printf '新版 Docker 服务未能健康启动，且自动回滚失败。请检查 Compose 日志和本地镜像后再重试。' ;;
    en-US:docker_upgrade_rollback_failed) printf 'The upgraded Docker service did not become healthy, and automatic rollback failed. Inspect the Compose logs and local images before retrying.' ;;
    zh-CN:docker_upgrade_rollback_unavailable) printf '无法确认可用的旧 Docker 镜像，或 Compose 使用了不可重新标记的 digest 引用。为避免无法回滚，安装器已在拉取前停止；请先恢复并启动旧服务，或使用 regenerate 明确重建配置。' ;;
    en-US:docker_upgrade_rollback_unavailable) printf 'A usable previous Docker image could not be confirmed, or Compose uses an immutable digest reference. The installer stopped before pulling to preserve rollback safety. Restore and start the old service first, or explicitly regenerate the deployment config.' ;;
    zh-CN:legacy_docker_ports) printf '当前 Compose 未声明 CPAMP_PANEL_PORT，安装器已原样保留端口映射并继续使用旧入口生成完成提示。若尚未手动映射 18137 和 8137，请在确认备份和自定义配置后使用 CPAMP_OPERATION=regenerate。' ;;
    en-US:legacy_docker_ports) printf 'The preserved Compose file does not declare CPAMP_PANEL_PORT, so the installer kept its mappings unchanged and used the old entry in the completion URL. If 18137 and 8137 are not already mapped manually, review the backup and custom config before using CPAMP_OPERATION=regenerate.' ;;
    *) printf '%s' "$1" ;;
  esac
}

say() {
  printf '%s\n' "$*"
}

require_interactive_tty() {
  if [ "$non_interactive" = "1" ]; then
    return
  fi

  if [ ! -t 0 ]; then
    die "Interactive install requires a terminal on stdin. Download the script and run it with bash, or set CPAMP_NON_INTERACTIVE=1 CPAMP_CONFIRM=1."
  fi

  if [ ! -r /dev/tty ] || [ ! -w /dev/tty ]; then
    die "Interactive install requires access to /dev/tty. Run it from a terminal, or set CPAMP_NON_INTERACTIVE=1 CPAMP_CONFIRM=1."
  fi
}

prompt_line() {
  local prompt="$1"
  local default="$2"
  local answer=""

  if [ "$non_interactive" = "1" ]; then
    printf '%s\n' "$default"
    return
  fi

  if [ -n "$default" ]; then
    printf '%s [%s]: ' "$prompt" "$default" >&2
  else
    printf '%s: ' "$prompt" >&2
  fi
  IFS= read -r answer
  if [ -z "$answer" ]; then
    answer="$default"
  fi
  printf '%s\n' "$answer"
}

prompt_secret() {
  local prompt="$1"
  local env_value="$2"
  local answer=""

  if [ "$non_interactive" = "1" ]; then
    printf '%s\n' "$env_value"
    return
  fi

  printf '%s: ' "$prompt" >&2
  IFS= read -r -s answer
  printf '\n' >&2
  printf '%s\n' "$answer"
}

prompt_choice() {
  local prompt="$1"
  local default="$2"
  local allowed="$3"
  local answer=""

  if [ "$non_interactive" = "1" ]; then
    printf '%s\n' "$default"
    return
  fi

  while true; do
    answer="$(prompt_line "$prompt" "$default")"
    case " $allowed " in
      *" $answer "*) printf '%s\n' "$answer"; return ;;
      *) printf 'Invalid choice: %s\n' "$answer" >&2 ;;
    esac
  done
}

detect_environment() {
  os_name="$(uname -s 2>/dev/null || printf 'unknown')"
  arch_name="$(uname -m 2>/dev/null || printf 'unknown')"

  case "$os_name" in
    Linux) normalized_os="linux" ;;
    Darwin) normalized_os="darwin" ;;
    MINGW*|MSYS*|CYGWIN*) normalized_os="windows" ;;
    *) normalized_os="unknown" ;;
  esac

  case "$arch_name" in
    x86_64|amd64) normalized_arch="amd64" ;;
    arm64|aarch64) normalized_arch="arm64" ;;
    *) normalized_arch="unknown" ;;
  esac

  if [ -r /proc/version ] && grep -qiE 'microsoft|wsl' /proc/version 2>/dev/null; then
    is_wsl="true"
  fi
}

choose_language() {
  local choice=""

  if [ -n "$lang_code" ]; then
    case "$lang_code" in
      zh|zh-CN|cn) lang_code="zh-CN" ;;
      en|en-US) lang_code="en-US" ;;
      *) die "Unsupported CPAMP_LANG: $lang_code" ;;
    esac
    return
  fi

  printf 'Choose language / 选择语言:\n'
  printf '  1) 简体中文\n'
  printf '  2) English\n'
  choice="$(prompt_choice 'Language / 语言' '1' '1 2')"
  case "$choice" in
    2) lang_code="en-US" ;;
    *) lang_code="zh-CN" ;;
  esac
}

show_environment() {
  say "== $(text env_header) =="
  say "$(text os): ${os_name} (${normalized_os})"
  say "$(text arch): ${arch_name} (${normalized_arch})"
  say "$(text wsl): ${is_wsl}"
  if [ "$dry_run" = "1" ]; then
    say "$(text dry_run)"
  fi
}

confirm_environment() {
  local answer=""
  if [ "$non_interactive" = "1" ]; then
    return
  fi
  answer="$(prompt_choice "$(text continue)" "yes" "yes no")"
  [ "$answer" = "yes" ] || exit 0
}

expand_path() {
  case "$1" in
    "~") printf '%s\n' "${HOME:-.}" ;;
    "~/"*) printf '%s/%s\n' "${HOME:-.}" "${1#~/}" ;;
    *) printf '%s\n' "$1" ;;
  esac
}

validate_project_name() {
  local value="$1"
  validate_single_line "$(text project_name)" "$value"
  case "$value" in
    [-_]*|*[!a-z0-9_-]*) die "$(text project_name) contains unsupported characters." ;;
    *) ;;
  esac
}

read_env_value() {
  local file="$1"
  local key="$2"
  local line=""
  local value=""
  local found="0"

  [ -f "$file" ] || return 1
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      "$key="*) value="${line#*=}"; found="1" ;;
    esac
  done < "$file"
  if [ "$found" = "1" ]; then
    printf '%s\n' "$value"
    return 0
  fi
  return 1
}

read_shell_export_value() {
  local file="$1"
  local key="$2"
  local line=""
  local value=""

  [ -f "$file" ] || return 1
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      "export $key="*)
        value="${line#export $key=}"
        decode_shell_word "$value"
        return 0
        ;;
    esac
  done < "$file"
  return 1
}

decode_shell_word() {
  local value="$1"
  local result=""
  local char=""

  if [ "${#value}" -ge 2 ] && [ "${value:0:1}" = "'" ] && [ "${value: -1}" = "'" ]; then
    printf '%s\n' "${value:1:${#value}-2}"
    return
  fi
  if [ "${#value}" -ge 2 ] && [ "${value:0:1}" = '"' ] && [ "${value: -1}" = '"' ]; then
    value="${value:1:${#value}-2}"
  fi
  while [ -n "$value" ]; do
    char="${value:0:1}"
    value="${value:1}"
    if [ "$char" = "\\" ] && [ -n "$value" ]; then
      char="${value:0:1}"
      value="${value:1}"
    fi
    result="${result}${char}"
  done
  printf '%s\n' "$result"
}

read_shell_cd_value() {
  local file="$1"
  local line=""

  [ -f "$file" ] || return 1
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      "cd "*)
        decode_shell_word "${line#cd }"
        return 0
        ;;
    esac
  done < "$file"
  return 1
}

resolve_native_path() {
  local value=""
  local base="$2"

  value="$(expand_path "$1")"

  case "$value" in
    /*) printf '%s\n' "$value" ;;
    *) printf '%s/%s\n' "${base%/}" "$value" ;;
  esac
}

read_json_string_value() {
  local file="$1"
  local key="$2"

  [ -f "$file" ] || return 1
  sed -n "s/^[[:space:]]*\"${key}\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\\1/p" "$file" | tail -n 1
}

latest_native_config_file() {
  local candidate=""
  local latest=""

  for candidate in "$install_dir"/runtime/*/config.json; do
    [ -f "$candidate" ] || continue
    if [ -z "$latest" ] || [ "$candidate" -nt "$latest" ]; then
      latest="$candidate"
    fi
  done
  [ -n "$latest" ] || return 1
  printf '%s\n' "$latest"
}

collect_install_directory() {
  install_dir="$(expand_path "$(prompt_line "$(text install_dir)" "${CPAMP_INSTALL_DIR:-$default_install_dir}")")"
  validate_single_line "$(text install_dir)" "$install_dir"
}

docker_volume_exists() {
  local volume="$1"
  command_exists docker && docker volume inspect "$volume" >/dev/null 2>&1
}

detect_existing_installation() {
  local configured_project=""
  local has_docker_files="0"
  local has_native_files="0"

  if configured_project="$(read_env_value "$install_dir/.env" COMPOSE_PROJECT_NAME 2>/dev/null)"; then
    [ -n "$configured_project" ] || die "$(text project_name_empty)"
    compose_project_name="$configured_project"
  fi
  validate_project_name "$compose_project_name"
  existing_volume_name="${compose_project_name}_cpa-manager-plus-data"

  if [ -e "$install_dir/.env" ] ||
     [ -e "$install_dir/compose.yaml" ] ||
     [ -e "$install_dir/cliproxyapi/config.yaml" ]; then
    has_docker_files="1"
  fi
  if [ -e "$install_dir/run.sh" ] ||
     [ -e "$install_dir/cpa-manager-plus.service" ] ||
     [ -e "$install_dir/cpa-manager-plus.pid" ] ||
     [ -e "$install_dir/data/usage.sqlite" ]; then
    has_native_files="1"
  fi

  if [ -f "$install_dir/.env" ] && [ -f "$install_dir/compose.yaml" ]; then
    existing_install_state="managed"
  elif [ "$has_docker_files" = "1" ]; then
    existing_install_state="partial"
  elif [ "$has_native_files" = "1" ]; then
    existing_install_state="native-managed"
  elif docker_volume_exists "$existing_volume_name"; then
    existing_install_state="orphan-volume"
  elif [ -e "$install_dir/secrets/cpamp-admin-key" ]; then
    existing_install_state="partial"
  else
    existing_install_state="fresh"
  fi

  if [ -e "$install_dir/secrets/cpamp-admin-key" ]; then
    read_existing_secret "$install_dir/secrets/cpamp-admin-key" >/dev/null
  fi
}

normalize_operation() {
  case "$operation" in
    '') ;;
    install|new|fresh) operation="install" ;;
    upgrade|update) operation="upgrade" ;;
    repair|recover|reset-admin-key) operation="repair" ;;
    regenerate|overwrite|reconfigure) operation="regenerate" ;;
    *) die "Unsupported CPAMP_OPERATION: $operation" ;;
  esac
}

choose_new_project_name() {
  compose_project_name="$(prompt_line "$(text project_name)" "${CPAMP_PROJECT_NAME:-cpamp-new}")"
  validate_project_name "$compose_project_name"
  existing_volume_name="${compose_project_name}_cpa-manager-plus-data"
  if docker_volume_exists "$existing_volume_name"; then
    die "Docker volume already exists: $existing_volume_name"
  fi
  operation="install"
  existing_install_state="fresh"
}

resolve_operation() {
  local choice=""

  normalize_operation
  if [ "$existing_install_state" = "fresh" ]; then
    [ -n "$operation" ] || operation="install"
    [ "$operation" = "install" ] || die "CPAMP_OPERATION=$operation requires an existing Docker deployment."
    return
  fi

  if [ "$existing_install_state" = "native-managed" ]; then
    say ""
    say "== $(text existing_install) =="
    say "$(text directory_label): $install_dir"
    if [ -z "$operation" ]; then
      if [ "$non_interactive" = "1" ]; then
        die "$(text noninteractive_native)"
      fi
      choice="$(prompt_choice "$(text select_native_action)" "1" "1 2 3")"
      case "$choice" in
        1) operation="upgrade" ;;
        2) operation="regenerate" ;;
        3) exit 0 ;;
      esac
    fi
    case "$operation" in
      upgrade|regenerate) return ;;
      *) die "CPAMP_OPERATION=$operation is not valid for an existing native deployment." ;;
    esac
  fi

  if [ -z "$operation" ] && [ "${CPAMP_OVERWRITE:-0}" = "1" ]; then
    operation="regenerate"
  fi

  if [ "$existing_install_state" = "orphan-volume" ]; then
    say ""
    say "== $(text existing_volume) =="
    say "$(text project_name): $compose_project_name"
    say "Docker volume: $existing_volume_name"
    if [ -z "$operation" ]; then
      if [ "$non_interactive" = "1" ]; then
        die "$(text orphan_noninteractive)"
      fi
      choice="$(prompt_choice "$(text select_orphan_action)" "1" "1 2 3")"
      case "$choice" in
        1) operation="repair" ;;
        2) choose_new_project_name; return ;;
        3) exit 0 ;;
      esac
    fi
    case "$operation" in
      repair)
        if [ "$non_interactive" = "1" ] && [ -z "${CPAMP_INSTALL_MODE:-}" ]; then
          die "$(text orphan_mode_required)"
        fi
        if [ "$skip_execute" = "1" ] && [ "$dry_run" != "1" ]; then
          die "$(text repair_skip_execute)"
        fi
        return
        ;;
      install) die "$(text orphan_noninteractive)" ;;
      *) die "CPAMP_OPERATION=$operation is not available without an existing compose.yaml." ;;
    esac
    return
  fi

  if [ "$existing_install_state" = "partial" ]; then
    say ""
    say "== $(text existing_install) =="
    say "$(text directory_label): $install_dir"
    if [ -z "$operation" ]; then
      if [ "$non_interactive" = "1" ]; then
        die "$(text noninteractive_existing)"
      fi
      choice="$(prompt_choice "$(text select_partial_action)" "1" "1 2")"
      case "$choice" in
        1) operation="regenerate" ;;
        2) exit 0 ;;
      esac
    fi
    [ "$operation" = "regenerate" ] || die "$(text missing_config) Set CPAMP_OPERATION=regenerate to rebuild its config."
    return
  fi

  say ""
  say "== $(text existing_install) =="
  say "$(text directory_label): $install_dir"
  say "$(text project_name): $compose_project_name"
  if [ -z "$operation" ]; then
    if [ "$non_interactive" = "1" ]; then
      die "$(text noninteractive_existing)"
    fi
    choice="$(prompt_choice "$(text select_existing_action)" "1" "1 2 3 4")"
    case "$choice" in
      1) operation="upgrade" ;;
      2) operation="repair" ;;
      3) operation="regenerate" ;;
      4) exit 0 ;;
    esac
  fi

  case "$operation" in
    upgrade|repair)
      if [ "$existing_install_state" != "managed" ]; then
        die "$(text missing_config) Set CPAMP_OPERATION=regenerate to rebuild its config."
      fi
      ;;
    regenerate) ;;
    *) die "CPAMP_OPERATION=$operation is not valid for an existing deployment." ;;
  esac
}

read_existing_secret() {
  local file="$1"
  local value=""
  [ -f "$file" ] || return 1
  if [ "$dry_run" != "1" ]; then
    chmod 600 "$file" 2>/dev/null || die "Unable to restrict secret file permissions: $file"
  fi
  value="$(< "$file")"
  value="${value%$'\r'}"
  validate_secret_value "$file" "$value"
  printf '%s\n' "$value"
}

load_existing_docker_config() {
  local value=""

  [ -f "$install_dir/.env" ] || die "Missing existing config: $install_dir/.env"
  [ -f "$install_dir/compose.yaml" ] || die "Missing existing config: $install_dir/compose.yaml"
  deploy_method="docker"
  cpamp_image="$(read_env_value "$install_dir/.env" CPAMP_IMAGE 2>/dev/null || printf '%s' "$default_cpamp_image")"
  validate_image_ref "$(text cpamp_image)" "$cpamp_image"
  cpamp_port="$(read_env_value "$install_dir/.env" CPAMP_PORT 2>/dev/null || printf '18317')"
  normalize_port "$cpamp_port" || die "Invalid CPAMP port in existing .env: $cpamp_port"
  cpamp_api_port="$(read_env_value "$install_dir/.env" CPAMP_API_PORT 2>/dev/null || printf '8137')"
  normalize_port "$cpamp_api_port" || die "Invalid CPAMP API port in existing .env: $cpamp_api_port"
  if value="$(read_env_value "$install_dir/.env" CPAMP_PANEL_PORT 2>/dev/null)"; then
    cpamp_panel_port="$value"
    existing_docker_panel_port_configured="1"
  else
    cpamp_panel_port="18137"
  fi
  normalize_port "$cpamp_panel_port" || die "Invalid CPAMP panel port in existing .env: $cpamp_panel_port"
  if grep -q '^[[:space:]]*cli-proxy-api:' "$install_dir/compose.yaml"; then
    install_mode="stack"
    cpa_image="$(read_env_value "$install_dir/.env" CPA_IMAGE 2>/dev/null || printf '%s' "$default_cpa_image")"
    validate_image_ref "$(text cpa_image)" "$cpa_image"
    cpa_port="$(read_env_value "$install_dir/.env" CPA_PORT 2>/dev/null || printf '8317')"
    normalize_port "$cpa_port" || die "Invalid CPA port in existing .env: $cpa_port"
    cpa_url="http://cli-proxy-api:8317"
    cpa_connection_mode="env"
    deployment_mode="installer-managed"
  else
    install_mode="cpamp"
    if grep -q 'CPA_MANAGEMENT_KEY_FILE:' "$install_dir/compose.yaml"; then
      cpa_connection_mode="env"
      cpa_url="$(read_env_value "$install_dir/.env" CPA_UPSTREAM_URL 2>/dev/null || true)"
      deployment_mode="installer-managed"
    else
      cpa_connection_mode="setup"
      deployment_mode="slim"
    fi
  fi
  if value="$(read_existing_secret "$install_dir/secrets/cpamp-admin-key")"; then
    admin_key="$value"
  elif [ "$operation" = "repair" ]; then
    admin_secret_missing="1"
  fi
  existing_deploy_method="$deploy_method"
  existing_install_mode="$install_mode"
  existing_cpa_connection_mode="$cpa_connection_mode"
}

load_existing_native_config() {
  local metadata_file="$install_dir/.cpamp-native.env"
  local legacy_config=""
  local legacy_http_addr=""
  local gateway_addrs=""
  local first_addr=""
  local second_addr=""
  local third_addr=""
  local configured_mode=""
  local configured_install_mode=""
  local configured_cpa_url=""
  local configured_db_path=""
  local run_work_dir=""
  local legacy_db_path=""

  deploy_method="native"
  cpamp_version="${CPAMP_VERSION:-latest}"
  cpamp_api_port="8137"
  cpamp_panel_port="18137"
  cpamp_port="18317"
  existing_native_health_port="18317"
  existing_native_binary_dir=""

  if [ -f "$install_dir/run.sh" ]; then
    existing_native_binary_dir="$(read_shell_cd_value "$install_dir/run.sh" 2>/dev/null || true)"
    if [ -n "$existing_native_binary_dir" ]; then
      existing_native_binary_dir="$(resolve_native_path "$existing_native_binary_dir" "$install_dir")"
      if [ ! -f "$existing_native_binary_dir/cpa-manager-plus" ]; then
        existing_native_binary_dir=""
      fi
    fi
  fi

  configured_install_mode="$(read_env_value "$metadata_file" CPAMP_INSTALL_MODE 2>/dev/null || true)"
  configured_mode="$(read_env_value "$metadata_file" CPA_MANAGER_DEPLOYMENT_MODE 2>/dev/null || true)"
  configured_cpa_url="$(read_env_value "$metadata_file" CPA_UPSTREAM_URL 2>/dev/null || true)"
  configured_db_path="$(read_env_value "$metadata_file" USAGE_DB_PATH 2>/dev/null || true)"
  cpamp_api_port="$(read_env_value "$metadata_file" CPAMP_API_PORT 2>/dev/null || printf '%s' "$cpamp_api_port")"
  cpamp_panel_port="$(read_env_value "$metadata_file" CPAMP_PANEL_PORT 2>/dev/null || printf '%s' "$cpamp_panel_port")"
  cpamp_port="$(read_env_value "$metadata_file" CPAMP_PORT 2>/dev/null || printf '%s' "$cpamp_port")"

  if [ -z "$configured_mode" ]; then
    configured_mode="$(read_shell_export_value "$install_dir/run.sh" CPA_MANAGER_DEPLOYMENT_MODE 2>/dev/null || true)"
  fi
  if [ -z "$configured_cpa_url" ]; then
    configured_cpa_url="$(read_shell_export_value "$install_dir/run.sh" CPA_UPSTREAM_URL 2>/dev/null || true)"
  fi
  gateway_addrs="$(read_shell_export_value "$install_dir/run.sh" CPA_MANAGER_GATEWAY_ADDRS 2>/dev/null || true)"
  if [ -z "$configured_db_path" ]; then
    configured_db_path="$(read_shell_export_value "$install_dir/run.sh" USAGE_DB_PATH 2>/dev/null || true)"
    if [ -n "$configured_db_path" ]; then
      run_work_dir="$(read_shell_cd_value "$install_dir/run.sh" 2>/dev/null || true)"
      [ -n "$run_work_dir" ] || run_work_dir="$install_dir"
      configured_db_path="$(resolve_native_path "$configured_db_path" "$run_work_dir")"
    fi
  else
    configured_db_path="$(resolve_native_path "$configured_db_path" "$install_dir")"
  fi
  if [ -n "$gateway_addrs" ]; then
    IFS=',' read -r first_addr second_addr third_addr <<< "$gateway_addrs"
    if [ -n "$first_addr" ] && [ -n "$second_addr" ] && [ -n "$third_addr" ]; then
      cpamp_api_port="${first_addr##*:}"
      cpamp_panel_port="${second_addr##*:}"
      cpamp_port="${third_addr##*:}"
    fi
  fi

  legacy_config="$(latest_native_config_file 2>/dev/null || true)"
  if [ -n "$legacy_config" ]; then
    legacy_http_addr="$(read_json_string_value "$legacy_config" httpAddr 2>/dev/null || true)"
    if [ -n "$legacy_http_addr" ]; then
      existing_native_health_port="${legacy_http_addr##*:}"
    fi
    if [ -z "$configured_cpa_url" ]; then
      configured_cpa_url="$(read_json_string_value "$legacy_config" cpaUpstreamUrl 2>/dev/null || true)"
    fi
    if [ -z "$configured_db_path" ]; then
      legacy_db_path="$(read_json_string_value "$legacy_config" dbPath 2>/dev/null || true)"
      if [ -n "$legacy_db_path" ]; then
        configured_db_path="$(resolve_native_path "$legacy_db_path" "$(dirname "$legacy_config")")"
      fi
    fi
  elif [ -n "$gateway_addrs" ]; then
    existing_native_health_port="$cpamp_panel_port"
  fi

  normalize_port "$cpamp_api_port" || die "Invalid CPAMP API port in existing native config: $cpamp_api_port"
  normalize_port "$cpamp_panel_port" || die "Invalid CPAMP panel port in existing native config: $cpamp_panel_port"
  normalize_port "$cpamp_port" || die "Invalid CPAMP compatibility port in existing native config: $cpamp_port"
  normalize_port "$existing_native_health_port" || existing_native_health_port="$cpamp_port"
  usage_db_path="${CPAMP_USAGE_DB_PATH:-$configured_db_path}"
  if [ -z "$usage_db_path" ]; then
    usage_db_path="$install_dir/data/usage.sqlite"
  else
    usage_db_path="$(resolve_native_path "$usage_db_path" "$install_dir")"
  fi
  validate_single_line "$(text usage_db_path)" "$usage_db_path"

  case "$configured_install_mode" in
    stack|full|all) install_mode="stack" ;;
    cpamp|manager|cpamp-only) install_mode="cpamp" ;;
  esac

  case "$configured_mode" in
    integrated)
      install_mode="stack"
      cpa_connection_mode="integrated"
      deployment_mode="integrated"
      ;;
    installer-managed)
      install_mode="cpamp"
      cpa_connection_mode="env"
      deployment_mode="installer-managed"
      ;;
    slim)
      install_mode="cpamp"
      cpa_connection_mode="preserve"
      deployment_mode="slim"
      ;;
    external)
      install_mode="cpamp"
      cpa_connection_mode="preserve"
      deployment_mode="external"
      ;;
  esac

  if [ -z "$install_mode" ]; then
    install_mode="cpamp"
  fi
  if [ -z "$cpa_connection_mode" ]; then
    if [ -n "$configured_cpa_url" ] && [ -f "$install_dir/secrets/cpa-management-key" ]; then
      cpa_connection_mode="env"
      deployment_mode="installer-managed"
    else
      cpa_connection_mode="preserve"
      deployment_mode="slim"
    fi
  fi
  if [ "$cpa_connection_mode" = "env" ]; then
    [ -n "$configured_cpa_url" ] || die "Existing native config is missing CPA_UPSTREAM_URL. Use CPAMP_OPERATION=regenerate to choose the CPA connection again."
    validate_url_value "$(text cpa_url)" "$configured_cpa_url"
    cpa_url="$(normalize_cpa_url "$configured_cpa_url")"
    cpa_validation_url="$cpa_url"
    cpa_management_key="$(read_existing_secret "$install_dir/secrets/cpa-management-key")"
  fi
  existing_deploy_method="$deploy_method"
  existing_install_mode="$install_mode"
  existing_cpa_connection_mode="$cpa_connection_mode"
}

ensure_repair_admin_key() {
  if [ -n "$admin_key" ]; then
    return 0
  fi
  generated_admin_key="cpamp_$(random_alnum 32)"
  admin_key="$(ensure_secret_file "$install_dir/secrets/cpamp-admin-key" "$generated_admin_key")"
  admin_secret_missing="0"
}

normalize_port() {
  local value="$1"
  case "$value" in
    ''|*[!0-9]*) return 1 ;;
    *)
      if [ "$value" -lt 1 ] || [ "$value" -gt 65535 ]; then
        return 1
      fi
      ;;
  esac
}

has_line_break() {
  case "$1" in
    *$'\n'*|*$'\r'*) return 0 ;;
    *) return 1 ;;
  esac
}

validate_single_line() {
  local label="$1"
  local value="$2"
  [ -n "$value" ] || die "$label must not be empty."
  if has_line_break "$value"; then
    die "$label must be a single line."
  fi
}

validate_secret_value() {
  validate_single_line "$1" "$2"
}

validate_url_value() {
  local label="$1"
  local value="$2"
  validate_single_line "$label" "$value"
  case "$value" in
    http://*|https://*) ;;
    *) die "$label must start with http:// or https://." ;;
  esac
  if [[ "$value" == *[[:space:]]* ||
        "$value" == *'#'* ||
        "$value" == *'?'* ||
        "$value" == *\"* ||
        "$value" == *"'"* ||
        "$value" == *\\* ||
        "$value" == *'$'* ||
        "$value" == *'`'* ]]; then
    die "$label contains unsupported characters."
  fi
}

normalize_cpa_url() {
  local value="$1"
  while [ "$value" != "/" ] && [ "${value%/}" != "$value" ]; do
    value="${value%/}"
  done
  printf '%s\n' "$value"
}

cpa_url_for_docker() {
  local value="$1"
  case "$value" in
    http://127.0.0.1|http://localhost) printf 'http://host.docker.internal\n' ;;
    https://127.0.0.1|https://localhost) printf 'https://host.docker.internal\n' ;;
    http://127.0.0.1:*) printf 'http://host.docker.internal:%s\n' "${value#http://127.0.0.1:}" ;;
    http://localhost:*) printf 'http://host.docker.internal:%s\n' "${value#http://localhost:}" ;;
    https://127.0.0.1:*) printf 'https://host.docker.internal:%s\n' "${value#https://127.0.0.1:}" ;;
    https://localhost:*) printf 'https://host.docker.internal:%s\n' "${value#https://localhost:}" ;;
    *) printf '%s\n' "$value" ;;
  esac
}

probe_cpa_service() {
  local base_url="$1"
  local status=""
  command_exists curl || return 1
  status="$(curl -sS --connect-timeout 1 --max-time 2 -o /dev/null -w '%{http_code}' "${base_url}/healthz" 2>/dev/null || true)"
  case "$status" in
    2??) return 0 ;;
  esac
  status="$(curl -sS --connect-timeout 1 --max-time 2 -o /dev/null -w '%{http_code}' "${base_url}/v0/management/config" 2>/dev/null || true)"
  case "$status" in
    2??|401|403) return 0 ;;
    *) return 1 ;;
  esac
}

detect_existing_cpa() {
  local candidates="${CPAMP_CPA_DETECT_URLS:-http://127.0.0.1:8317,http://localhost:8317}"
  local candidate=""
  local remaining="$candidates"

  if [ "${CPAMP_DISABLE_CPA_DETECTION:-0}" = "1" ]; then
    return
  fi
  if [ -n "${CPAMP_DETECTED_CPA_URL:-}" ]; then
    candidates="$CPAMP_DETECTED_CPA_URL"
    remaining="$candidates"
  fi
  while [ -n "$remaining" ]; do
    case "$remaining" in
      *,*) candidate="${remaining%%,*}"; remaining="${remaining#*,}" ;;
      *) candidate="$remaining"; remaining="" ;;
    esac
    candidate="$(normalize_cpa_url "${candidate//[[:space:]]/}")"
    [ -n "$candidate" ] || continue
    validate_url_value "$(text cpa_url)" "$candidate"
    if [ "$dry_run" = "1" ] && [ -n "${CPAMP_DETECTED_CPA_URL:-}" ]; then
      detected_cpa_url="$candidate"
      break
    fi
    if probe_cpa_service "$candidate"; then
      detected_cpa_url="$candidate"
      break
    fi
  done
  if [ "${CPAMP_USE_DETECTED_CPA:-0}" = "1" ] && [ -z "$detected_cpa_url" ]; then
    die "$(text cpa_unreachable)"
  fi
  return 0
}

validate_cpa_management_access() (
  local base_url="$1"
  local key="$2"
  local header_file=""
  local status=""
  local curl_status=0

  cleanup_cpa_header() {
    if [ -n "$header_file" ]; then
      rm -f "$header_file"
      header_file=""
    fi
  }

  trap cleanup_cpa_header EXIT
  trap 'cleanup_cpa_header; exit 129' HUP
  trap 'cleanup_cpa_header; exit 130' INT
  trap 'cleanup_cpa_header; exit 143' TERM

  if [ "$dry_run" = "1" ]; then
    say "$(text run_command): validate ${base_url}/v0/management/config"
    return
  fi
  require_command curl
  header_file="$(mktemp "${TMPDIR:-/tmp}/cpamp-cpa-header.XXXXXX")"
  chmod 600 "$header_file"
  printf 'Authorization: Bearer %s\n' "$key" > "$header_file"
  set +e
  status="$(curl -sS --connect-timeout 3 --max-time 10 -o /dev/null -w '%{http_code}' -H "@$header_file" "${base_url}/v0/management/config")"
  curl_status="$?"
  set -e
  cleanup_cpa_header
  if [ "$curl_status" -ne 0 ]; then
    die "$(text cpa_unreachable)"
  fi
  case "$status" in
    2??) say "$(text cpa_key_valid)" ;;
    401|403) die "$(text cpa_key_invalid)" ;;
    *) die "$(text cpa_unreachable) HTTP $status" ;;
  esac
)

collect_existing_cpa_connection() {
  local default_cpa_url="$1"
  local default_cpa_key="${CPAMP_CPA_MANAGEMENT_KEY:-}"
  if [ -z "$default_cpa_key" ] && [ -f "$install_dir/secrets/cpa-management-key" ]; then
    default_cpa_key="$(read_existing_secret "$install_dir/secrets/cpa-management-key")"
  fi
  cpa_validation_url="$(normalize_cpa_url "$(prompt_line "$(text cpa_url)" "${CPAMP_CPA_URL:-${cpa_validation_url:-$default_cpa_url}}")")"
  validate_url_value "$(text cpa_url)" "$cpa_validation_url"
  cpa_management_key="$(prompt_secret "$(text cpa_key)" "$default_cpa_key")"
  validate_secret_value "$(text cpa_key)" "$cpa_management_key"
  validate_cpa_management_access "$cpa_validation_url" "$cpa_management_key"
  cpa_url="$cpa_validation_url"
}

choose_detected_cpa() {
  local answer="no"
  [ -n "$detected_cpa_url" ] || return 0
  say ""
  say "== $(text detected_cpa) =="
  say "$(text cpa_url): $detected_cpa_url"
  if [ "$non_interactive" = "1" ]; then
    if [ "${CPAMP_USE_DETECTED_CPA:-0}" != "1" ]; then
      return 0
    fi
    answer="yes"
  else
    answer="$(prompt_choice "$(text use_detected_cpa)" "yes" "yes no")"
  fi
  [ "$answer" = "yes" ] || return 0
  case "${CPAMP_INSTALL_MODE:-}" in
    stack|full|all) die "CPAMP_USE_DETECTED_CPA conflicts with CPAMP_INSTALL_MODE=${CPAMP_INSTALL_MODE}." ;;
  esac
  install_mode="cpamp"
  cpa_connection_mode="env"
  cpa_validation_url="$detected_cpa_url"
  collect_existing_cpa_connection "$detected_cpa_url"
  return 0
}

validate_image_ref() {
  local label="$1"
  local value="$2"
  validate_single_line "$label" "$value"
  case "$value" in
    *[!A-Za-z0-9._/@:-]*)
      die "$label contains unsupported characters."
      ;;
  esac
}

validate_release_version() {
  local value="$1"
  case "$value" in
    ''|*[!A-Za-z0-9._+-]*|[._+-]*) die "CPAMP version contains unsupported characters: $value" ;;
  esac
}

yaml_double_quote_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '%s\n' "$value"
}

shell_quote() {
  printf '%q' "$1"
}

systemd_double_quote_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//%/%%}"
  printf '%s\n' "$value"
}

collect_choices() {
  local mode_choice=""
  local method_choice=""
  local conn_choice=""

  mode_choice="${CPAMP_INSTALL_MODE:-${install_mode:-}}"
  if [ -z "$mode_choice" ]; then
    mode_choice="$(prompt_choice "$(text select_mode)" "1" "1 2")"
    case "$mode_choice" in
      1) install_mode="stack" ;;
      2) install_mode="cpamp" ;;
    esac
  else
    install_mode="$mode_choice"
  fi

  method_choice="${CPAMP_DEPLOY_METHOD:-${deploy_method:-}}"
  if [ "$existing_install_state" = "orphan-volume" ] && [ "$operation" = "repair" ]; then
    method_choice="docker"
  fi
  if [ -z "$method_choice" ]; then
    method_choice="$(prompt_choice "$(text select_method)" "1" "1 2")"
    case "$method_choice" in
      1) deploy_method="docker" ;;
      2) deploy_method="native" ;;
    esac
  else
    deploy_method="$method_choice"
  fi

  case "$install_mode" in
    stack|full|all) install_mode="stack" ;;
    cpamp|manager|cpamp-only) install_mode="cpamp" ;;
    *) die "Unsupported CPAMP_INSTALL_MODE: $install_mode" ;;
  esac

  case "$deploy_method" in
    docker|compose) deploy_method="docker" ;;
    native|binary) deploy_method="native" ;;
    *) die "Unsupported CPAMP_DEPLOY_METHOD: $deploy_method" ;;
  esac

  cpamp_api_port="${CPAMP_API_PORT:-${cpamp_api_port:-8137}}"
  cpamp_panel_port="$(prompt_line "$(text cpamp_panel_port)" "${CPAMP_PANEL_PORT:-${cpamp_panel_port:-18137}}")"
  cpamp_port="${CPAMP_LEGACY_PORT:-${CPAMP_PORT:-${cpamp_port:-18317}}}"
  normalize_port "$cpamp_api_port" || die "Invalid CPAMP API port: $cpamp_api_port"
  normalize_port "$cpamp_panel_port" || die "Invalid CPAMP panel port: $cpamp_panel_port"
  normalize_port "$cpamp_port" || die "Invalid CPAMP port: $cpamp_port"
  if [ "$cpamp_api_port" = "$cpamp_panel_port" ] || [ "$cpamp_api_port" = "$cpamp_port" ] || [ "$cpamp_panel_port" = "$cpamp_port" ]; then
    die "CPAMP gateway, management, and compatibility ports must be different."
  fi

  if [ "$deploy_method" = "docker" ]; then
    cpamp_image="$(prompt_line "$(text cpamp_image)" "${CPAMP_IMAGE:-${cpamp_image:-$default_cpamp_image}}")"
    validate_image_ref "$(text cpamp_image)" "$cpamp_image"
    if [ "$install_mode" = "stack" ]; then
      cpa_port="$(prompt_line "$(text cpa_port)" "${CPAMP_CPA_PORT:-${cpa_port:-8317}}")"
      normalize_port "$cpa_port" || die "Invalid CPA port: $cpa_port"
      if [ "$cpa_port" = "$cpamp_api_port" ] || [ "$cpa_port" = "$cpamp_panel_port" ] || [ "$cpa_port" = "$cpamp_port" ]; then
        die "CPA and CPAMP host ports must be different."
      fi
      cpa_image="$(prompt_line "$(text cpa_image)" "${CPAMP_CPA_IMAGE:-${cpa_image:-$default_cpa_image}}")"
      validate_image_ref "$(text cpa_image)" "$cpa_image"
      cpa_url="http://cli-proxy-api:8317"
      cpa_connection_mode="env"
      deployment_mode="installer-managed"
    fi
  else
    cpamp_version="$(prompt_line "$(text version)" "${CPAMP_VERSION:-latest}")"
    validate_single_line "$(text version)" "$cpamp_version"
    validate_release_version "$cpamp_version"
    if [ -n "${CPAMP_USAGE_DB_PATH:-}" ]; then
      usage_db_path="$(resolve_native_path "$CPAMP_USAGE_DB_PATH" "$install_dir")"
    elif [ -z "$usage_db_path" ]; then
      usage_db_path="$install_dir/data/usage.sqlite"
    fi
    validate_single_line "$(text usage_db_path)" "$usage_db_path"
    if [ "$install_mode" = "stack" ]; then
      cpa_connection_mode="integrated"
      deployment_mode="integrated"
    fi
  fi

  if [ "$install_mode" = "cpamp" ]; then
    conn_choice="${CPAMP_CPA_CONNECTION_MODE:-${cpa_connection_mode:-}}"
    if [ -z "$conn_choice" ]; then
      if [ "$non_interactive" = "1" ]; then
        cpa_connection_mode="setup"
      elif [ "$existing_install_state" = "native-managed" ] &&
           { [ "$operation" = "upgrade" ] || [ "$operation" = "regenerate" ]; }; then
        conn_choice="$(prompt_choice "$(text cpa_conn_mode_preserve)" "3" "1 2 3")"
        case "$conn_choice" in
          1) cpa_connection_mode="env" ;;
          2) cpa_connection_mode="setup" ;;
          3) cpa_connection_mode="preserve" ;;
        esac
      else
        conn_choice="$(prompt_choice "$(text cpa_conn_mode)" "1" "1 2")"
        case "$conn_choice" in
          1) cpa_connection_mode="env" ;;
          2) cpa_connection_mode="setup" ;;
        esac
      fi
    else
      cpa_connection_mode="$conn_choice"
    fi

    case "$cpa_connection_mode" in
      setup|panel|later) cpa_connection_mode="setup" ;;
      env|secret|managed) cpa_connection_mode="env" ;;
      preserve)
        if [ "$existing_install_state" != "native-managed" ] ||
           { [ "$operation" != "upgrade" ] && [ "$operation" != "regenerate" ]; }; then
          die "CPA connection preservation is only available during a native upgrade or config regeneration."
        fi
        ;;
      *) die "Unsupported CPAMP_CPA_CONNECTION_MODE: $cpa_connection_mode" ;;
    esac

    if [ "$cpa_connection_mode" = "env" ]; then
      local default_cpa_url="http://127.0.0.1:8317"
      if [ -n "${CPAMP_CPA_URL:-}" ] || [ -n "${CPAMP_CPA_MANAGEMENT_KEY:-}" ] ||
         [ -z "$cpa_validation_url" ] || [ -z "$cpa_management_key" ]; then
        collect_existing_cpa_connection "$default_cpa_url"
      fi
      if [ "$deploy_method" = "docker" ]; then
        cpa_url="$(cpa_url_for_docker "$cpa_validation_url")"
      else
        cpa_url="$cpa_validation_url"
      fi
      deployment_mode="installer-managed"
    elif [ "$cpa_connection_mode" = "preserve" ]; then
      deployment_mode="${deployment_mode:-slim}"
    else
      deployment_mode="slim"
    fi
  fi

  if { [ "$existing_install_state" = "managed" ] || [ "$existing_install_state" = "native-managed" ]; } &&
     { [ -n "$existing_deploy_method" ] || [ -n "$existing_install_mode" ] || [ -n "$existing_cpa_connection_mode" ]; }; then
    if [ "$deploy_method" != "$existing_deploy_method" ] ||
       [ "$install_mode" != "$existing_install_mode" ] ||
       [ "$cpa_connection_mode" != "$existing_cpa_connection_mode" ]; then
      die "$(text topology_conversion)"
    fi
  fi
}

reset_choices_for_modify() {
  local existing_layout_locked="0"
  if [ "$existing_install_state" = "managed" ] || [ "$existing_install_state" = "native-managed" ]; then
    existing_layout_locked="1"
  fi
  if [ -z "${CPAMP_INSTALL_MODE:-}" ] && [ "$existing_layout_locked" != "1" ]; then
    install_mode=""
  fi
  if [ -z "${CPAMP_DEPLOY_METHOD:-}" ] && [ "$existing_layout_locked" != "1" ]; then
    deploy_method=""
  fi
  if [ -z "${CPAMP_CPA_CONNECTION_MODE:-}" ]; then
    if [ "$existing_layout_locked" = "1" ]; then
      if [ "$operation" = "regenerate" ] && [ "$cpa_connection_mode" = "env" ]; then
        cpa_validation_url=""
        cpa_url=""
        cpa_management_key=""
      fi
    else
      cpa_connection_mode=""
      cpa_validation_url=""
      cpa_url=""
      cpa_management_key=""
    fi
  fi
  deployment_mode=""
}

print_summary() {
  say ""
  say "== $(text summary) =="
  say "$(text operation_label): $(text "operation_${operation}")"
  if [ "$install_mode" = "stack" ]; then
    say "$(text install_mode_label): $(text stack_mode)"
  else
    say "$(text install_mode_label): $(text cpamp_mode)"
  fi
  if [ "$deploy_method" = "docker" ]; then
    say "$(text deploy_method_label): $(text docker_method)"
  else
    say "$(text deploy_method_label): $(text native_method)"
  fi
  say "$(text directory_label): $install_dir"
  if [ "$deploy_method" = "docker" ]; then
    say "$(text project_name): $compose_project_name"
  fi
  say "$(text cpamp_api_port): $cpamp_api_port"
  say "$(text cpamp_panel_port): $cpamp_panel_port"
  say "$(text cpamp_port): $cpamp_port"
  if [ "$deploy_method" = "docker" ]; then
    say "$(text cpamp_image): $cpamp_image"
    if [ "$install_mode" = "stack" ]; then
      say "$(text cpa_image): $cpa_image"
      say "$(text cpa_port): $cpa_port"
      say "$(text cpa_url_for_cpamp): $cpa_url"
      say "$(text cpa_connection_label): $(text cpa_connection_env)"
    else
      if [ "$cpa_connection_mode" = "env" ]; then
        say "$(text cpa_connection_label): $(text cpa_connection_env)"
      else
        say "$(text cpa_connection_label): $(text cpa_connection_setup)"
      fi
      if [ "$cpa_connection_mode" = "env" ]; then
        say "$(text cpa_url): $cpa_url"
      fi
    fi
  else
    say "$(text version): $cpamp_version"
    say "$(text usage_db_path): $usage_db_path"
    if [ "$cpa_connection_mode" = "integrated" ]; then
      say "$(text cpa_connection_label): bundled CPA"
    elif [ "$cpa_connection_mode" = "env" ]; then
      say "$(text cpa_connection_label): $(text cpa_connection_env)"
    elif [ "$cpa_connection_mode" = "preserve" ]; then
      say "$(text cpa_connection_label): $(text cpa_connection_preserve)"
    else
      say "$(text cpa_connection_label): $(text cpa_connection_setup)"
    fi
    if [ "$cpa_connection_mode" = "env" ]; then
      say "$(text cpa_url): $cpa_url"
    fi
  fi
}

confirm_choices() {
  local answer=""

  if [ "$non_interactive" = "1" ]; then
    if [ "$dry_run" = "1" ] || [ "${CPAMP_CONFIRM:-0}" = "1" ]; then
      return
    fi
    die "Set CPAMP_CONFIRM=1 to execute non-interactively."
  fi

  answer="$(prompt_choice "$(text confirm)" "confirm" "confirm modify abort")"
  case "$answer" in
    confirm) return 0 ;;
    modify)
      reset_choices_for_modify
      return 1
      ;;
    abort) exit 0 ;;
  esac
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

check_port() {
  local port="$1"
  if command_exists lsof && lsof -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    say "$(text port_busy): $port"
  fi
}

require_command() {
  local name="$1"
  if ! command_exists "$name"; then
    if [ "$dry_run" = "1" ] || [ "$skip_execute" = "1" ]; then
      say "$(text missing_command): $name"
    else
      die "$(text missing_command): $name"
    fi
  fi
}

check_requirements() {
  check_port "$cpamp_api_port"
  check_port "$cpamp_panel_port"
  check_port "$cpamp_port"
  if [ "$install_mode" = "stack" ] && [ "$deploy_method" = "docker" ]; then
    check_port "$cpa_port"
  fi

  if [ "$deploy_method" = "docker" ]; then
    require_command docker
    if [ "$dry_run" != "1" ] && [ "$skip_execute" != "1" ]; then
      if ! docker compose version >/dev/null 2>&1; then
        die "docker compose is required."
      fi
      if ! docker info >/dev/null 2>&1; then
        die "$(text docker_unavailable)"
      fi
    fi
  else
    case "$normalized_os" in
      linux|darwin) ;;
      *) die "Native install supports Linux and macOS in this script." ;;
    esac
    case "$normalized_arch" in
      amd64|arm64) ;;
      *) die "Unsupported architecture for native package: $arch_name" ;;
    esac
    require_command curl
    if [ "$normalized_os" = "darwin" ] || [ "$normalized_os" = "linux" ]; then
      require_command tar
    fi
  fi
}

random_alnum() {
  local length="${1:-32}"
  local value=""
  local candidate=""
  local empty_attempts=0
  local max_empty_attempts=32

  while [ "${#value}" -lt "$length" ]; do
    if command_exists openssl; then
      candidate="$(openssl rand -base64 96 | LC_ALL=C tr -dc 'A-Za-z0-9')"
    elif [ -r /dev/urandom ] && command_exists dd; then
      candidate="$(dd if=/dev/urandom bs=256 count=1 2>/dev/null | LC_ALL=C tr -dc 'A-Za-z0-9')"
    else
      die "openssl or /dev/urandom is required to generate secure keys."
    fi
    if [ -z "$candidate" ]; then
      empty_attempts=$((empty_attempts + 1))
      if [ "$empty_attempts" -ge "$max_empty_attempts" ]; then
        die "Random source produced no usable alphanumeric characters."
      fi
      continue
    fi
    value="${value}${candidate}"
  done

  printf '%s\n' "${value:0:$length}"
}

ensure_dir() {
  local dir="$1"
  if [ "$dry_run" = "1" ]; then
    return
  fi
  mkdir -p "$dir"
}

overwrite_enabled() {
  [ "${CPAMP_OVERWRITE:-0}" = "1" ] || [ "$operation" = "regenerate" ] || [ "$operation" = "upgrade" ]
}

native_runtime_dir_reuse_enabled() {
  [ "${CPAMP_OVERWRITE:-0}" = "1" ] || [ "$operation" = "regenerate" ] || [ "$operation" = "upgrade" ]
}

native_runtime_dir_is_active() {
  local dir="$1"
  local current=""

  current="$(read_shell_cd_value "$install_dir/run.sh" 2>/dev/null || true)"
  [ -n "$current" ] || return 1
  current="$(resolve_native_path "$current" "$install_dir")"
  [ "${current%/}" = "${dir%/}" ]
}

backup_generated_config() {
  local backup_dir=""
  local source=""
  local relative=""

  if { [ "$operation" != "regenerate" ] &&
       { [ "$operation" != "upgrade" ] || [ "$existing_install_state" != "native-managed" ]; }; } ||
     [ "$dry_run" = "1" ]; then
    return 0
  fi
  backup_dir="$install_dir/backups/installer-$(date '+%Y%m%d-%H%M%S')-$$"
  for relative in .env .cpamp-native.env compose.yaml cliproxyapi/config.yaml run.sh cpa-manager-plus.service secrets/cpa-management-key; do
    source="$install_dir/$relative"
    [ -e "$source" ] || continue
    mkdir -p "$backup_dir/$(dirname "$relative")"
    cp -p "$source" "$backup_dir/$relative"
  done
  if [ -d "$backup_dir" ]; then
    generated_backup_dir="$backup_dir"
    say "$(text config_backup): $backup_dir"
  fi
}

prepare_file() {
  local file="$1"
  if [ -e "$file" ] && ! overwrite_enabled; then
    die "File already exists: $file. Set CPAMP_OVERWRITE=1 if you want to overwrite generated config files."
  fi
  mkdir -p "$(dirname "$file")"
}

preflight_file_write() {
  local file="$1"
  if [ "$dry_run" = "1" ]; then
    return
  fi
  if [ -e "$file" ] && ! overwrite_enabled; then
    die "File already exists: $file. Set CPAMP_OVERWRITE=1 if you want to overwrite generated config files."
  fi
}

preflight_native_binary_dir() {
  local dir="$1"
  if [ "$dry_run" = "1" ]; then
    return
  fi
  if [ -d "$dir" ] && ! native_runtime_dir_reuse_enabled; then
    die "Directory already exists: $dir. Set CPAMP_OVERWRITE=1 if you want to reuse it."
  fi
}

ensure_secret_file() {
  local file="$1"
  local value="$2"
  local replace_existing="${3:-0}"
  local existing=""
  local tmp=""

  if [ "$dry_run" = "1" ]; then
    if [ -f "$file" ]; then
      existing="$(< "$file")"
      existing="${existing%$'\r'}"
      validate_secret_value "$file" "$existing"
      if [ "$replace_existing" = "1" ] && [ "$existing" != "$value" ]; then
        validate_secret_value "$file" "$value"
        printf '%s: %s\n' "$(text write_file)" "$file" >&2
        printf '%s\n' "$value"
        return
      fi
      printf '%s\n' "$existing"
      return
    fi
    validate_secret_value "$file" "$value"
    printf '%s: %s\n' "$(text write_file)" "$file" >&2
    printf '%s\n' "$value"
    return
  fi

  mkdir -p "$(dirname "$file")"
  if [ -f "$file" ]; then
    chmod 600 "$file" 2>/dev/null || die "Unable to restrict secret file permissions: $file"
    existing="$(< "$file")"
    existing="${existing%$'\r'}"
    validate_secret_value "$file" "$existing"
    if [ "$replace_existing" = "1" ] && [ "$existing" != "$value" ]; then
      validate_secret_value "$file" "$value"
      tmp="$(mktemp "${file}.tmp.XXXXXX")"
      chmod 600 "$tmp"
      printf '%s\n' "$value" > "$tmp"
      mv -f "$tmp" "$file"
      printf '%s\n' "$value"
      return
    fi
    printf '%s\n' "$existing"
    return
  fi

  validate_secret_value "$file" "$value"
  printf '%s\n' "$value" > "$file"
  chmod 600 "$file"
  printf '%s\n' "$value"
}

write_env_file() {
  local file="$install_dir/.env"
  local tmp="${file}.tmp.$$"
  if [ "$dry_run" = "1" ]; then
    say "$(text write_file): $file"
    return
  fi
  prepare_file "$file"
  {
    printf 'COMPOSE_PROJECT_NAME=%s\n' "$compose_project_name"
    printf 'CPAMP_IMAGE=%s\n' "$cpamp_image"
    printf 'CPAMP_API_PORT=%s\n' "$cpamp_api_port"
    printf 'CPAMP_PANEL_PORT=%s\n' "$cpamp_panel_port"
    printf 'CPAMP_PORT=%s\n' "$cpamp_port"
    printf 'CPA_MANAGER_DEPLOYMENT_MODE=%s\n' "$deployment_mode"
    if [ "$install_mode" = "stack" ]; then
      printf 'CPA_IMAGE=%s\n' "$cpa_image"
      printf 'CPA_PORT=%s\n' "$cpa_port"
    elif [ "$cpa_connection_mode" = "env" ]; then
      printf 'CPA_UPSTREAM_URL=%s\n' "$cpa_url"
    fi
  } > "$tmp"
  mv -f "$tmp" "$file"
}

write_cpa_config() {
  local file="$install_dir/cliproxyapi/config.yaml"
  local tmp="${file}.tmp.$$"
  local escaped_cpa_management_key=""
  local escaped_demo_client_key=""
  if [ "$dry_run" = "1" ]; then
    say "$(text write_file): $file"
    return
  fi
  prepare_file "$file"
  escaped_cpa_management_key="$(yaml_double_quote_escape "$cpa_management_key")"
  escaped_demo_client_key="$(yaml_double_quote_escape "$demo_client_key")"
  cat > "$tmp" <<EOF
host: "0.0.0.0"
port: 8317

remote-management:
  secret-key: "$escaped_cpa_management_key"
  allow-remote: true
  disable-control-panel: false
  disable-auto-update-panel: true
  panel-github-repository: "https://github.com/seakee/CPA-Manager-Plus"

usage-statistics-enabled: true
redis-usage-queue-retention-seconds: 60

auth-dir: "/root/.cli-proxy-api"

api-keys:
  - "$escaped_demo_client_key"
EOF
  mv -f "$tmp" "$file"
}

docker_needs_host_gateway() {
  [ "$deploy_method" = "docker" ] &&
    [ "$install_mode" = "cpamp" ] &&
    [ "$cpa_connection_mode" = "env" ] &&
    [ "$normalized_os" = "linux" ] &&
    case "$cpa_url" in
      *host.docker.internal*) true ;;
      *) false ;;
    esac
}

write_docker_compose() {
  local file="$install_dir/compose.yaml"
  local tmp="${file}.tmp.$$"
  if [ "$dry_run" = "1" ]; then
    say "$(text write_file): $file"
    return
  fi
  prepare_file "$file"

  if [ "$install_mode" = "stack" ]; then
    cat > "$tmp" <<'EOF'
services:
  cli-proxy-api:
    image: ${CPA_IMAGE}
    restart: unless-stopped
    stop_grace_period: 35s
    ports:
      - "${CPA_PORT}:8317"
    volumes:
      - ./cliproxyapi/config.yaml:/CLIProxyAPI/config.yaml
      - ./cliproxyapi/auths:/root/.cli-proxy-api
      - ./cliproxyapi/logs:/CLIProxyAPI/logs

  cpa-manager-plus:
    image: ${CPAMP_IMAGE}
    restart: unless-stopped
    stop_grace_period: 45s
    ports:
      - "${CPAMP_API_PORT}:8137"
      - "${CPAMP_PANEL_PORT}:18137"
      - "${CPAMP_PORT}:18317"
    environment:
      CPA_MANAGER_DEPLOYMENT_MODE: "installer-managed"
      CPA_MANAGER_GATEWAY_ADDRS: "0.0.0.0:8137,0.0.0.0:18137,0.0.0.0:18317"
      USAGE_DB_PATH: "/data/usage.sqlite"
      CPA_MANAGER_DATA_KEY_PATH: "/data/data.key"
      CPA_UPSTREAM_URL: "http://cli-proxy-api:8317"
      CPA_MANAGEMENT_KEY_FILE: "/run/secrets/cpa_management_key"
      USAGE_COLLECTOR_MODE: "auto"
      USAGE_RESP_QUEUE: "usage"
      USAGE_RESP_POP_SIDE: "right"
      USAGE_BATCH_SIZE: "100"
      USAGE_POLL_INTERVAL_MS: "500"
      USAGE_QUERY_LIMIT: "50000"
      USAGE_CORS_ORIGINS: "*"
    volumes:
      - cpa-manager-plus-data:/data
    secrets:
      - cpa_management_key
    depends_on:
      - cli-proxy-api
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:18137/health"]
      interval: 10s
      timeout: 3s
      retries: 3

volumes:
  cpa-manager-plus-data:

secrets:
  cpa_management_key:
    file: ./secrets/cpa-management-key
EOF
  elif [ "$cpa_connection_mode" = "env" ]; then
    cat > "$tmp" <<'EOF'
services:
  cpa-manager-plus:
    image: ${CPAMP_IMAGE}
    restart: unless-stopped
    stop_grace_period: 45s
    ports:
      - "${CPAMP_API_PORT}:8137"
      - "${CPAMP_PANEL_PORT}:18137"
      - "${CPAMP_PORT}:18317"
EOF
    if docker_needs_host_gateway; then
      cat >> "$tmp" <<'EOF'
    extra_hosts:
      - "host.docker.internal:host-gateway"
EOF
    fi
    cat >> "$tmp" <<'EOF'
    environment:
      CPA_MANAGER_DEPLOYMENT_MODE: "installer-managed"
      CPA_MANAGER_GATEWAY_ADDRS: "0.0.0.0:8137,0.0.0.0:18137,0.0.0.0:18317"
      USAGE_DB_PATH: "/data/usage.sqlite"
      CPA_MANAGER_DATA_KEY_PATH: "/data/data.key"
      CPA_UPSTREAM_URL: "${CPA_UPSTREAM_URL}"
      CPA_MANAGEMENT_KEY_FILE: "/run/secrets/cpa_management_key"
      USAGE_COLLECTOR_MODE: "auto"
      USAGE_RESP_QUEUE: "usage"
      USAGE_RESP_POP_SIDE: "right"
      USAGE_BATCH_SIZE: "100"
      USAGE_POLL_INTERVAL_MS: "500"
      USAGE_QUERY_LIMIT: "50000"
      USAGE_CORS_ORIGINS: "*"
    volumes:
      - cpa-manager-plus-data:/data
    secrets:
      - cpa_management_key
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:18137/health"]
      interval: 10s
      timeout: 3s
      retries: 3

volumes:
  cpa-manager-plus-data:

secrets:
  cpa_management_key:
    file: ./secrets/cpa-management-key
EOF
  else
    cat > "$tmp" <<'EOF'
services:
  cpa-manager-plus:
    image: ${CPAMP_IMAGE}
    restart: unless-stopped
    stop_grace_period: 45s
    ports:
      - "${CPAMP_API_PORT}:8137"
      - "${CPAMP_PANEL_PORT}:18137"
      - "${CPAMP_PORT}:18317"
    environment:
      CPA_MANAGER_DEPLOYMENT_MODE: "slim"
      CPA_MANAGER_GATEWAY_ADDRS: "0.0.0.0:8137,0.0.0.0:18137,0.0.0.0:18317"
      USAGE_DB_PATH: "/data/usage.sqlite"
      CPA_MANAGER_DATA_KEY_PATH: "/data/data.key"
      USAGE_COLLECTOR_MODE: "auto"
      USAGE_RESP_QUEUE: "usage"
      USAGE_RESP_POP_SIDE: "right"
      USAGE_BATCH_SIZE: "100"
      USAGE_POLL_INTERVAL_MS: "500"
      USAGE_QUERY_LIMIT: "50000"
      USAGE_CORS_ORIGINS: "*"
    volumes:
      - cpa-manager-plus-data:/data
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:18137/health"]
      interval: 10s
      timeout: 3s
      retries: 3

volumes:
  cpa-manager-plus-data:
EOF
  fi
  mv -f "$tmp" "$file"
}

preflight_docker_files() {
  preflight_file_write "$install_dir/.env"
  preflight_file_write "$install_dir/compose.yaml"
  if [ "$install_mode" = "stack" ]; then
    preflight_file_write "$install_dir/cliproxyapi/config.yaml"
  fi
}

generate_docker_files() {
  local replace_cpa_secret="0"
  preflight_docker_files

  ensure_dir "$install_dir"
  ensure_dir "$install_dir/secrets"

  write_env_file

  if [ "$install_mode" = "stack" ]; then
    ensure_dir "$install_dir/cliproxyapi/auths"
    ensure_dir "$install_dir/cliproxyapi/logs"
    generated_cpa_management_key="cpa_$(random_alnum 32)"
    cpa_management_key="$(ensure_secret_file "$install_dir/secrets/cpa-management-key" "$generated_cpa_management_key")"
    generated_demo_client_key="sk-$(random_alnum 64)"
    demo_client_key="$(ensure_secret_file "$install_dir/secrets/cpa-demo-client-key" "$generated_demo_client_key")"
    write_cpa_config
  elif [ "$cpa_connection_mode" = "env" ]; then
    if [ "$operation" = "regenerate" ]; then
      replace_cpa_secret="1"
    fi
    ensure_secret_file "$install_dir/secrets/cpa-management-key" "$cpa_management_key" "$replace_cpa_secret" >/dev/null
  fi

  write_docker_compose
}

run_docker_install() {
  if [ "$dry_run" = "1" ]; then
    say "$(text run_command): cd \"$install_dir\" && docker compose pull && docker compose up -d"
    return
  fi
  if [ "$skip_execute" = "1" ]; then
    say "$(text skip_execute)"
    say "cd \"$install_dir\" && docker compose pull && docker compose up -d"
    return
  fi
  (
    cd "$install_dir"
    docker compose pull
    docker compose up -d
  )
}

run_docker_repair() {
  local repair_mount=""
  local reset_status=0
  if ! grep -q 'cpamp_admin_key' "$install_dir/compose.yaml" 2>/dev/null; then
    repair_mount="-v $install_dir/secrets/cpamp-admin-key:/run/secrets/cpamp_admin_key:ro"
  fi
  if [ "$dry_run" = "1" ]; then
    say "$(text run_command): cd \"$install_dir\" && docker compose stop -t $docker_stop_timeout_seconds cpa-manager-plus"
    say "$(text run_command): docker compose run --rm ${repair_mount:+$repair_mount }cpa-manager-plus reset-admin-key --admin-key-file /run/secrets/cpamp_admin_key"
    say "$(text run_command): docker compose up -d"
    return
  fi
  if [ "$skip_execute" = "1" ]; then
    say "$(text skip_execute)"
    return
  fi
  say "$(text repairing_admin)"
  (
    cd "$install_dir"
    if [ "$existing_install_state" = "orphan-volume" ]; then
      docker compose pull
    fi
    docker compose stop -t "$docker_stop_timeout_seconds" cpa-manager-plus
    if grep -q 'cpamp_admin_key' compose.yaml 2>/dev/null; then
      if docker compose run --rm cpa-manager-plus reset-admin-key --admin-key-file /run/secrets/cpamp_admin_key; then
        reset_status=0
      else
        reset_status="$?"
      fi
    else
      if docker compose run --rm -v "$install_dir/secrets/cpamp-admin-key:/run/secrets/cpamp_admin_key:ro" cpa-manager-plus reset-admin-key --admin-key-file /run/secrets/cpamp_admin_key; then
        reset_status=0
      else
        reset_status="$?"
      fi
    fi
    if [ "$reset_status" -ne 0 ]; then
      docker compose up -d || true
      die "$(text repair_failed)"
    fi
    if ! docker compose up -d; then
      die "$(text repair_restart_failed)"
    fi
  )
}

wait_docker_health() {
  local attempts="${CPAMP_DOCKER_HEALTH_ATTEMPTS:-30}"
  local i=1
  local internal_port=""

  case "$attempts" in
    ''|*[!0-9]*) attempts=30 ;;
  esac
  if [ "$attempts" -lt 1 ] || [ "$attempts" -gt 300 ]; then
    attempts=30
  fi

  while [ "$i" -le "$attempts" ]; do
    for internal_port in 18137 18317 8137; do
      if (
        cd "$install_dir"
        docker compose exec -T cpa-manager-plus wget -qO- \
          "http://127.0.0.1:${internal_port}/health" >/dev/null 2>&1
      ); then
        active_cpamp_internal_port="$internal_port"
        return
      fi
    done
    if [ "$i" -lt "$attempts" ]; then
      sleep 2
    fi
    i=$((i + 1))
  done
  return 1
}

capture_docker_upgrade_state() {
  previous_cpamp_image_id=""
  previous_cpa_image_id=""
  previous_cpamp_image_ref=""
  previous_cpa_image_ref=""
  if [ "$dry_run" = "1" ] || [ "$skip_execute" = "1" ]; then
    return 0
  fi
  previous_cpamp_image_ref="$(resolve_compose_service_image cpa-manager-plus 2>/dev/null || true)"
  previous_cpamp_image_id="$(
    cd "$install_dir"
    docker compose images -q cpa-manager-plus 2>/dev/null | sed -n '1p'
  )" || true
  if [ "$install_mode" = "stack" ]; then
    previous_cpa_image_ref="$(resolve_compose_service_image cli-proxy-api 2>/dev/null || true)"
    previous_cpa_image_id="$(
      cd "$install_dir"
      docker compose images -q cli-proxy-api 2>/dev/null | sed -n '1p'
    )" || true
  fi
}

resolve_compose_service_image() {
  local service="$1"

  (
    cd "$install_dir"
    docker compose config 2>/dev/null
  ) | awk -v service="$service" '
    $0 == "  " service ":" { in_service = 1; next }
    in_service && /^  [^[:space:]][^:]*:/ { exit }
    in_service && /^    image:[[:space:]]*/ {
      sub(/^    image:[[:space:]]*/, "")
      gsub(/^"|"$/, "")
      print
      exit
    }
  '
}

preflight_docker_upgrade_rollback() {
  local missing=""

  if [ -z "$previous_cpamp_image_id" ]; then
    missing="cpa-manager-plus image ID"
  elif ! docker image inspect "$previous_cpamp_image_id" >/dev/null 2>&1; then
    missing="cpa-manager-plus image $previous_cpamp_image_id"
  fi
  if [ -z "$previous_cpamp_image_ref" ]; then
    missing="${missing:+$missing; }cpa-manager-plus image reference"
  fi
  case "$previous_cpamp_image_ref" in
    *@*) missing="${missing:+$missing; }cpa-manager-plus digest reference $previous_cpamp_image_ref" ;;
  esac
  if [ "$install_mode" = "stack" ]; then
    if [ -z "$previous_cpa_image_id" ]; then
      missing="${missing:+$missing; }cli-proxy-api image ID"
    elif ! docker image inspect "$previous_cpa_image_id" >/dev/null 2>&1; then
      missing="${missing:+$missing; }cli-proxy-api image $previous_cpa_image_id"
    fi
    if [ -z "$previous_cpa_image_ref" ]; then
      missing="${missing:+$missing; }cli-proxy-api image reference"
    fi
    case "$previous_cpa_image_ref" in
      *@*) missing="${missing:+$missing; }cli-proxy-api digest reference $previous_cpa_image_ref" ;;
    esac
  fi
  if [ -n "$missing" ]; then
    die "$(text docker_upgrade_rollback_unavailable) [$missing]"
  fi
}

rollback_docker_upgrade() {
  local services=(cpa-manager-plus)

  [ -n "$previous_cpamp_image_id" ] || return 1
  case "$previous_cpamp_image_ref" in
    *@*) return 1 ;;
  esac
  if [ "$install_mode" = "stack" ]; then
    services+=(cli-proxy-api)
    [ -n "$previous_cpa_image_id" ] || return 1
    case "$previous_cpa_image_ref" in
      *@*) return 1 ;;
    esac
  fi

  (
    cd "$install_dir"
    docker compose stop -t "$docker_stop_timeout_seconds" "${services[@]}"
    docker image tag "$previous_cpamp_image_id" "$previous_cpamp_image_ref"
    if [ "$install_mode" = "stack" ]; then
      docker image tag "$previous_cpa_image_id" "$previous_cpa_image_ref"
      docker compose up -d --force-recreate --pull never "${services[@]}"
    else
      docker compose up -d --force-recreate --pull never "${services[@]}"
    fi
  ) || return 1

  active_cpamp_internal_port=""
  wait_docker_health
}

run_docker_upgrade() {
  local services=(cpa-manager-plus)

  if [ "$install_mode" = "stack" ]; then
    services+=(cli-proxy-api)
  fi
  if [ "$dry_run" = "1" ] || [ "$skip_execute" = "1" ]; then
    if [ "$dry_run" = "1" ]; then
      say "$(text run_command): cd \"$install_dir\" && docker compose pull ${services[*]} && docker compose stop -t $docker_stop_timeout_seconds ${services[*]} && docker compose up -d ${services[*]}"
    else
      say "$(text skip_execute)"
      say "cd \"$install_dir\" && docker compose pull ${services[*]} && docker compose stop -t $docker_stop_timeout_seconds ${services[*]} && docker compose up -d ${services[*]}"
    fi
    return
  fi

  capture_docker_upgrade_state
  preflight_docker_upgrade_rollback
  active_cpamp_internal_port=""
  if ! (
    cd "$install_dir"
    docker compose pull "${services[@]}"
    docker compose stop -t "$docker_stop_timeout_seconds" "${services[@]}"
    docker compose up -d "${services[@]}"
  ); then
    if rollback_docker_upgrade; then
      die "$(text docker_upgrade_rolled_back)"
    fi
    die "$(text docker_upgrade_rollback_failed)"
  fi
  if ! wait_docker_health; then
    if rollback_docker_upgrade; then
      die "$(text docker_upgrade_rolled_back)"
    fi
    die "$(text docker_upgrade_rollback_failed)"
  fi
}

verify_docker_admin_key() {
  local internal_port="${active_cpamp_internal_port:-18137}"
  [ -n "$admin_key" ] || return 1
  (
    cd "$install_dir"
    docker compose exec -T cpa-manager-plus wget -qO- \
      --header="Authorization: Bearer $admin_key" \
      "http://127.0.0.1:${internal_port}/status" >/dev/null 2>&1
  )
}

validate_docker_install() {
  local answer=""

  if [ "$dry_run" = "1" ] || [ "$skip_execute" = "1" ]; then
    auth_validation_status="skipped"
    return
  fi
  if [ -z "$active_cpamp_internal_port" ] && ! wait_docker_health; then
    die "$(text health_failed) Run 'cd \"$install_dir\" && docker compose logs cpa-manager-plus' for details."
  fi
  if [ -z "$admin_key" ]; then
    auth_validation_status="pending-ui"
    return
  fi
  if verify_docker_admin_key; then
    auth_validation_status="verified"
    return
  fi

  say "$(text auth_failed)" >&2
  if [ "$operation" = "repair" ]; then
    die "$(text repair_verify_failed)"
  fi
  if [ "$non_interactive" = "1" ]; then
    die "Run again with CPAMP_OPERATION=repair to synchronize the admin credential without deleting data."
  fi
  answer="$(prompt_choice "$(text auth_repair_prompt)" "yes" "yes no")"
  if [ "$answer" != "yes" ]; then
    die "Run the installer again and choose repair admin login."
  fi
  operation="repair"
  run_docker_repair
  if ! wait_docker_health || ! verify_docker_admin_key; then
    die "$(text repair_verify_failed)"
  fi
  auth_validation_status="verified"
}

resolve_latest_version() {
  local version="$cpamp_version"
  local effective_url=""
  if [ "$version" != "latest" ]; then
    validate_release_version "$version"
    printf '%s\n' "$version"
    return
  fi
  if [ "$dry_run" = "1" ]; then
    version="${CPAMP_VERSION_RESOLVED:-vX.Y.Z}"
    validate_release_version "$version"
    printf '%s\n' "$version"
    return
  fi
  effective_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${repo}/releases/latest")"
  version="${effective_url##*/}"
  validate_release_version "$version"
  printf '%s\n' "$version"
}

sha256_digest() {
  local file="$1"

  if command_exists sha256sum; then
    sha256sum "$file" | awk '{print $1}'
    return
  fi
  if command_exists shasum; then
    shasum -a 256 "$file" | awk '{print $1}'
    return
  fi
  if command_exists openssl; then
    openssl dgst -sha256 "$file" | awk '{print $NF}'
    return
  fi
  die "sha256sum, shasum, or openssl is required to verify native release assets."
}

verify_release_asset_checksum() {
  local checksum_file="$1"
  local asset_file="$2"
  local asset_name="$3"
  local expected=""
  local checksum=""
  local filename=""
  local actual=""

  while read -r checksum filename _; do
    filename="${filename#\*}"
    filename="${filename#./}"
    if [ "$filename" = "$asset_name" ]; then
      expected="$checksum"
      break
    fi
  done < "$checksum_file"
  if [ "${#expected}" -ne 64 ]; then
    die "Release checksum is missing for native asset: $asset_name"
  fi
  case "$expected" in
    *[!0-9A-Fa-f]*) die "Release checksum is invalid for native asset: $asset_name" ;;
  esac
  actual="$(sha256_digest "$asset_file")"
  if [ "$(printf '%s' "$actual" | tr 'A-F' 'a-f')" != "$(printf '%s' "$expected" | tr 'A-F' 'a-f')" ]; then
    die "Native release checksum verification failed: $asset_name"
  fi
}

download_native_release_asset() {
  local asset_url="$1"
  local archive="$2"
  local version="$3"
  local asset_name="$4"
  local checksum_file="$install_dir/downloads/checksums-${version}.txt"
  local checksum_url="https://github.com/${repo}/releases/download/${version}/checksums.txt"

  if ! curl -fL "$asset_url" -o "$archive"; then
    return 1
  fi
  if ! curl -fL "$checksum_url" -o "$checksum_file"; then
    die "Unable to download native release checksums: $checksum_url"
  fi
  verify_release_asset_checksum "$checksum_file" "$archive" "$asset_name"
}

install_native_archive() {
  local archive="$1"
  local runtime_dir="$2"
  local package="$3"
  local target_dir="$runtime_dir/$package"
  local staging_dir="$runtime_dir/.cpamp-installer-${package}.$$"
  local extracted_dir="$staging_dir/$package"

  rm -rf "$staging_dir"
  mkdir -p "$staging_dir"
  if ! tar -xzf "$archive" -C "$staging_dir"; then
    rm -rf "$staging_dir"
    die "Unable to extract native package: $archive"
  fi
  if [ ! -f "$extracted_dir/cpa-manager-plus" ]; then
    rm -rf "$staging_dir"
    die "Native package is missing cpa-manager-plus: $archive"
  fi
  if [ -e "$target_dir" ]; then
    if native_runtime_dir_is_active "$target_dir"; then
      rm -rf "$staging_dir"
      die "Refusing to replace the active native runtime directory: $target_dir"
    fi
    if ! native_runtime_dir_reuse_enabled; then
      rm -rf "$staging_dir"
      die "Directory already exists: $target_dir. Set CPAMP_OVERWRITE=1 if you want to reuse it."
    fi
    rm -rf "$target_dir"
  fi
  if ! mv "$extracted_dir" "$target_dir"; then
    rm -rf "$staging_dir"
    die "Unable to activate native package: $target_dir"
  fi
  rm -rf "$staging_dir"
}

write_native_run_script() {
  local binary_dir="$1"
  local file="$install_dir/run.sh"
  local tmp="${file}.tmp.$$"
  if [ "$dry_run" = "1" ]; then
    say "$(text write_file): $file"
    return
  fi
  prepare_file "$file"
  {
    printf '#!/usr/bin/env bash\n'
    printf 'set -euo pipefail\n'
    printf 'export CPA_MANAGER_RUNTIME_DATA_DIR=%s\n' "$(shell_quote "$install_dir/data")"
    printf 'export USAGE_DATA_DIR=%s\n' "$(shell_quote "$install_dir/data")"
    printf 'export USAGE_DB_PATH=%s\n' "$(shell_quote "$usage_db_path")"
    printf 'export CPA_MANAGER_DATA_KEY_PATH=%s\n' "$(shell_quote "$install_dir/data/data.key")"
    if [ -n "$deployment_mode" ]; then
      printf 'export CPA_MANAGER_DEPLOYMENT_MODE=%s\n' "$(shell_quote "$deployment_mode")"
    fi
    printf 'export CPA_MANAGER_GATEWAY_ADDRS=%s\n' "$(shell_quote "0.0.0.0:$cpamp_api_port,0.0.0.0:$cpamp_panel_port,0.0.0.0:$cpamp_port")"
    if [ "$cpa_connection_mode" = "env" ]; then
      printf 'export CPA_UPSTREAM_URL=%s\n' "$(shell_quote "$cpa_url")"
      printf 'export CPA_MANAGEMENT_KEY_FILE=%s\n' "$(shell_quote "$install_dir/secrets/cpa-management-key")"
    fi
    printf 'cd %s\n' "$(shell_quote "$binary_dir")"
    printf 'exec ./cpa-manager-plus runtime\n'
  } > "$tmp"
  mv -f "$tmp" "$file"
  chmod 755 "$file"
}

write_native_metadata() {
  local file="$install_dir/.cpamp-native.env"
  local tmp="${file}.tmp.$$"
  if [ "$dry_run" = "1" ]; then
    say "$(text write_file): $file"
    return
  fi
  prepare_file "$file"
  {
    printf 'CPAMP_INSTALL_MODE=%s\n' "$install_mode"
    printf 'CPAMP_API_PORT=%s\n' "$cpamp_api_port"
    printf 'CPAMP_PANEL_PORT=%s\n' "$cpamp_panel_port"
    printf 'CPAMP_PORT=%s\n' "$cpamp_port"
    printf 'USAGE_DB_PATH=%s\n' "$usage_db_path"
    printf 'CPA_MANAGER_DEPLOYMENT_MODE=%s\n' "$deployment_mode"
    if [ "$cpa_connection_mode" = "env" ]; then
      printf 'CPA_UPSTREAM_URL=%s\n' "$cpa_url"
    fi
  } > "$tmp"
  mv -f "$tmp" "$file"
  chmod 600 "$file"
}

preflight_native_files() {
  local binary_dir="$1"
  preflight_native_binary_dir "$binary_dir"
  preflight_file_write "$install_dir/run.sh"
  preflight_file_write "$install_dir/.cpamp-native.env"
  if [ "$normalized_os" = "linux" ]; then
    preflight_file_write "$install_dir/cpa-manager-plus.service"
  fi
}

write_native_systemd_service() {
  local binary_dir="$1"
  local file="$install_dir/cpa-manager-plus.service"
  local tmp="${file}.tmp.$$"
  local escaped_run_script=""
  if [ "$normalized_os" != "linux" ]; then
    return
  fi
  if [ "$dry_run" = "1" ]; then
    say "$(text write_file): $file"
    return
  fi
  prepare_file "$file"
  escaped_run_script="$(systemd_double_quote_escape "$install_dir/run.sh")"
  cat > "$tmp" <<EOF
[Unit]
Description=CPA Manager Plus
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
ExecStart=/bin/bash "$escaped_run_script"
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF
  mv -f "$tmp" "$file"
}

generate_native_files() {
  local version=""
  local package=""
  local ext="tar.gz"
  local archive=""
  local asset_url=""
  local runtime_dir="$install_dir/runtime"
  local binary_dir=""
  local flavor="slim"
  local compatibility_package=""
  local compatibility_url=""
  local checksum_url=""
  local reuse_active_runtime="0"
  local replace_cpa_secret="0"

  version="$(resolve_latest_version)"
  if [ "$install_mode" = "stack" ]; then
    flavor="full"
  fi
  package="cpa-manager-plus_${version}_${normalized_os}_${normalized_arch}_${flavor}"
  archive="$install_dir/downloads/${package}.${ext}"
  asset_url="https://github.com/${repo}/releases/download/${version}/${package}.${ext}"
  binary_dir="$runtime_dir/$package"
  compatibility_package="cpa-manager-plus_${version}_${normalized_os}_${normalized_arch}"
  compatibility_url="https://github.com/${repo}/releases/download/${version}/${compatibility_package}.${ext}"
  checksum_url="https://github.com/${repo}/releases/download/${version}/checksums.txt"

  preflight_native_files "$binary_dir"
  if [ "$flavor" = "slim" ]; then
    preflight_native_binary_dir "$runtime_dir/$compatibility_package"
  fi
  if [ -f "$binary_dir/cpa-manager-plus" ] && native_runtime_dir_is_active "$binary_dir"; then
    reuse_active_runtime="1"
  fi

  ensure_dir "$install_dir"
  ensure_dir "$install_dir/secrets"
  ensure_dir "$install_dir/data"
  ensure_dir "$install_dir/downloads"
  ensure_dir "$runtime_dir"

  if [ "$cpa_connection_mode" = "env" ]; then
    if [ "$operation" = "regenerate" ]; then
      replace_cpa_secret="1"
    fi
    ensure_secret_file "$install_dir/secrets/cpa-management-key" "$cpa_management_key" "$replace_cpa_secret" >/dev/null
  fi

  if [ "$dry_run" = "1" ]; then
    say "$(text run_command): curl -fL \"$asset_url\" -o \"$archive\""
    say "$(text run_command): curl -fL \"$checksum_url\" -o \"$install_dir/downloads/checksums-${version}.txt\""
    say "$(text run_command): verify SHA-256 for ${package}.${ext}"
    say "$(text run_command): tar -xzf \"$archive\" -C \"$runtime_dir\""
    target_native_binary_dir="$binary_dir"
    write_native_run_script "$binary_dir"
    write_native_systemd_service "$binary_dir"
    write_native_metadata
    return
  fi

  if [ "$skip_execute" != "1" ] && [ "$reuse_active_runtime" != "1" ]; then
    if ! download_native_release_asset "$asset_url" "$archive" "$version" "${package}.${ext}"; then
      if [ "$flavor" != "slim" ]; then
        die "Unable to download native Full package: $asset_url"
      fi
      say "Slim asset suffix is unavailable; trying the compatibility asset."
      download_native_release_asset "$compatibility_url" "$archive" "$version" "${compatibility_package}.${ext}"
      package="$compatibility_package"
      binary_dir="$runtime_dir/$compatibility_package"
    fi
    install_native_archive "$archive" "$runtime_dir" "$package"
  elif [ "$skip_execute" = "1" ]; then
    say "$(text skip_execute)"
    say "curl -fL \"$asset_url\" -o \"$archive\""
    say "tar -xzf \"$archive\" -C \"$runtime_dir\""
  else
    say "The requested native runtime is already active; reusing the existing package files."
  fi

  target_native_binary_dir="$binary_dir"
  write_native_run_script "$binary_dir"
  write_native_systemd_service "$binary_dir"
  write_native_metadata
}

generate_native_regenerated_files() {
  local binary_dir="$existing_native_binary_dir"
  local replace_cpa_secret="0"

  if [ -z "$binary_dir" ] || [ ! -f "$binary_dir/cpa-manager-plus" ]; then
    generate_native_files
    return
  fi

  target_native_binary_dir="$binary_dir"
  preflight_native_files "$binary_dir"
  ensure_dir "$install_dir"
  ensure_dir "$install_dir/secrets"
  ensure_dir "$install_dir/data"
  if [ "$cpa_connection_mode" = "env" ]; then
    replace_cpa_secret="1"
    ensure_secret_file "$install_dir/secrets/cpa-management-key" "$cpa_management_key" "$replace_cpa_secret" >/dev/null
  fi
  write_native_run_script "$binary_dir"
  write_native_systemd_service "$binary_dir"
  write_native_metadata
}

print_log_tail() {
  local log_file="$1"
  if [ -s "$log_file" ] && command_exists tail; then
    printf 'Native CPAMP log tail (%s):\n' "$log_file" >&2
    tail -n 80 "$log_file" >&2 || true
  else
    printf 'Native CPAMP log file: %s\n' "$log_file" >&2
  fi
}

wait_native_health() {
  local pid="$1"
  local log_file="$2"
  local health_port="${3:-$cpamp_panel_port}"
  local health_url="http://127.0.0.1:${health_port}/health"
  local attempts="${CPAMP_NATIVE_HEALTH_ATTEMPTS:-20}"
  local i=1

  case "$attempts" in
    ''|*[!0-9]*) attempts=20 ;;
  esac
  if [ "$attempts" -lt 1 ] || [ "$attempts" -gt 300 ]; then
    attempts=20
  fi

  while [ "$i" -le "$attempts" ]; do
    if ! kill -0 "$pid" >/dev/null 2>&1; then
      print_log_tail "$log_file"
      return 1
    fi
    if command_exists curl && curl -fsS "$health_url" >/dev/null 2>&1; then
      return
    fi
    sleep 0.5
    i=$((i + 1))
  done

  if ! command_exists curl; then
    printf 'curl is not available; native health endpoint was not checked.\n' >&2
    return
  fi

  print_log_tail "$log_file"
  return 1
}

resolve_existing_native_path() {
  local value="$1"
  local resolved=""

  if command_exists realpath; then
    resolved="$(realpath "$value" 2>/dev/null || true)"
    if [ -n "$resolved" ]; then
      printf '%s\n' "$resolved"
      return 0
    fi
  fi
  if command_exists readlink; then
    resolved="$(readlink -f "$value" 2>/dev/null || true)"
    if [ -n "$resolved" ]; then
      printf '%s\n' "$resolved"
      return 0
    fi
  fi
  if [ -d "$value" ]; then
    (cd "$value" 2>/dev/null && pwd -P)
    return
  fi
  (
    cd "$(dirname "$value")" 2>/dev/null
    printf '%s/%s\n' "$(pwd -P)" "$(basename "$value")"
  )
}

native_process_start_marker() {
  local pid="$1"

  if [ -r "/proc/${pid}/stat" ]; then
    awk '{print $22}' "/proc/${pid}/stat" 2>/dev/null
    return
  fi
  ps -ww -p "$pid" -o lstart= 2>/dev/null | awk '{$1=$1;print}'
}

read_native_pid_record() {
  local pid_file="$1"
  local line=""
  local saw_metadata=0

  native_record_format=""
  native_record_pid=""
  native_record_start=""
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      pid=*)
        native_record_pid="${line#pid=}"
        saw_metadata=1
        ;;
      start=*)
        native_record_start="${line#start=}"
        saw_metadata=1
        ;;
      '')
        ;;
      *)
        if [ "$saw_metadata" -eq 0 ]; then
          case "$line" in
            *[!0-9]*) ;;
            *)
              native_record_pid="$line"
              native_record_format="legacy"
              ;;
          esac
        fi
        ;;
    esac
  done < "$pid_file"

  case "$native_record_pid" in
    ''|*[!0-9]*) return 1 ;;
  esac
  if [ "$saw_metadata" -eq 1 ]; then
    native_record_format="metadata"
  fi
  return 0
}

native_managed_cpamp_component_root() {
  resolve_existing_native_path "$install_dir/data/runtime/components/cpamp"
}

native_path_is_managed_cpamp_component() {
  local candidate="$1"
  local candidate_path=""
  local managed_root=""

  candidate_path="$(resolve_existing_native_path "$candidate" 2>/dev/null || true)"
  managed_root="$(native_managed_cpamp_component_root 2>/dev/null || true)"
  [ -n "$candidate_path" ] && [ -n "$managed_root" ] || return 1
  case "$candidate_path" in
    "$managed_root"/*) return 0 ;;
  esac
  return 1
}

native_process_executable_path() {
  local pid="$1"
  local value=""

  if [ -L "/proc/${pid}/exe" ]; then
    resolve_existing_native_path "/proc/${pid}/exe"
    return
  fi
  if command_exists lsof; then
    value="$(lsof -a -p "$pid" -d txt -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1)"
    if [ -n "$value" ]; then
      resolve_existing_native_path "$value"
    fi
  fi
}

native_process_working_directory() {
  local pid="$1"
  local value=""

  if [ -L "/proc/${pid}/cwd" ]; then
    resolve_existing_native_path "/proc/${pid}/cwd"
    return
  fi
  if command_exists lsof; then
    value="$(lsof -a -p "$pid" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1)"
    if [ -n "$value" ]; then
      resolve_existing_native_path "$value"
    fi
  fi
}

native_process_matches_binary_dir() {
  local pid="$1"
  local binary_dir="$2"
  local allow_managed_component="${3:-0}"
  local expected_binary=""
  local expected_dir=""
  local process_binary=""
  local process_dir=""
  local command_line=""

  [ -n "$binary_dir" ] || return 1
  expected_dir="$(resolve_existing_native_path "$binary_dir" 2>/dev/null || true)"
  expected_binary="$(resolve_existing_native_path "$binary_dir/cpa-manager-plus" 2>/dev/null || true)"
  [ -n "$expected_dir" ] && [ -n "$expected_binary" ] || return 1

  process_binary="$(native_process_executable_path "$pid" 2>/dev/null || true)"
  if [ -n "$process_binary" ]; then
    if [ "$process_binary" = "$expected_binary" ]; then
      return 0
    fi
    if [ "$allow_managed_component" = "1" ] && native_path_is_managed_cpamp_component "$process_binary"; then
      return 0
    fi
    return 1
  fi

  command_line="$(ps -ww -p "$pid" -o command= 2>/dev/null | sed 's/^[[:space:]]*//')"
  case "$command_line" in
    "$expected_binary"|"$expected_binary "*) return 0 ;;
  esac
  if [ "$allow_managed_component" = "1" ]; then
    local managed_root=""
    managed_root="$(native_managed_cpamp_component_root 2>/dev/null || true)"
    if [ -n "$managed_root" ]; then
      case "$command_line" in
        "$managed_root"/*) return 0 ;;
      esac
    fi
  fi

  process_dir="$(native_process_working_directory "$pid" 2>/dev/null || true)"
  if [ -n "$process_dir" ] && [ "$process_dir" = "$expected_dir" ]; then
    case "$command_line" in
      ./cpa-manager-plus|./cpa-manager-plus\ *|cpa-manager-plus|cpa-manager-plus\ *) return 0 ;;
    esac
  fi
  return 1
}

find_managed_native_pid() {
  local expected_binary_dir="${1:-$existing_native_binary_dir}"
  local pid_file="$install_dir/cpa-manager-plus.pid"
  local pid=""
  local allow_managed_component="0"
  local current_start=""

  native_running_pid=""
  native_running_start=""
  [ -f "$pid_file" ] || return 1
  [ ! -L "$pid_file" ] || return 2
  read_native_pid_record "$pid_file" || return 1
  pid="$native_record_pid"
  kill -0 "$pid" >/dev/null 2>&1 || return 1
  if [ "$native_record_format" = "metadata" ]; then
    [ -n "$native_record_start" ] || return 2
    current_start="$(native_process_start_marker "$pid")"
    [ -n "$current_start" ] && [ "$current_start" = "$native_record_start" ] || return 2
    allow_managed_component="1"
  fi
  native_process_matches_binary_dir "$pid" "$expected_binary_dir" "$allow_managed_component" || return 2
  native_running_pid="$pid"
  native_running_start="$native_record_start"
  return 0
}

native_health_port_in_use() {
  if command_exists lsof; then
    lsof -iTCP:"$existing_native_health_port" -sTCP:LISTEN >/dev/null 2>&1
    return
  fi
  command_exists curl && curl -fsS --connect-timeout 1 --max-time 1 \
    "http://127.0.0.1:${existing_native_health_port}/health" >/dev/null 2>&1
}

preflight_native_upgrade_process() {
  local status=0

  if [ "$dry_run" = "1" ] || [ "$skip_execute" = "1" ]; then
    return 0
  fi
  if find_managed_native_pid "$existing_native_binary_dir"; then
    return 0
  else
    status="$?"
  fi
  if [ "$status" -eq 2 ] || native_health_port_in_use; then
    die "$(text native_upgrade_manual_stop)"
  fi
}

stop_managed_native_process() {
  local pid="$1"
  local expected_binary_dir="$2"
  local expected_start="${3:-}"
  local attempts=$((native_graceful_stop_timeout_seconds * 4))
  local i=1
  local allow_managed_component="0"
  local current_start=""

  if [ -n "$expected_start" ]; then
    current_start="$(native_process_start_marker "$pid")"
    [ -n "$current_start" ] && [ "$current_start" = "$expected_start" ] || return 1
    allow_managed_component="1"
  fi
  native_process_matches_binary_dir "$pid" "$expected_binary_dir" "$allow_managed_component" || return 1
  kill "$pid" >/dev/null 2>&1 || return 1
  while [ "$i" -le "$attempts" ]; do
    if ! kill -0 "$pid" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
    i=$((i + 1))
  done
  ! kill -0 "$pid" >/dev/null 2>&1
}

write_native_pid_record() {
  local pid="$1"
  local pid_file="$install_dir/cpa-manager-plus.pid"
  local start=""
  local tmp="$pid_file.tmp.$$"

  start="$(native_process_start_marker "$pid")"
  [ -n "$start" ] || return 1
  (
    umask 077
    {
      printf 'pid=%s\n' "$pid"
      printf 'start=%s\n' "$start"
    } > "$tmp"
  )
  mv -f "$tmp" "$pid_file"
  chmod 600 "$pid_file"
}

start_native_process() {
  local health_port="$1"
  local pid_file="$install_dir/cpa-manager-plus.pid"
  local log_file="$install_dir/cpa-manager-plus.log"
  local pid=""

  nohup "$install_dir/run.sh" >> "$log_file" 2>&1 &
  pid="$!"
  if ! write_native_pid_record "$pid"; then
    kill "$pid" >/dev/null 2>&1 || true
    rm -f "$pid_file" "$pid_file.tmp.$$"
    return 1
  fi
  wait_native_health "$pid" "$log_file" "$health_port"
}

restore_native_generated_files() {
  local relative=""
  [ -n "$generated_backup_dir" ] || return 1
  for relative in run.sh cpa-manager-plus.service .cpamp-native.env secrets/cpa-management-key; do
    if [ -f "$generated_backup_dir/$relative" ]; then
      cp -p "$generated_backup_dir/$relative" "$install_dir/$relative"
    else
      rm -f "$install_dir/$relative"
    fi
  done
}

run_native_install() {
  local pid_file="$install_dir/cpa-manager-plus.pid"
  local log_file="$install_dir/cpa-manager-plus.log"
  local status=0

  if [ "$dry_run" = "1" ]; then
    say "$(text run_command): nohup \"$install_dir/run.sh\" >> \"$log_file\" 2>&1 &"
    return
  fi
  if [ "$skip_execute" = "1" ]; then
    say "$(text skip_execute)"
    return
  fi
  if [ -f "$pid_file" ]; then
    if find_managed_native_pid "$target_native_binary_dir"; then
      say "CPAMP is already running with PID $native_running_pid."
      return
    else
      status="$?"
    fi
    if [ "$status" -eq 2 ]; then
      die "$(text native_upgrade_manual_stop)"
    fi
    rm -f "$pid_file"
  fi
  if ! start_native_process "$cpamp_panel_port"; then
    die "Native CPAMP process exited before becoming healthy. Check the log file: $log_file"
  fi
}

run_native_upgrade() {
  local old_pid=""
  local old_start=""
  local had_running_process="0"
  local log_file="$install_dir/cpa-manager-plus.log"
  local status=0

  if [ "$dry_run" = "1" ]; then
    say "$(text run_command): stop the installer-managed native process if it is running"
    say "$(text run_command): nohup \"$install_dir/run.sh\" >> \"$log_file\" 2>&1 &"
    return
  fi
  if [ "$skip_execute" = "1" ]; then
    say "$(text skip_execute)"
    return
  fi

  if find_managed_native_pid "$existing_native_binary_dir"; then
    old_pid="$native_running_pid"
    old_start="$native_running_start"
    had_running_process="1"
  else
    status="$?"
    if [ "$status" -eq 2 ] || native_health_port_in_use; then
      restore_native_generated_files || true
      die "$(text native_upgrade_manual_stop)"
    fi
  fi

  if [ "$had_running_process" = "1" ] && ! stop_managed_native_process "$old_pid" "$existing_native_binary_dir" "$old_start"; then
    restore_native_generated_files || true
    die "Unable to stop the installer-managed native CPAMP process $old_pid."
  fi

  if start_native_process "$cpamp_panel_port"; then
    return
  fi

  if find_managed_native_pid "$target_native_binary_dir"; then
    if ! stop_managed_native_process "$native_running_pid" "$target_native_binary_dir" "$native_running_start"; then
      restore_native_generated_files || true
      die "Native upgrade failed and the new process could not be stopped safely. Check the log file: $log_file"
    fi
  else
    status="$?"
    if [ "$status" -eq 2 ]; then
      restore_native_generated_files || true
      die "Native upgrade failed and the new process could not be identified safely. Check the log file: $log_file"
    fi
  fi
  restore_native_generated_files || true
  if [ "$had_running_process" = "1" ]; then
    if start_native_process "$existing_native_health_port"; then
      die "$(text native_upgrade_rolled_back)"
    fi
    die "Native upgrade failed and the previous version could not be restarted. Check the log file: $log_file"
  fi
  die "Native upgrade failed before a healthy process was started. The previous run script was restored. Check the log file: $log_file"
}

discover_bootstrap_token() {
  local info=""
  local logs=""
  local port=""
  bootstrap_token=""
  bootstrap_required=""
  if [ "$dry_run" = "1" ] || [ "$skip_execute" = "1" ] || [ -n "$admin_key" ]; then
    return
  fi
  if command_exists curl; then
    for port in "$cpamp_panel_port" "$cpamp_port" "$cpamp_api_port"; do
      info="$(curl -fsS --connect-timeout 1 --max-time 2 \
        "http://127.0.0.1:${port}/usage-service/info" 2>/dev/null || true)"
      if printf '%s\n' "$info" | grep -Eq '"bootstrapRequired"[[:space:]]*:[[:space:]]*true([[:space:]]*[,}])'; then
        bootstrap_required="1"
        break
      fi
      if printf '%s\n' "$info" | grep -Eq '"bootstrapRequired"[[:space:]]*:[[:space:]]*false([[:space:]]*[,}])'; then
        bootstrap_required="0"
        return 0
      fi
    done
  fi
  [ "$bootstrap_required" = "1" ] || return 0
  if [ "$deploy_method" = "docker" ]; then
    logs="$(cd "$install_dir" && docker compose logs --no-color cpa-manager-plus 2>/dev/null || true)"
  elif [ -f "$install_dir/cpa-manager-plus.log" ]; then
    logs="$(< "$install_dir/cpa-manager-plus.log")"
  fi
  bootstrap_token="$(printf '%s\n' "$logs" | sed -n 's/.*one-time bootstrap token: \([^[:space:]]*\).*/\1/p' | tail -n 1)"
}

discover_runtime_panel_path() {
  local state=""
  local candidate=""
  local state_file="$install_dir/data/runtime/state.json"

  if [ "$deploy_method" = "docker" ] && command_exists docker && [ -f "$install_dir/compose.yaml" ]; then
    state="$(
      cd "$install_dir" 2>/dev/null &&
        docker compose exec -T cpa-manager-plus cat /data/runtime/state.json 2>/dev/null || true
    )"
  elif [ -f "$state_file" ] && [ ! -L "$state_file" ]; then
    state="$(< "$state_file")"
  fi

  candidate="$(printf '%s\n' "$state" | sed -n 's/.*"panelBasePath"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  case "$candidate" in
    /*)
      case "$candidate" in
        *'?'*|*'#'*|*'\\'*|*$'\r'*|*$'\n'*) return 1 ;;
      esac
      ;;
    *) return 1 ;;
  esac
  printf '%s\n' "$candidate"
}

discover_panel_url() {
  local candidate="$(discover_runtime_panel_path 2>/dev/null || true)"
  local port=""
  local fallback_port="$cpamp_panel_port"

  [ -n "$candidate" ] || candidate="/management.html"
  if command_exists curl; then
    for port in "$cpamp_panel_port" "$cpamp_port" "$cpamp_api_port"; do
      if curl -fsS --connect-timeout 1 --max-time 2 \
        "http://127.0.0.1:${port}/health" >/dev/null 2>&1; then
        printf 'http://127.0.0.1:%s%s\n' "$port" "$candidate"
        return
      fi
    done
  fi

  if [ "$existing_install_state" = "managed" ] && [ "$existing_docker_panel_port_configured" != "1" ]; then
    fallback_port="$cpamp_port"
  fi
  printf 'http://127.0.0.1:%s%s\n' "$fallback_port" "$candidate"
}

post_install_message() {
  local reveal=""
  local panel_url=""
  say ""
  if [ "$dry_run" = "1" ]; then
    say "== $(text dry_run_done) =="
  elif [ "$skip_execute" = "1" ] && { [ "$operation" = "install" ] || [ "$operation" = "regenerate" ]; }; then
    say "== $(text config_done) =="
  elif [ "$skip_execute" = "1" ]; then
    say "== $(text operation_skipped) =="
  else
    say "== $(text done) =="
  fi
  say "$(text operation_label): $(text "operation_${operation}")"
  if [ "$admin_secret_missing" = "1" ]; then
    say "$(text admin_key_file): $install_dir/secrets/cpamp-admin-key"
  elif [ -n "$admin_key" ]; then
    say "$(text key_saved): $install_dir/secrets/cpamp-admin-key"
    say "$(text key_view_command): cat \"$install_dir/secrets/cpamp-admin-key\""
  fi
  if [ "$dry_run" != "1" ] && [ "$skip_execute" != "1" ]; then
    panel_url="$(discover_panel_url)"
    say "$(text open_panel): $panel_url"
  fi
  if [ "$auth_validation_status" = "verified" ]; then
    say "$(text auth_verified)"
  fi
  if [ "$deploy_method" = "docker" ] &&
     [ "$existing_install_state" = "managed" ] &&
     [ "$existing_docker_panel_port_configured" != "1" ] &&
     { [ "$operation" = "upgrade" ] || [ "$operation" = "repair" ]; }; then
    say "$(text legacy_docker_ports)"
  fi
  if [ -n "$admin_key" ] && [ "$non_interactive" != "1" ] && [ "$dry_run" != "1" ] && [ "$skip_execute" != "1" ]; then
    reveal="$(prompt_choice "$(text key_reveal_prompt)" "no" "yes no")"
    if [ "$reveal" = "yes" ]; then
      say "$(text admin_key): $admin_key"
    fi
  fi
  discover_bootstrap_token
  if [ -n "$bootstrap_token" ]; then
    say "$(text bootstrap_token): $bootstrap_token"
    say "$(text bootstrap_hint)"
  elif [ "$bootstrap_required" = "1" ]; then
    say "$(text bootstrap_command)"
  fi
  if [ "$install_mode" = "stack" ] && [ "$deploy_method" = "docker" ]; then
    say "$(text cpa_key_file): $install_dir/secrets/cpa-management-key"
    say "$(text demo_client_key_file): $install_dir/secrets/cpa-demo-client-key"
    if [ "$dry_run" != "1" ] && [ "$skip_execute" != "1" ]; then
      say "$(text next_full_stack)"
    fi
  elif [ "$install_mode" = "stack" ]; then
    if [ "$dry_run" != "1" ] && [ "$skip_execute" != "1" ]; then
      say "$(text next_full_stack)"
    fi
  elif [ "$cpa_connection_mode" = "setup" ]; then
    if [ "$dry_run" != "1" ] && [ "$skip_execute" != "1" ]; then
      say "$(text next_setup)"
    fi
  elif [ "$cpa_connection_mode" = "preserve" ]; then
    if [ "$dry_run" != "1" ] && [ "$skip_execute" != "1" ]; then
      say "$(text next_native_preserved)"
    fi
  else
    say "$(text cpa_key_file): $install_dir/secrets/cpa-management-key"
    if [ "$dry_run" != "1" ] && [ "$skip_execute" != "1" ]; then
      say "$(text next_env_managed)"
    fi
  fi
  if [ "$deploy_method" = "native" ] && [ "$normalized_os" = "linux" ]; then
    say "$(text systemd_file): $install_dir/cpa-manager-plus.service"
  fi
}

main() {
  detect_environment
  require_interactive_tty
  if [ -z "$lang_code" ] && [ "$non_interactive" != "1" ]; then
    show_environment
  fi
  choose_language
  show_environment
  confirm_environment

  collect_install_directory
  detect_existing_installation
  resolve_operation
  export COMPOSE_PROJECT_NAME="$compose_project_name"

  if [ "$existing_install_state" = "managed" ] &&
     { [ "$operation" = "upgrade" ] || [ "$operation" = "repair" ]; }; then
    load_existing_docker_config
    print_summary
    check_requirements
    if [ "$operation" = "upgrade" ]; then
      run_docker_upgrade
    else
      if [ "$dry_run" != "1" ] && [ "$skip_execute" != "1" ]; then
        ensure_repair_admin_key
      fi
      run_docker_repair
    fi
    validate_docker_install
    post_install_message
    return
  fi

  if [ "$existing_install_state" = "native-managed" ] &&
     { [ "$operation" = "upgrade" ] || [ "$operation" = "regenerate" ]; }; then
    load_existing_native_config
    while true; do
      collect_choices || continue
      [ "$deploy_method" = "native" ] || die "An existing native deployment can only be upgraded with CPAMP_DEPLOY_METHOD=native."
      print_summary
      if confirm_choices; then
        break
      fi
    done
    check_requirements
    preflight_native_upgrade_process
    backup_generated_config
    if [ "$operation" = "regenerate" ]; then
      generate_native_regenerated_files
    else
      generate_native_files
    fi
    run_native_upgrade
    post_install_message
    return
  fi

  if [ "$operation" = "regenerate" ] && [ "$existing_install_state" = "managed" ]; then
    load_existing_docker_config
  fi

  if [ "$operation" = "install" ]; then
    detect_existing_cpa
    choose_detected_cpa
  fi

  while true; do
    collect_choices || continue
    print_summary
    if confirm_choices; then
      break
    fi
  done

  check_requirements

  if [ "$deploy_method" = "docker" ]; then
    backup_generated_config
    generate_docker_files
    if [ "$operation" = "repair" ]; then
      if [ "$dry_run" != "1" ] && [ "$skip_execute" != "1" ]; then
        ensure_repair_admin_key
      fi
      run_docker_repair
    else
      run_docker_install
    fi
    validate_docker_install
  else
    backup_generated_config
    generate_native_files
    run_native_install
  fi

  post_install_message
}

main "$@"
