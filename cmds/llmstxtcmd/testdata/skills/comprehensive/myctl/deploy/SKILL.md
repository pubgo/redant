---
name: myctl_deploy
description: "Deploy to environment."
argument-hint: "target"
---

# myctl deploy

## 使用场景

- Deploy the application to the specified target environment.

## 用法

```sh
myctl deploy <target>
```

### 参数

- `target`（必填） — Target environment.

### 选项

- `-f, --force` — Force deploy.
- `--count int64` — Instance count.（默认: 1; 必填）
- `--endpoint string` — Deploy endpoint.（默认: https://deploy.example.com; 环境变量: DEPLOY_ENDPOINT）

