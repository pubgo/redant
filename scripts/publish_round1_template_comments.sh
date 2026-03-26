#!/usr/bin/env bash
set -eo pipefail
repo='pubgo/redant'
pr='8'
sha=$(GH_PAGER=cat gh pr view "$pr" --repo "$repo" --json headRefOid --jq '.headRefOid')
existing=$(GH_PAGER=cat gh api "repos/$repo/pulls/$pr/comments?per_page=100")

ensure_comment(){
  local path="$1" line="$2" category="$3" module="$4" level="$5" problem="$6" reason="$7" fix="$8"
  local found
  found=$(echo "$existing" | jq -r --arg p "$path" --argjson l "$line" --arg c "$category" --arg m "$module" --arg lv "$level" --arg prob "$problem" '
    .[]
    | select(.path==$p and .line==$l)
    | select((.body|contains("分类："+$c)) and (.body|contains("模块："+$m)) and (.body|contains("等级："+$lv)) and (.body|contains("问题："+$prob)))
    | .html_url' | head -n1)
  if [[ -n "$found" ]]; then
    echo "$found"
    return 0
  fi

  body="分类：$category
模块：$module
等级：$level
问题：$problem
原因：$reason
修改意见：$fix"

  jq -nc --arg body "$body" --arg commit_id "$sha" --arg path "$path" --argjson line "$line" '{body:$body,commit_id:$commit_id,path:$path,line:$line,side:"RIGHT"}' \
    | GH_PAGER=cat gh api --method POST "repos/$repo/pulls/$pr/comments" --input - --jq '.html_url'
}

u1=$(ensure_comment 'args.go' 66 '兼容性' 'args' 'Blocker' '内部覆盖参数标志使用公开语义名 --args，存在与业务命令自定义 --args 冲突风险。' '同名时可能把业务输入误当内部协议，触发参数覆盖，破坏既有参数解析语义并导致兼容性回归。' '将内部标志改为保留命名（如 --__redant-internal-args），并仅在内部模式启用；同时补充冲突回归测试。')
u2=$(ensure_comment 'command.go' 776 '功能正确性' 'command' 'Blocker' '检测到 internalArgsOverrideFlag 被设置后会直接覆盖 inv.Args。' '一旦与业务 flag 同名，位置参数会被非预期重写，影响命令行为正确性。' '增加“仅内部模式可覆盖”的前置条件，或改内部 flag 命名并提供兼容迁移与测试。')
u3=$(ensure_comment 'env_preload.go' 119 '可维护性' 'env_preload' 'Minor' 'parseShortEFlag 对 -e 前缀匹配范围较宽，存在误判风险。' '宽匹配可能把常规短参数提前吞入预加载阶段，导致报错时机与类型偏移。' '收紧匹配规则（仅接受 -e / -e=... / -eKEY=VAL），并补充歧义输入测试用例。')

echo "comment1=$u1"
echo "comment2=$u2"
echo "comment3=$u3"
