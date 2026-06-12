#!/usr/bin/env bash
#
# run.sh — 清理缓存并后台启动 brand / admin 服务
#
# 用法:
#   ./run.sh          启动 brand 和 admin（默认）
#   ./run.sh brand    仅启动 brand
#   ./run.sh admin    仅启动 admin
#   ./run.sh stop     停止由本脚本启动的服务
#
set -euo pipefail

# 切换到脚本所在目录（backend/）
cd "$(dirname "$0")"

LOG_DIR="logs"
PID_DIR="run"
mkdir -p "$LOG_DIR" "$PID_DIR"

# 服务定义: 名称 -> 配置文件（兼容 macOS 自带 bash 3.2，不用关联数组）
config_for() {
  case "$1" in
    brand) echo "configs/config-brand.yaml" ;;
    admin) echo "configs/config-admin.yaml" ;;
    *) echo "" ;;
  esac
}

# 清理 Go 构建缓存（全局，只能在启动任何服务前调用一次，
# 否则会删掉其它服务正在编译的缓存，导致编译失败）
clean_cache() {
  echo "清理 Go 构建缓存..."
  go clean -cache
}

# 启动单个服务: 后台运行
start_service() {
  local name="$1"
  local cfg
  cfg="$(config_for "$name")"
  local pidfile="$PID_DIR/$name.pid"
  local logfile="$LOG_DIR/$name.log"

  # 若已在运行则跳过
  if [[ -f "$pidfile" ]] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
    echo "[$name] 已在运行 (PID $(cat "$pidfile"))，跳过"
    return
  fi

  echo "[$name] 后台启动 (config=$cfg)..."
  CONFIG_PATH="$cfg" nohup go run ./cmd/api-"$name"/ >"$logfile" 2>&1 &
  echo $! >"$pidfile"
  echo "[$name] 已启动 PID $(cat "$pidfile")，日志: $logfile"
}

# 停止单个服务
stop_service() {
  local name="$1"
  local pidfile="$PID_DIR/$name.pid"
  if [[ -f "$pidfile" ]]; then
    local pid
    pid="$(cat "$pidfile")"
    if kill -0 "$pid" 2>/dev/null; then
      echo "[$name] 停止 PID $pid"
      # go run 会派生子进程，杀掉整个进程组
      pkill -P "$pid" 2>/dev/null || true
      kill "$pid" 2>/dev/null || true
    fi
    rm -f "$pidfile"
  else
    echo "[$name] 未在运行"
  fi
}

case "${1:-all}" in
  brand)
    clean_cache
    start_service brand
    ;;
  admin)
    clean_cache
    start_service admin
    ;;
  all)
    clean_cache
    start_service brand
    start_service admin
    ;;
  stop)
    stop_service brand
    stop_service admin
    ;;
  *)
    echo "用法: $0 [brand|admin|all|stop]" >&2
    exit 1
    ;;
esac
