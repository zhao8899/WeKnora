# Git 工作流说明

## 目标

本仓库采用“双远端 + 双主分支”模式：

- `upstream` 指向官方仓库 `Tencent/WeKnora`
- `origin` 指向你的个人仓库 `zhao8899/WeKnora`

这样可以同时满足两个目标：

- 持续同步官方新功能
- 长期保留本地定制优化

## 当前远端

- `origin`: `https://github.com/zhao8899/WeKnora.git`
- `upstream`: `https://github.com/Tencent/WeKnora.git`

## 当前主分支约定

- `main`
  - 只用于同步官方代码
  - 跟踪 `upstream/main`
  - 不在这个分支上做业务定制

- `custom-main`
  - 用于承载你的定制版本
  - 跟踪 `origin/custom-main`
  - 日常开发、优化、交付都在这个分支进行

## 日常开发

默认在 `custom-main` 上工作：

```bash
git checkout custom-main
```

如需开发新功能，建议从 `custom-main` 拉功能分支：

```bash
git checkout custom-main
git pull
git checkout -b feat/your-feature-name
```

开发完成后：

```bash
git add .
git commit -m "feat(scope): your change"
git push -u origin feat/your-feature-name
```

如果不单独开功能分支，也可以直接在 `custom-main` 上提交并推送：

```bash
git checkout custom-main
git add .
git commit -m "feat(scope): your change"
git push
```

## 同步官方更新

当官方仓库有新提交时，按下面流程同步：

### 1. 更新本地官方同步分支

```bash
git checkout main
git fetch upstream
git reset --hard upstream/main
```

说明：

- `main` 始终保持和官方 `upstream/main` 一致
- 不要在 `main` 上做你的定制修改

### 2. 把官方更新合并到你的定制分支

```bash
git checkout custom-main
git merge main
```

如果有冲突，解决后提交：

```bash
git add .
git commit
```

### 3. 推送你的更新版本

```bash
git push
```

## 推荐同步频率

- 官方活跃更新期：每周同步一次
- 版本升级前：同步一次并完成测试
- 重要安全修复出现时：立即同步评估

## 冲突处理原则

优先级建议如下：

1. 保留你的业务定制和一期收口策略
2. 吸收官方新增能力和缺陷修复
3. 对明显不属于一期目标的官方前台入口继续隐藏而不是删除

如果冲突较大，优先保留：

- 权限收口
- 菜单裁剪
- 一期产品文案
- 首页 / FAQ / 共享空间的一期入口形态

## 不建议的做法

- 不要把 `origin` 指回官方仓库
- 不要直接在 `main` 上做定制开发
- 不要同步官方后立刻覆盖你的前台裁剪逻辑
- 不要把二期、三期能力直接删掉，优先隐藏保留

## 当前建议

当前仓库推荐使用方式：

```bash
git checkout custom-main
```

以后需要同步官方时：

```bash
git checkout main
git fetch upstream
git reset --hard upstream/main
git checkout custom-main
git merge main
git push
```

## 当前状态参考

- 官方同步分支：`main`
- 定制主分支：`custom-main`
- GitHub 推送目标：`origin/custom-main`

