#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   bash scripts/get-current-pr.sh [owner/repo]
#   bash scripts/get-current-pr.sh [owner/repo] --ensure-draft [--base main]

repo=""
ensure_draft="false"
base_branch="main"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --ensure-draft)
      ensure_draft="true"
      shift
      ;;
    --base)
      if [[ $# -lt 2 ]]; then
        echo "❌ --base 需要一个分支名参数。" >&2
        exit 1
      fi
      base_branch="$2"
      shift 2
      ;;
    -h|--help)
      cat <<'EOF'
用法:
  bash scripts/get-current-pr.sh [owner/repo]
  bash scripts/get-current-pr.sh [owner/repo] --ensure-draft [--base main]

说明:
  - 默认模式: 仅查询当前分支对应 PR。
  - --ensure-draft: 若未找到 PR，则自动创建 Draft PR。
  - --base: 创建 Draft PR 时使用的 base 分支，默认 main。
EOF
      exit 0
      ;;
    *)
      if [[ -z "$repo" ]]; then
        repo="$1"
      else
        echo "❌ 未识别参数: $1" >&2
        exit 1
      fi
      shift
      ;;
  esac
done

if [[ -z "$repo" ]]; then
  # Prefer gh-native repo detection (works with authenticated gh context).
  repo="$(GH_PAGER=cat gh repo view --json nameWithOwner --jq '.nameWithOwner' 2>/dev/null || true)"
fi

if [[ -z "$repo" ]]; then
  # Fallback: parse git remote URL.
  remote_url="$(git remote get-url origin 2>/dev/null || true)"
  if [[ "$remote_url" =~ github.com[:/]([^/]+/[^/.]+)(\.git)?$ ]]; then
    repo="${BASH_REMATCH[1]}"
  fi
fi

if [[ -z "$repo" ]]; then
  echo "❌ 无法识别仓库，请显式传入 owner/repo，例如: bash scripts/get-current-pr.sh pubgo/redant" >&2
  exit 1
fi

branch="$(git branch --show-current 2>/dev/null || true)"
if [[ -z "$branch" ]]; then
  echo "❌ 当前不在分支上（可能是 detached HEAD），无法自动定位 PR。" >&2
  exit 1
fi

lookup_pr_line() {
  GH_PAGER=cat gh pr list \
    --repo "$repo" \
    --head "$branch" \
    --state all \
    --limit 1 \
    --json number,state,headRefName,baseRefName,title,url \
    --jq 'if length==0 then "" else .[0] | "#\(.number)\t\(.state)\t\(.headRefName)->\(.baseRefName)\t\(.title)\t\(.url)" end'
}

pr_line="$(lookup_pr_line)"

if [[ -z "$pr_line" ]]; then
  if [[ "$ensure_draft" == "true" ]]; then
    echo "ℹ️ 当前分支 '$branch' 在 $repo 下没有找到对应 PR，开始创建 Draft PR..."
    set +e
    create_output="$(GH_PAGER=cat gh pr create --repo "$repo" --head "$branch" --base "$base_branch" --draft --fill 2>&1)"
    create_code=$?
    set -e

    if [[ $create_code -ne 0 ]]; then
      echo "❌ Draft PR 创建失败（退出码: $create_code）。" >&2
      echo "$create_output" >&2
      exit 3
    fi

    pr_line="$(lookup_pr_line)"
    if [[ -z "$pr_line" ]]; then
      echo "❌ Draft PR 似乎已创建，但未能重新查询到，请稍后重试。" >&2
      exit 4
    fi
    echo "✅ Draft PR 已创建并定位成功。"
    echo "$pr_line"
    exit 0
  fi

  echo "ℹ️ 当前分支 '$branch' 在 $repo 下没有找到对应 PR。"
  exit 2
fi

echo "$pr_line"
